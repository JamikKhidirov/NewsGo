package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Message is a push payload.
type Message struct {
	Title     string
	Body      string
	Data      map[string]string
	ArticleID string
}

// SendResult reports delivery attempts.
type SendResult struct {
	Targets int      `json:"targets"`
	Sent    int      `json:"sent"`
	Failed  int      `json:"failed"`
	Invalid []string `json:"-"`
}

// Sender delivers a message to device tokens.
type Sender interface {
	Send(ctx context.Context, tokens []string, msg Message) (SendResult, error)
}

// NewSender builds the configured sender. Without FCM credentials a logging
// sender is used so local development still works end-to-end.
func NewSender(projectID, serviceAccountFile string, log *slog.Logger) (Sender, error) {
	if projectID == "" || serviceAccountFile == "" {
		log.Info("FCM is not configured; push messages will be logged only")
		return &logSender{log: log}, nil
	}
	sa, err := loadServiceAccount(serviceAccountFile)
	if err != nil {
		return nil, err
	}
	if sa.ProjectID == "" {
		sa.ProjectID = projectID
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &fcmSender{
		projectID: sa.ProjectID,
		sa:        sa,
		client:    &http.Client{Timeout: 20 * time.Second},
		log:       log,
	}, nil
}

type logSender struct{ log *slog.Logger }

func (s *logSender) Send(ctx context.Context, tokens []string, msg Message) (SendResult, error) {
	s.log.Info("push (dry-run)", "title", msg.Title, "body", msg.Body, "targets", len(tokens), "data", msg.Data)
	return SendResult{Targets: len(tokens), Sent: len(tokens)}, nil
}

// --- FCM HTTP v1 ---

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type fcmSender struct {
	projectID string
	sa        serviceAccount
	client    *http.Client
	log       *slog.Logger

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func loadServiceAccount(path string) (serviceAccount, error) {
	var sa serviceAccount
	body, err := os.ReadFile(path)
	if err != nil {
		return sa, fmt.Errorf("read FCM service account: %w", err)
	}
	if err := json.Unmarshal(body, &sa); err != nil {
		return sa, fmt.Errorf("parse FCM service account: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return sa, errors.New("FCM service account is missing client_email/private_key")
	}
	return sa, nil
}

func (s *fcmSender) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry.Add(-60*time.Second)) {
		return s.accessToken, nil
	}

	now := time.Now().Unix()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   s.sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   s.sa.TokenURI,
		"iat":   now,
		"exp":   now + 3600,
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)

	block, _ := pem.Decode([]byte(s.sa.PrivateKey))
	if block == nil {
		return "", errors.New("FCM private key is not valid PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("FCM private key is not RSA")
		}
		key = rk
	} else {
		rk, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return "", fmt.Errorf("parse FCM private key: %w", err)
		}
		key = rk
	}

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)

	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + assertion
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.sa.TokenURI, strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("FCM token endpoint: http %d: %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", errors.New("FCM token endpoint returned an empty token")
	}
	s.accessToken = tok.AccessToken
	expires := tok.ExpiresIn
	if expires <= 0 {
		expires = 3600
	}
	s.tokenExpiry = time.Now().Add(time.Duration(expires) * time.Second)
	return s.accessToken, nil
}

func (s *fcmSender) Send(ctx context.Context, tokens []string, msg Message) (SendResult, error) {
	result := SendResult{Targets: len(tokens)}
	if len(tokens) == 0 {
		return result, nil
	}
	access, err := s.token(ctx)
	if err != nil {
		return result, err
	}
	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)
	data := map[string]string{}
	for k, v := range msg.Data {
		data[k] = v
	}
	if msg.ArticleID != "" {
		data["articleId"] = msg.ArticleID
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, token := range tokens {
		token := token
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			payload := map[string]any{
				"message": map[string]any{
					"token":        token,
					"notification": map[string]string{"title": msg.Title, "body": msg.Body},
					"data":         data,
					"android": map[string]any{
						"priority":     "HIGH",
						"notification": map[string]any{"channel_id": "news", "sound": "default"},
					},
					"apns": map[string]any{
						"payload": map[string]any{"aps": map[string]any{"sound": "default", "content-available": 1}},
						"headers": map[string]string{"apns-priority": "10"},
					},
				},
			}
			body, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err != nil {
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			req.Header.Set("Authorization", "Bearer "+access)
			req.Header.Set("Content-Type", "application/json")
			resp, err := s.client.Do(req)
			if err != nil {
				s.log.Warn("FCM send failed", "error", err)
				mu.Lock()
				result.Failed++
				mu.Unlock()
				return
			}
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			mu.Lock()
			defer mu.Unlock()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				result.Sent++
				return
			}
			result.Failed++
			s.log.Warn("FCM rejected token", "status", resp.StatusCode, "body", strings.TrimSpace(string(respBody)))
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
				result.Invalid = append(result.Invalid, token)
			}
		}()
	}
	wg.Wait()
	return result, nil
}
