package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gonews/gonews/internal/langdetect"
	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/platform"
	"github.com/gonews/gonews/internal/store"
)

// --- comments ---

func (a *API) handleListComments(w http.ResponseWriter, r *http.Request) {
	articleID := chi.URLParam(r, "id")
	page, limit := normalizePage(queryInt(r, "page", 1), queryInt(r, "limit", 30))
	comments, total, err := a.st.ListComments(r.Context(), articleID, page, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if comments == nil {
		comments = []model.Comment{}
	}
	writeList(w, http.StatusOK, comments, newMeta(page, limit, total))
}

type commentRequest struct {
	Body     string  `json:"body"`
	ParentID *string `json:"parentId"`
}

func (a *API) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	articleID := chi.URLParam(r, "id")
	if _, err := a.st.GetArticleRaw(r.Context(), articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	var req commentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := len([]rune(body)); n < 1 || n > 2000 {
		writeError(w, http.StatusBadRequest, "validation_error", "comment must contain 1-2000 characters")
		return
	}
	if req.ParentID != nil && strings.TrimSpace(*req.ParentID) == "" {
		req.ParentID = nil
	}
	comment, err := a.st.CreateComment(r.Context(), model.Comment{
		ID:         platform.NewID("c"),
		ArticleID:  articleID,
		UserID:     u.ID,
		ParentID:   req.ParentID,
		Body:       body,
		AuthorName: u.DisplayName,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_parent", "parent comment not found")
			return
		}
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusCreated, comment)
}

type updateCommentRequest struct {
	Body string `json:"body"`
}

func (a *API) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	var req updateCommentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	body := strings.TrimSpace(req.Body)
	if n := len([]rune(body)); n < 1 || n > 2000 {
		writeError(w, http.StatusBadRequest, "validation_error", "comment must contain 1-2000 characters")
		return
	}
	if err := a.st.UpdateComment(r.Context(), chi.URLParam(r, "id"), u.ID, body); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"updated": true})
}

func (a *API) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := a.st.DeleteComment(r.Context(), chi.URLParam(r, "id"), u.ID, u.Role == "admin"); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- likes / bookmarks / views ---

func (a *API) handleLike(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	articleID := chi.URLParam(r, "id")
	if _, err := a.st.GetArticleRaw(r.Context(), articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	count, err := a.st.LikeArticle(r.Context(), u.ID, articleID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"liked": true, "likesCount": count})
}

func (a *API) handleUnlike(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	articleID := chi.URLParam(r, "id")
	if _, err := a.st.GetArticleRaw(r.Context(), articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	count, err := a.st.UnlikeArticle(r.Context(), u.ID, articleID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"liked": false, "likesCount": count})
}

func (a *API) handleBookmark(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	articleID := chi.URLParam(r, "id")
	if _, err := a.st.GetArticleRaw(r.Context(), articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := a.st.AddBookmark(r.Context(), u.ID, articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"bookmarked": true})
}

func (a *API) handleUnbookmark(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := a.st.RemoveBookmark(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"bookmarked": false})
}

