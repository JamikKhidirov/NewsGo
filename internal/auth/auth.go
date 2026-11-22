// Package auth implements email/password authentication with rotating refresh
// tokens and JWT access tokens.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/platform"
	"github.com/gonews/gonews/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrValidation         = errors.New("validation error")
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Claims are the JWT access token claims.
type Claims struct {
	Role string `json:"role"`
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

// TokenPair is returned by register/login/refresh.
type TokenPair struct {
	AccessToken  string     `json:"accessToken"`
	RefreshToken string     `json:"refreshToken"`
	TokenType    string     `json:"tokenType"`
	ExpiresIn    int        `json:"expiresIn"`
	User         model.User `json:"user"`
}

// Service provides authentication operations.
type Service struct {
	st         *store.Store
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
	roleFor    func(email string) string
}

// NewService wires the auth service.
func NewService(st *store.Store, secret string, accessTTL, refreshTTL time.Duration, roleFor func(string) string) *Service {
	if accessTTL <= 0 {
		accessTTL = 30 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	if roleFor == nil {
		roleFor = func(string) string { return "user" }
	}
	return &Service{st: st, secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, issuer: "gonews", roleFor: roleFor}
}

// Register creates a user and returns tokens.
func (s *Service) Register(ctx context.Context, email, password, displayName, userAgent string) (*TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if !emailRe.MatchString(email) {
		return nil, errors.New("invalid email")
	}
	if len(password) < 8 {
		return nil, errors.New("password must contain at least 8 characters")
	}
	if len(password) > 72 {
		return nil, errors.New("password is too long (max 72 bytes)")
	}
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	if len([]rune(displayName)) < 2 || len([]rune(displayName)) > 50 {
		return nil, errors.New("display name must contain 2-50 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := model.User{
		ID:          platform.NewID("u"),
		Email:       email,
		DisplayName: displayName,
		Role:        s.roleFor(email),
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.st.UpsertUser(ctx, u, string(hash)); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return s.issue(ctx, u, userAgent)
}

// Login verifies credentials and returns tokens.
func (s *Service) Login(ctx context.Context, email, password, userAgent string) (*TokenPair, error) {
	u, hash, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issue(ctx, *u, userAgent)
}

// Refresh rotates a refresh token and returns a new pair.
func (s *Service) Refresh(ctx context.Context, refreshToken, userAgent string) (*TokenPair, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, ErrUnauthorized
	}
	oldHash := hashToken(refreshToken)
	newID := platform.NewID("rt")
	newToken := platform.NewToken(32)
	userID, err := s.st.RotateRefreshToken(ctx, oldHash, newID, hashToken(newToken), userAgent, time.Now().UTC().Add(s.refreshTTL))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	access, expiresIn, err := s.sign(*u)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: newToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         *u,
	}, nil
}

// ChangePassword verifies the current password and stores a new one.
func (s *Service) ChangePassword(ctx context.Context, user *model.User, current, next string) error {
	if user == nil {
		return ErrUnauthorized
	}
	_, hash, err := s.st.GetUserByEmail(ctx, user.Email)
	if err != nil {
		return ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return ErrInvalidCredentials
	}
	if len(next) < 8 {
		return errors.New("new password must contain at least 8 characters")
	}
	if len(next) > 72 {
		return errors.New("new password is too long (max 72 bytes)")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.st.UpdatePassword(ctx, user.ID, string(newHash))
}

// Logout revokes a refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.st.RevokeRefreshToken(ctx, hashToken(refreshToken))
}

// ParseAccess validates an access token.
func (s *Service) ParseAccess(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnauthorized
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Type != "access" || claims.Subject == "" {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

func (s *Service) issue(ctx context.Context, u model.User, userAgent string) (*TokenPair, error) {
	access, expiresIn, err := s.sign(u)
	if err != nil {
		return nil, err
	}
	refresh := platform.NewToken(32)
	if err := s.st.SaveRefreshToken(ctx, platform.NewID("rt"), u.ID, hashToken(refresh), truncateUA(userAgent), time.Now().UTC().Add(s.refreshTTL)); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User:         u,
	}, nil
}

func (s *Service) sign(u model.User) (string, int, error) {
	now := time.Now().UTC()
	claims := Claims{
		Role: u.Role,
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, int(s.accessTTL.Seconds()), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func truncateUA(ua string) string {
	if len(ua) > 300 {
		return ua[:300]
	}
	return ua
}
