package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gonews/gonews/internal/auth"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	pair, err := a.auth.Register(r.Context(), req.Email, req.Password, req.DisplayName, r.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken", "email is already registered")
		default:
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		}
		return
	}
	writeData(w, http.StatusCreated, pair)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	pair, err := a.auth.Login(r.Context(), req.Email, req.Password, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "login failed")
		return
	}
	writeData(w, http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	pair, err := a.auth.Refresh(r.Context(), req.RefreshToken, r.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "refresh failed")
		return
	}
	writeData(w, http.StatusOK, pair)
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if err := a.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	stats, err := a.st.GetUserStats(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"user": u, "stats": stats})
}

type updateMeRequest struct {
	DisplayName string `json:"displayName"`
}

func (a *API) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	var req updateMeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if len([]rune(name)) < 2 || len([]rune(name)) > 50 {
		writeError(w, http.StatusBadRequest, "validation_error", "display name must contain 2-50 characters")
		return
	}
	if err := a.st.UpdateUserProfile(r.Context(), u.ID, name); err != nil {
		writeStoreError(w, err)
		return
	}
	u.DisplayName = name
	writeData(w, http.StatusOK, u)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if err := a.auth.ChangePassword(r.Context(), u, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "current password is incorrect")
			return
		}
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"updated": true})
}

func (a *API) handleMeStats(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	stats, err := a.st.GetUserStats(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, stats)
}