func (a *API) handleView(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	articleID := chi.URLParam(r, "id")
	if _, err := a.st.GetArticleRaw(r.Context(), articleID); err != nil {
		writeStoreError(w, err)
		return
	}
	if err := a.st.IncrementViews(r.Context(), articleID, u.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- personalized lists ---

func (a *API) handleFeed(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	lang := langParam(r)
	tagIDs, err := a.st.FeedTagIDs(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	page, limit := normalizePage(queryInt(r, "page", 1), queryInt(r, "limit", 20))
	f := store.ArticleFilter{
		Lang:   lang,
		TagIDs: tagIDs,
		Page:   page,
		Limit:  limit,
		UserID: u.ID,
	}
	items, total, err := a.st.ListArticles(r.Context(), f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if total == 0 {
		f.TagIDs = nil
		items, total, err = a.st.ListArticles(r.Context(), f)
		if err != nil {
			writeStoreError(w, err)
			return
		}
	}
	writeList(w, http.StatusOK, toArticleDTOs(items, lang, false), newMeta(f.Page, f.Limit, total))
}

func (a *API) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	a.personalList(w, r, store.ArticleFilter{Bookmarked: true})
}

func (a *API) handleLikes(w http.ResponseWriter, r *http.Request) {
	a.personalList(w, r, store.ArticleFilter{Liked: true})
}

func (a *API) handleHistory(w http.ResponseWriter, r *http.Request) {
	a.personalList(w, r, store.ArticleFilter{History: true})
}

func (a *API) personalList(w http.ResponseWriter, r *http.Request, f store.ArticleFilter) {
	u := currentUser(r.Context())
	f.Lang = langParam(r)
	f.Page, f.Limit = normalizePage(queryInt(r, "page", 1), queryInt(r, "limit", 20))
	f.UserID = u.ID
	items, total, err := a.st.ListArticles(r.Context(), f)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeList(w, http.StatusOK, toArticleDTOs(items, f.Lang, false), newMeta(f.Page, f.Limit, total))
}

// --- follows ---

func (a *API) handleListFollows(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	lang := langParam(r)
	tags, err := a.st.ListFollows(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, toTagDTOs(tags, lang))
}

func (a *API) handleFollow(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	tag, err := a.st.GetTagBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := a.st.FollowTag(r.Context(), u.ID, tag.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]any{"following": true, "tag": toTagDTO(*tag, langParam(r))})
}

func (a *API) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	tag, err := a.st.GetTagBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := a.st.UnfollowTag(r.Context(), u.ID, tag.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"following": false})
}

// --- devices ---

type deviceRequest struct {
	Token    string   `json:"token"`
	Platform string   `json:"platform"`
	Locale   string   `json:"locale"`
	Topics   []string `json:"topics"`
}

func (a *API) handleListDevices(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	devices, err := a.st.ListDevices(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if devices == nil {
		devices = []model.Device{}
	}
	writeData(w, http.StatusOK, devices)
}

func (a *API) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	var req deviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) < 10 || len(req.Token) > 4096 {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid device token")
		return
	}
	plat := strings.ToLower(strings.TrimSpace(req.Platform))
	switch plat {
	case "android", "ios", "web":
	case "":
		plat = "android"
	default:
		writeError(w, http.StatusBadRequest, "validation_error", "platform must be android, ios or web")
		return
	}
	locale := langdetect.Normalize(req.Locale)
	if len(locale) != 2 {
		locale = "ru"
	}
	if len(req.Topics) > 20 {
		req.Topics = req.Topics[:20]
	}
	topics := make([]string, 0, len(req.Topics))
	for _, t := range req.Topics {
		t = strings.TrimSpace(t)
		if t != "" && len([]rune(t)) <= 50 {
			topics = append(topics, t)
		}
	}
	d := model.Device{
		ID:       platform.NewID("d"),
		UserID:   u.ID,
		Token:    req.Token,
		Platform: plat,
		Locale:   locale,
		Topics:   topics,
	}
	if err := a.st.UpsertDevice(r.Context(), d); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusCreated, d)
}

func (a *API) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := a.st.DeleteDevice(r.Context(), u.ID, chi.URLParam(r, "token")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- notifications ---

func (a *API) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	unread := r.URL.Query().Get("unread") == "true" || r.URL.Query().Get("unread") == "1"
	page, limit := normalizePage(queryInt(r, "page", 1), queryInt(r, "limit", 30))
	items, total, err := a.st.ListNotifications(r.Context(), u.ID, page, limit, unread)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := make([]notificationDTO, 0, len(items))
	for _, n := range items {
		out = append(out, toNotificationDTO(n))
	}
	writeList(w, http.StatusOK, out, newMeta(page, limit, total))
}

func (a *API) handleUnreadCount(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	n, err := a.st.UnreadNotificationCount(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]int{"unread": n})
}

func (a *API) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	if err := a.st.MarkNotificationRead(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"read": true})
}

func (a *API) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	n, err := a.st.MarkAllNotificationsRead(r.Context(), u.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]int64{"marked": n})
}
