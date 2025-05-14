package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	mattermost "github.com/mattermost/mattermost/server/public/model"
)

var (
	//go:embed icons/dark.svg
	SVGDark []byte

	//go:embed icons/dark_dot_blue.svg
	SVGDarkDotBlue []byte

	//go:embed icons/dark_dot_red.svg
	SVGDarkDotRed []byte

	//go:embed icons/light.svg
	SVGLight []byte

	//go:embed icons/light_dot_blue.svg
	SVGLightDotBlue []byte

	//go:embed icons/light_dot_red.svg
	SVGLightDotRed []byte
)

var (
	iconDark        = fyne.NewStaticResource("iconDark", SVGDark)
	iconDarkDotBlue = fyne.NewStaticResource("iconDarkDotBlue", SVGDarkDotBlue)
	iconDarkDotRed  = fyne.NewStaticResource("iconDarkDotRed", SVGDarkDotRed)
	icon            = fyne.NewStaticResource("icon", SVGLight)
	iconDotBlue     = fyne.NewStaticResource("iconDotBlue", SVGLightDotBlue)
	iconDotRed      = fyne.NewStaticResource("iconDotRed", SVGLightDotRed)
)

type Counts struct {
	Direct   int64
	Mentions int64
	Group    int64
	Channel  int64
	Replies  int64
}

type Counter struct {
	mu     sync.Mutex
	counts map[string]*Counts
	logger *slog.Logger
}

func (s *Counter) SetCounts(counts map[string]*Counts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts = counts
}

func (s *Counter) ProcessChannelViewed(event *mattermost.WebSocketEvent, callback func(counts map[string]*Counts)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	viewedChannels := event.GetData()["channel_times"].(map[string]any)
	for channelId := range viewedChannels {
		before, ok := s.counts[channelId]
		if ok {
			logger.Debug("Channel viewed. Clearing notifications", "channel", channelId, "before", before)
		}
		s.counts[channelId] = &Counts{}
	}

	callback(s.counts)
}

func (s *Counter) ProcessPost(event *mattermost.WebSocketEvent, userId string, callback func(counts map[string]*Counts)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelId := event.GetBroadcast().ChannelId

	counts, ok := s.counts[channelId]
	if !ok {
		s.counts[channelId] = &Counts{}
		counts = s.counts[channelId]
	}
	logger.Debug("Processing post event", "before", counts)

	p, ok := event.GetData()["post"].(string)
	if !ok {
		logger.Error("Failed parsing post", "post", event.GetData()["post"], "error", "not a string")
		return
	}

	var post mattermost.Post
	if err := json.Unmarshal([]byte(p), &post); err != nil {
		logger.Error("Could not unmarshal post", "error", err)
	}

	// Skip system messages like "@user left the channel" etc
	if strings.HasPrefix(post.Type, mattermost.PostSystemMessagePrefix) {
		return
	}

	if post.RootId != "" {
		counts.Replies++
	}

	channelType := mattermost.ChannelType(event.GetData()["channel_type"].(string))
	switch channelType {
	case mattermost.ChannelTypeDirect:
		counts.Direct++
	case mattermost.ChannelTypeGroup:
		counts.Group++
	case mattermost.ChannelTypeOpen:
		counts.Channel++
	case mattermost.ChannelTypePrivate:
		counts.Channel++
	}

	if raw, ok := event.GetData()["mentions"].(string); ok && raw != "" {
		var mentions []string
		if err := json.Unmarshal([]byte(raw), &mentions); err != nil {
			logger.Error("Could not unmarshal mentions", "error", err)
		} else {
			logger.Debug("Got mentions", "mentions", mentions)
			for _, mention := range mentions {
				if mention == userId {
					counts.Mentions++
					break
				}
			}
		}

	}

	callback(s.counts)
	logger.Debug("Processed post event", "after", counts)
}

func (s *Counter) LogAllUnread() {
	s.mu.Lock()
	defer s.mu.Unlock()

	channelIds := make([]string, 0, len(s.counts))
	for k := range s.counts {
		channelIds = append(channelIds, k)
	}
	sort.Strings(channelIds)

	for _, channelId := range channelIds {
		counts := s.counts[channelId]
		if counts.Direct == 0 && counts.Mentions == 0 && counts.Group == 0 && counts.Channel == 0 && counts.Replies == 0 {
			continue
		}
		logger.Debug(fmt.Sprintf(
			"%4d direct, %4d mentions, %4d group, %4d channel, %4d replies, channel: %s",
			counts.Direct, counts.Mentions, counts.Group, counts.Channel, counts.Replies, channelId,
		))
	}
}

var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelDebug,
}))

