package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gonews/gonews/internal/model"
	"github.com/gonews/gonews/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seed(t *testing.T, st *store.Store) (model.Source, model.Tag) {
	t.Helper()
	ctx := context.Background()
	src := model.Source{ID: "s_test", Slug: "test", Name: "Test", Kind: "rss", Lang: "en", Enabled: true}
	if err := st.UpsertSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	tag := model.Tag{ID: "t_tech", Slug: "technology", NameRU: "Технологии", NameEN: "Technology"}
	if err := st.EnsureTags(ctx, []model.Tag{tag}); err != nil {
		t.Fatal(err)
	}
	return src, tag
}

func makeArticle(src model.Source) model.Article {
	now := time.Now().UTC()
	return model.Article{
		ID:           "a_testarticle00000001",
		SourceID:     src.ID,
		ExternalID:   "ext-1",
		URL:          "https://example.com/news/1",
		CanonicalURL: "https://example.com/news/1",
		Title:        "A new breakthrough in battery technology",
		Summary:      "Researchers presented a new battery.",
		Content:      "Researchers presented a new battery with higher density.",
		ImageURL:     "https://example.com/1.jpg",
		Author:       "Jane Doe",
		Lang:         "en",
		Category:     "technology",
		PublishedAt:  now.Add(-time.Hour),
		FetchedAt:    now,
		HotScore:     42,
		UniqueHash:   "hash-1",
	}
}

func TestArticleLifecycle(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src, tag := seed(t, st)

	article := makeArticle(src)
	id, inserted, err := st.UpsertArticle(ctx, article, []model.Tag{tag}, []string{"ru"})
	if err != nil {
		t.Fatal(err)
	}
	if !inserted || id != article.ID {
		t.Fatalf("expected insert, got inserted=%v id=%s", inserted, id)
	}

	_, inserted, err = st.UpsertArticle(ctx, article, []model.Tag{tag}, []string{"ru"})
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("second upsert must update, not insert")
	}

	got, err := st.GetArticle(ctx, id, "ru", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.TranslationStatus != "pending" {
		t.Fatalf("expected pending translation, got %q", got.TranslationStatus)
	}
	if got.DisplayTitle != article.Title {
		t.Fatal("original title must be returned while translation is pending")
	}

	if err := st.SaveTranslation(ctx, id, "ru", "Новый прорыв в технологии аккумуляторов", "Краткое описание", "Полный текст", "google"); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetArticle(ctx, id, "ru", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.TranslationStatus != "ready" || got.DisplayTitle != "Новый прорыв в технологии аккумуляторов" {
		t.Fatalf("unexpected localized article: %#v", got)
	}

	items, total, err := st.ListArticles(ctx, store.ArticleFilter{Lang: "ru", Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 article, got total=%d len=%d", total, len(items))
	}
	if len(items[0].Tags) != 1 || items[0].Tags[0].Slug != "technology" {
		t.Fatalf("tags not attached: %#v", items[0].Tags)
	}

	items, total, err = st.ListArticles(ctx, store.ArticleFilter{Lang: "ru", Query: "аккумулятор", Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatal("search over translations failed")
	}
}

func TestSocialInteractions(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	src, tag := seed(t, st)
	article := makeArticle(src)
	id, _, err := st.UpsertArticle(ctx, article, []model.Tag{tag}, nil)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "u_test", Email: "u@example.com", DisplayName: "User", Role: "user", CreatedAt: time.Now().UTC()}
	if err := st.UpsertUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}

	likes, err := st.LikeArticle(ctx, user.ID, id)
	if err != nil || likes != 1 {
		t.Fatalf("like failed: %d %v", likes, err)
	}
	likes, err = st.LikeArticle(ctx, user.ID, id)
	if err != nil || likes != 1 {
		t.Fatal("like must be idempotent")
	}
	likes, err = st.UnlikeArticle(ctx, user.ID, id)
	if err != nil || likes != 0 {
		t.Fatal("unlike failed")
	}

	if err := st.AddBookmark(ctx, user.ID, id); err != nil {
		t.Fatal(err)
	}
	bookmarked, err := st.IsBookmarked(ctx, user.ID, id)
	if err != nil || !bookmarked {
		t.Fatal("bookmark failed")
	}

	comment, err := st.CreateComment(ctx, model.Comment{ID: "c_1", ArticleID: id, UserID: user.ID, Body: "Nice", AuthorName: "User"})
	if err != nil {
		t.Fatal(err)
	}
	if comment.CreatedAt.IsZero() {
		t.Fatal("comment timestamp missing")
	}
	comments, total, err := st.ListComments(ctx, id, 1, 10)
	if err != nil || total != 1 || len(comments) != 1 {
		t.Fatalf("list comments: %d %d %v", len(comments), total, err)
	}
	if err := st.DeleteComment(ctx, "c_1", user.ID, false); err != nil {
		t.Fatal(err)
	}
	comments, total, err = st.ListComments(ctx, id, 1, 10)
	if err != nil || total != 0 || !comments[0].Deleted {
		t.Fatal("soft delete failed")
	}

	if err := st.FollowTag(ctx, user.ID, tag.ID); err != nil {
		t.Fatal(err)
	}
	interests, err := st.FeedTagIDs(ctx, user.ID)
	if err != nil || len(interests) == 0 {
		t.Fatal("feed interests missing")
	}

	items, total, err := st.ListArticles(ctx, store.ArticleFilter{Lang: "ru", UserID: user.ID, Bookmarked: true, Page: 1, Limit: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("bookmarks list failed: %d %v", total, err)
	}
	if !items[0].Bookmarked {
		t.Fatal("bookmark flag missing")
	}
}

func TestNotifications(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	user := model.User{ID: "u_notify", Email: "n@example.com", DisplayName: "N", Role: "user", CreatedAt: time.Now().UTC()}
	if err := st.UpsertUser(ctx, user, "hash"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateNotification(ctx, model.Notification{ID: "n_1", Title: "Hello", Body: "World", Kind: "broadcast"}); err != nil {
		t.Fatal(err)
	}
	items, total, err := st.ListNotifications(ctx, user.ID, 1, 10, false)
	if err != nil || total != 1 || len(items) != 1 || items[0].Read {
		t.Fatalf("notifications: %d %v", total, err)
	}
	if err := st.MarkNotificationRead(ctx, user.ID, "n_1"); err != nil {
		t.Fatal(err)
	}
	unread, err := st.UnreadNotificationCount(ctx, user.ID)
	if err != nil || unread != 0 {
		t.Fatalf("unread count: %d %v", unread, err)
	}
	if _, err := st.MarkAllNotificationsRead(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNotFound(t *testing.T) {
	st := newStore(t)
	if _, err := st.GetArticle(context.Background(), "missing", "ru", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
