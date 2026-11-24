package push

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/gonews/gonews/internal/metrics"
	"github.com/gonews/gonews/internal/model"
)

// Delivery routes messages to the platform-specific transports.
type Delivery struct {
	fcm  Sender
	apns Sender
	log  *slog.Logger
}

// NewDelivery builds FCM (Android) and APNs (iOS) senders. Missing credentials
// fall back to a logging sender, which keeps the whole pipeline testable.
func NewDelivery(projectID, serviceAccountFile, apnsKeyFile, apnsKeyID, apnsTeamID, apnsTopic string, apnsSandbox bool, log *slog.Logger) (*Delivery, error) {
	if log == nil {
		log = slog.Default()
	}
	fcm, err := NewSender(projectID, serviceAccountFile, log)
	if err != nil {
		return nil, err
	}
	apns, err := NewAPNsSender(apnsKeyFile, apnsKeyID, apnsTeamID, apnsTopic, apnsSandbox, log)
	if err != nil {
		return nil, err
	}
	return &Delivery{fcm: fcm, apns: apns, log: log}, nil
}

// Send delivers to devices, splitting them by platform.
func (d *Delivery) Send(ctx context.Context, devices []model.Device, msg Message) (SendResult, error) {
	var android, ios []string
	for _, dev := range devices {
		if strings.TrimSpace(dev.Token) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(dev.Platform)) {
		case "ios", "apns", "apple":
			ios = append(ios, dev.Token)
		default:
			android = append(android, dev.Token)
		}
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		combined SendResult
		firstErr error
	)
	run := func(sender Sender, tokens []string, platform string) {
		if len(tokens) == 0 || sender == nil {
			return
		}
		defer wg.Done()
		res, err := sender.Send(ctx, tokens, msg)
		mu.Lock()
		defer mu.Unlock()
		combined.Targets += res.Targets
		combined.Sent += res.Sent
		combined.Failed += res.Failed
		combined.Invalid = append(combined.Invalid, res.Invalid...)
		metrics.PushDeliveries.WithLabelValues(platform, "sent").Add(float64(res.Sent))
		if res.Failed > 0 {
			metrics.PushDeliveries.WithLabelValues(platform, "failed").Add(float64(res.Failed))
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if len(android) > 0 {
		wg.Add(1)
		go run(d.fcm, android, "android")
	}
	if len(ios) > 0 {
		wg.Add(1)
		go run(d.apns, ios, "ios")
	}
	wg.Wait()
	return combined, firstErr
}