func runSocket(
	ctx context.Context,
	cfg *Config,
	counter *Counter,
	desk desktop.App,
	user *mattermost.User,
	teams []*mattermost.Team,
	api *mattermost.Client4,
) {

	backoff := 1 * time.Second

	for ctx.Err() == nil { // Keep running as long as the context is not canceled

		if err := syncCounts(ctx, api, teams, user.Id, counter); err != nil {
			logger.Error("Could not sync unread counts", "error", err)
			backoff = sleepBackoff(ctx, backoff)
			continue
		}
		i := pickIcon(counter.counts)
		fyne.Do(func() {
			desk.SetSystemTrayIcon(i)
		})

		ws, err := mattermost.NewWebSocketClient4(fmt.Sprintf("wss://%s", cfg.Host), cfg.Pat)
		if err != nil {
			logger.Error("WebSocket connect failed", "error", err)
			backoff = sleepBackoff(ctx, backoff)
			continue
		}
		ws.Listen()
		logger.Info("Connected to websocket")

		backoff = 1 * time.Second

		done := make(chan struct{})
		go func() {
			for event := range ws.EventChannel {
				logger.Debug("Received event", "eventType", event.EventType(), "event", event)

				switch event.EventType() {
				case mattermost.WebsocketEventPosted:
					counter.ProcessPost(event, user.Id, func(counts map[string]*Counts) {
						pi := pickIcon(counts) // We run pickIcon outside the UI goroutine to prevent concurrent map access
						fyne.Do(func() {
							desk.SetSystemTrayIcon(pi)
						})
					})
				case mattermost.WebsocketEventMultipleChannelsViewed:
					counter.ProcessChannelViewed(event, func(counts map[string]*Counts) {
						pi := pickIcon(counts) // We run pickIcon outside the UI goroutine to prevent concurrent map access
						fyne.Do(func() {
							desk.SetSystemTrayIcon(pi)
						})
					})
				default:
					continue
				}
			}
			close(done) // WebSocket is closed, possibly due to an error
		}()

		select {
		case <-ctx.Done():
			logger.Info("Context canceled, closing websocket")
			ws.Close()
			<-done
		case <-done:
			if err := ws.ListenError; err != nil {
				logger.Error("WebSocket listen failed", "error", err.Error())
			} else {
				logger.Warn("Disconnected from websocket")
			}
			ws.Close()
		}

		backoff = sleepBackoff(ctx, backoff)
	}
}

func sleepBackoff(ctx context.Context, current time.Duration) time.Duration {
	jitter := time.Duration(float64(current) * (1 + 0.2*rand.Float64()))
	wait := current + jitter

	if wait > 60*time.Second { // Never wait for more than a minute
		wait = 60 * time.Second
	}

	logger.Info("Reconnecting in", "wait", wait)
	select {
	case <-time.After(wait):
	case <-ctx.Done():
	}

	if current < 30*time.Second {
		current = current * 2
	}
	return current
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		logger.Error("Could not load config. Exiting.", "error", err)
		os.Exit(1)
	}

	if config.Theme == ThemeDark {
		icon = iconDark
		iconDotBlue = iconDarkDotBlue
		iconDotRed = iconDarkDotRed
	}

	api := mattermost.NewAPIv4Client(fmt.Sprintf("https://%s", config.Host))
	api.SetToken(config.Pat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	user, _, err := api.GetMe(ctx, "")
	if err != nil {
		logger.Error("Could not get user info. Exiting.", "error", err)
		os.Exit(1)
	}
	logger.Debug("Got user info", "user", user)

	teams, _, err := api.GetTeamsForUser(ctx, user.Id, "")
	if err != nil {
		logger.Error("Could not get teams. Exiting.", "error", err)
		os.Exit(1)
	}
	logger.Debug("Found user teams", "teams", teams)

	a := app.New()
	w := a.NewWindow("Mattermost Notifier")
	w.SetContent(widget.NewLabel("Running in the background"))
	w.SetCloseIntercept(w.Hide)

	desk, ok := a.(desktop.App)
	if !ok {
		logger.Error("Detected non desktop OS. Exiting.")
		os.Exit(1)
	}
	desk.SetSystemTrayMenu(fyne.NewMenu("Mattermost",
		fyne.NewMenuItem("Show Window", func() { w.Show() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() { a.Quit() }),
	))

	counter := &Counter{logger: logger}
	go runSocket(ctx, config, counter, desk, user, teams, api)

	ticker := time.NewTicker(10 * time.Second)
	go func() {
		time.Sleep(3 * time.Second)
		for {
			select {
			case <-ticker.C:
				counter.LogAllUnread()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	w.Hide()
	a.Run()
}

func pickIcon(counts map[string]*Counts) fyne.Resource {
	var i fyne.Resource = icon
	for _, c := range counts {
		if c.Direct > 0 || c.Mentions > 0 {
			return iconDotRed
		} else if c.Group > 0 || c.Channel > 0 || c.Replies > 0 {
			i = iconDotBlue
		}
	}
	return i
}

func syncCounts(
	ctx context.Context,
	api *mattermost.Client4,
	teams []*mattermost.Team,
	userId string,
	counter *Counter,
) error {

	fresh := make(map[string]*Counts)

	for _, team := range teams {
		chans, _, err := api.GetChannelsForTeamAndUserWithLastDeleteAt(ctx, team.Id, userId, false, 0, "")
		if err != nil {
			return fmt.Errorf("failed getting for team %s: %w", team.Name, err)
		}

		chansById := make(map[string]*mattermost.Channel, len(chans))
		for _, cn := range chans {
			chansById[cn.Id] = cn
		}

		members, _, err := api.GetChannelMembersForUser(ctx, userId, team.Id, "")
		if err != nil {
			return fmt.Errorf("failed getting members for team %s: %w", team.Name, err)
		}

		for _, mem := range members {
			channel, ok := chansById[mem.ChannelId]
			if !ok {
				continue
			}

			repliesTotal := channel.TotalMsgCount - channel.TotalMsgCountRoot
			repliesSeen := mem.MsgCount - mem.MsgCountRoot
			unreadTotal := channel.TotalMsgCount - mem.MsgCount

			cnt := &Counts{
				Replies:  repliesTotal - repliesSeen,
				Mentions: mem.MentionCount, //TODO: Check if this includes @<some_other_user>
			}

			switch channel.Type {
			case mattermost.ChannelTypeDirect:
				cnt.Direct = unreadTotal
			case mattermost.ChannelTypeGroup:
				cnt.Group = unreadTotal
			case mattermost.ChannelTypeOpen:
				cnt.Channel = unreadTotal
			case mattermost.ChannelTypePrivate:
				cnt.Channel = unreadTotal
			}
			fresh[channel.Id] = cnt
		}
	}

	counter.SetCounts(fresh)
	return nil
}
