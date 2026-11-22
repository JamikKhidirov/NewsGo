package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonews/gonews/internal/auth"
	"github.com/gonews/gonews/internal/store"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return auth.NewService(st, "test-secret", time.Minute, time.Hour, func(email string) string {
		if email == "admin@example.com" {
			return "admin"
		}
		return "user"
	})
}

func TestRegisterLoginRefresh(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	pair, err := svc.Register(ctx, "User@Example.com", "password123", "Test User", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("tokens must be issued")
	}
	if pair.User.Email != "user@example.com" {
		t.Fatalf("email must be normalized, got %q", pair.User.Email)
	}
	if pair.User.Role != "user" {
		t.Fatalf("unexpected role %q", pair.User.Role)
	}

	claims, err := svc.ParseAccess(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != pair.User.ID || claims.Role != "user" {
		t.Fatalf("unexpected claims %#v", claims)
	}

	if _, err := svc.Register(ctx, "user@example.com", "password123", "", ""); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if _, err := svc.Login(ctx, "user@example.com", "wrong-password", ""); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}

	refreshed, err := svc.Refresh(ctx, pair.RefreshToken, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == pair.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := svc.Refresh(ctx, pair.RefreshToken, "test-agent"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("old refresh token must be rejected")
	}

	if err := svc.ChangePassword(ctx, &refreshed.User, "password123", "newpassword123"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(ctx, "user@example.com", "newpassword123", ""); err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, refreshed.RefreshToken, "test-agent"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("logged out refresh token must be rejected")
	}
}

func TestAdminRole(t *testing.T) {
	svc := newService(t)
	pair, err := svc.Register(context.Background(), "admin@example.com", "password123", "Admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if pair.User.Role != "admin" {
		t.Fatalf("expected admin role, got %q", pair.User.Role)
	}
}

func TestValidation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	cases := []struct{ email, password, name string }{
		{"bad-email", "password123", ""},
		{"ok@example.com", "short", ""},
		{"ok2@example.com", "password123", "x"},
	}
	for _, c := range cases {
		if _, err := svc.Register(ctx, c.email, c.password, c.name, ""); err == nil {
			t.Fatalf("expected validation error for %+v", c)
		}
	}
}
