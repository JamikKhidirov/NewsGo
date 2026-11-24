package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// NewAPNsSender builds the Apple Push Notification service sender. Without a
// .p8 key it returns a logging sender (dry-run).
func NewAPNsSender(keyFile, keyID, teamID, topic string, sandbox bool, log *slog.Logger) (Sender, error) {
	if log == nil {
		log = slog.Default()
	}
	if keyFile == "" || keyID == "" || teamID == "" {
		log.Info("APNs is not configured; iOS push messages will be logged only")
		return &logSender{log: log, platform: "ios"}, nil
	}
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read APNs key: %w", err)
	}
	key, err := jwt.ParseECPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs key: %w", err)
	}
	if topic == "" {
		topic = "com.gonews.app"
	}
	host := "https://api.push.apple.com"
	if sandbox {
		host = "https://api.sandbox.push.apple.com"
	}
	return &apnsSender{
		key:    key,
		keyID:  keyID,
		teamID: teamID,
		topic:  topic,
		host:   host,
		client: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				MaxIdleConns:      32,
				IdleConnTimeout:   90 * time.Second,
			},
		},
		log: log,
	}, nil
}

type apnsSender struct {
	key    *ecdsa.PrivateKey
	keyID  string
	teamID string
	topic  string
	host   string
	client *http.Client
	log    *slog.Logger

	mu      sync.Mutex
	token   string
	expires time.Time
}

func (s *apnsSender) jwt() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expires.Add(-2*time.Minute)) {
		return s.token, nil
	}
	now := time.Now()
	claims := jwt.MapClaims{"iss": s.teamID, "iat": now.Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = s.keyID
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", err
	}
	s.token = signed
	s.expires = now.Add(50 * time.Minute)
	return signed, nil
}

func (s *apnsSender) Send(ctx context.Context, tokens []string, msg Message) (SendResult, error) {
	result := SendResult{Targets: len(tokens)}
	if len(tokens) == 0 {
		return result, nil
	}
	token, err := s.jwt()
	if err != nil {
		return result, err
	}

	aps := map[string]any{
		"alert": map[string]string{"title": msg.Title, "body": msg.Body},
		"sound": "default",
	}
	payload := map[string]any{"aps": aps}
	for k, v := range msg.Data {
		payload[k] = v
	}
	if msg.ArticleID != "" {
		payload["articleId"] = msg.ArticleID
	}
	body, _ := json.Marshal(payload)

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, device := range tokens {
		device := device
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			url := s.host + "/3/device/" + device
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			req.Header.Set("Authorization", "bearer "+token)
			req.Header.Set("apns-topic", s.topic)
			req.Header.Set("apns-push-type", "alert")
			req.Header.Set("apns-priority", "10")
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.client.Do(req)
			if err != nil {
				s.log.Warn("APNs send failed", "error", err)
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			mu.Lock()
			defer mu.Unlock()
			if resp.StatusCode == http.StatusOK {
				result.Sent++
				return
			}
			result.Failed++
			reason := strings.TrimSpace(string(respBody))
			s.log.Warn("APNs rejected token", "status", resp.StatusCode, "reason", reason)
			if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusBadRequest {
				result.Invalid = append(result.Invalid, device)
			}
		}()
	}
	wg.Wait()
	return result, nil
}

// errAPNsDisabled is kept for callers that want to distinguish dry-run mode.
var errAPNsDisabled = errors.New("apns is not configured")
