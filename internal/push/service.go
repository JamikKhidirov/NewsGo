// Package push provides in-app notifications and FCM push delivery.
package push

import (
	"context"
	"log/slog"
	"time"

	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/platform"
	"github.com/gonews/gonews/internal/store"
)

// Service combines notification persistence and device delivery.
type Service struct {
	st      *store.Store
	sender  Sender
	enabled bool
	log     *slog.Logger
}

// NewService builds the push service.
func NewService(st *store.Store, sender Sender, enabled bool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{st: st, sender: sender, enabled: enabled, log: log}
}

// Broadcast stores an in-app broadcast and (optionally) pushes it to all devices.
func (s *Service) Broadcast(ctx context.Context, title, body, articleID, kind string) (SendResult, error) {
	n := model.Notification{
		ID:    platform.NewID("n"),
		Kind:  defaultKind(kind),
		Title: title,
		Body:  body,
	}
	if articleID != "" {
		n.ArticleID = &articleID
	}
	if err := s.st.CreateNotification(ctx, n); err != nil {
		return SendResult{}, err
	}
	if !s.enabled {
		return SendResult{}, nil
	}
	tokens, err := s.st.AllDeviceTokens(ctx, "")
	if err != nil {
		return SendResult{}, err
	}
	return s.deliver(ctx, tokens, title, body, articleID)
}

// SendToUser stores a personal notification and pushes it to the user's devices.
func (s *Service) SendToUser(ctx context.Context, userID, title, body, articleID, kind string) (SendResult, error) {
	n := model.Notification{
		ID:     platform.NewID("n"),
		UserID: &userID,
		Kind:   defaultKind(kind),
		Title:  title,
		Body:   body,
	}
	if articleID != "" {
		n.ArticleID = &articleID
	}
	if err := s.st.CreateNotification(ctx, n); err != nil {
		return SendResult{}, err
	}
	if !s.enabled {
		return SendResult{}, nil
	}
	devices, err := s.st.ListDevices(ctx, userID)
	if err != nil {
		return SendResult{}, err
	}
	tokens := make([]string, 0, len(devices))
	for _, d := range devices {
		tokens = append(tokens, d.Token)
	}
	return s.deliver(ctx, tokens, title, body, articleID)
}

func (s *Service) deliver(ctx context.Context, tokens []string, title, body, articleID string) (SendResult, error) {
	if len(tokens) == 0 {
		return SendResult{}, nil
	}
	res, err := s.sender.Send(ctx, tokens, Message{
		Title:     title,
		Body:      body,
		ArticleID: articleID,
		Data:      map[string]string{"kind": "news", "articleId": articleID},
	})
	if err != nil {
		return res, err
	}
	for _, token := range res.Invalid {
		if err := s.st.DisableDevice(ctx, token); err != nil {
			s.log.Warn("disable invalid device", "error", err)
		}
	}
	return res, nil
}

// BroadcastTopNews sends a throttled "breaking news" alert about the hottest
// recent article. It returns sent=false when there is nothing worth sending or
// the throttle window has not elapsed.
func (s *Service) BroadcastTopNews(ctx context.Context, minScore float64, minInterval time.Duration) (SendResult, bool, error) {
	if last, err := s.st.GetSetting(ctx, "last_news_broadcast"); err == nil {
		if t, parseErr := time.Parse(time.RFC3339Nano, last); parseErr == nil && time.Since(t) < minInterval {
			return SendResult{}, false, nil
		}
	}
	items, err := s.st.ArticlePublishedAfter(ctx, time.Now().UTC().Add(-3*time.Hour), 10)
	if err != nil {
		return SendResult{}, false, err
	}
	var chosen *model.LocalizedArticle
	for i := range items {
		if items[i].HotScore >= minScore {
			chosen = &items[i]
			break
		}
	}
	if chosen == nil {
		return SendResult{}, false, nil
	}
	res, err := s.Broadcast(ctx, "Главное сейчас", chosen.DisplayTitle, chosen.ID, "breaking")
	if err != nil {
		return res, false, err
	}
	_ = s.st.SetSetting(ctx, "last_news_broadcast", time.Now().UTC().Format(time.RFC3339Nano))
	return res, true, nil
}

func defaultKind(kind string) string {
	if kind == "" {
		return "broadcast"
	}
	return kind
}
