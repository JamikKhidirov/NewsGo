// Package push provides in-app notifications and multi-platform push delivery.
//
// The HTTP layer only persists the notification and publishes a push event.
// Delivery happens asynchronously in a Kafka consumer (or the in-process bus in
// development), so a slow APNs/FCM call never blocks an API request.
package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gonews/gonews/internal/events"
	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/platform"
	"github.com/gonews/gonews/internal/store"
)

// Event is the Kafka payload for one push notification.
type Event struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	ArticleID string    `json:"articleId,omitempty"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service combines notification persistence and event publishing.
type Service struct {
	st       *store.Store
	bus      events.Bus
	delivery *Delivery
	enabled  bool
	topic    string
	log      *slog.Logger
}

// NewService builds the push service.
func NewService(st *store.Store, bus events.Bus, delivery *Delivery, enabled bool, topic string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{st: st, bus: bus, delivery: delivery, enabled: enabled, topic: topic, log: log}
}

// Broadcast stores an in-app broadcast and queues delivery to all devices.
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
	return s.queue(ctx, Event{
		ID: n.ID, Title: title, Body: body, ArticleID: articleID, Kind: n.Kind, CreatedAt: n.CreatedAt,
	})
}

// SendToUser stores a personal notification and queues delivery to the user's devices.
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
	return s.queue(ctx, Event{
		ID: n.ID, UserID: userID, Title: title, Body: body, ArticleID: articleID, Kind: n.Kind, CreatedAt: n.CreatedAt,
	})
}

func (s *Service) queue(ctx context.Context, evt Event) (SendResult, error) {
	payload, err := json.Marshal(evt)
	if err != nil {
		return SendResult{}, err
	}
	if s.bus == nil {
		return s.deliverNow(ctx, evt)
	}
	if err := s.bus.Publish(ctx, s.topic, evt.ID, payload); err != nil {
		// Kafka is unavailable: degrade to synchronous delivery instead of
		// losing the notification.
		s.log.Warn("push event publish failed; delivering synchronously", "error", err)
		return s.deliverNow(ctx, evt)
	}
	return SendResult{Queued: 1}, nil
}

func (s *Service) deliverNow(ctx context.Context, evt Event) (SendResult, error) {
	devices, err := s.devices(ctx, evt.UserID)
	if err != nil {
		return SendResult{}, err
	}
	if len(devices) == 0 {
		return SendResult{}, nil
	}
	res, err := s.delivery.Send(ctx, devices, Message{
		Title:     evt.Title,
		Body:      evt.Body,
		ArticleID: evt.ArticleID,
		Data:      map[string]string{"kind": evt.Kind, "articleId": evt.ArticleID, "notificationId": evt.ID},
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

func (s *Service) devices(ctx context.Context, userID string) ([]model.Device, error) {
	if userID != "" {
		return s.st.ListDevices(ctx, userID)
	}
	return s.st.AllDevices(ctx)
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

// Worker consumes push events and delivers them to FCM/APNs.
type Worker struct {
	st       *store.Store
	delivery *Delivery
	log      *slog.Logger
}

// NewWorker builds the delivery worker.
func NewWorker(st *store.Store, delivery *Delivery, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{st: st, delivery: delivery, log: log}
}

// Start subscribes the worker to the push topic.
func (w *Worker) Start(ctx context.Context, bus events.Bus, topic, group string) {
	if bus == nil {
		return
	}
	bus.Subscribe(ctx, topic, group, w.Handle)
	w.log.Info("push worker started", "bus", bus.Name(), "topic", topic, "group", group)
}

// Handle processes one push event.
func (w *Worker) Handle(ctx context.Context, payload []byte) error {
	var evt Event
	if err := json.Unmarshal(payload, &evt); err != nil {
		return err
	}
	var devices []model.Device
	var err error
	if evt.UserID != "" {
		devices, err = w.st.ListDevices(ctx, evt.UserID)
	} else {
		devices, err = w.st.AllDevices(ctx)
	}
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		w.log.Debug("push event has no devices", "notification_id", evt.ID)
		return nil
	}
	res, err := w.delivery.Send(ctx, devices, Message{
		Title:     evt.Title,
		Body:      evt.Body,
		ArticleID: evt.ArticleID,
		Data:      map[string]string{"kind": evt.Kind, "articleId": evt.ArticleID, "notificationId": evt.ID},
	})
	if err != nil {
		return err
	}
	for _, token := range res.Invalid {
		if err := w.st.DisableDevice(ctx, token); err != nil {
			w.log.Warn("disable invalid device", "error", err)
		}
	}
	w.log.Info("push delivered", "notification_id", evt.ID, "targets", res.Targets, "sent", res.Sent, "failed", res.Failed)
	return nil
}
