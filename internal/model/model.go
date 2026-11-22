// Package model contains the domain entities shared by all layers.
package model

import "time"

type Source struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Homepage string `json:"homepage"`
	Kind     string `json:"kind"`
	Lang     string `json:"lang"`
	Enabled  bool   `json:"enabled"`
}

type Article struct {
	ID            string
	SourceID      string
	ExternalID    string
	URL           string
	CanonicalURL  string
	Title         string
	Summary       string
	Content       string
	ImageURL      string
	Author        string
	Lang          string
	Category      string
	PublishedAt   time.Time
	FetchedAt     time.Time
	HotScore      float64
	LikesCount    int
	CommentsCount int
	ViewsCount    int
	UniqueHash    string
}

// LocalizedArticle is an article prepared for a specific UI language.
type LocalizedArticle struct {
	Article
	DisplayTitle        string
	DisplaySummary      string
	DisplayContent      string
	TranslationStatus   string // original | ready | pending | failed
	TranslationProvider string
	Liked               bool
	Bookmarked          bool
	Source              *Source
	Tags                []Tag
}

type Tag struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	NameRU string `json:"nameRu"`
	NameEN string `json:"nameEn"`
}

type TagCount struct {
	Tag
	Count int `json:"count"`
}

type TranslationJob struct {
	ID         int64
	ArticleID  string
	TargetLang string
	Attempts   int
}

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Comment struct {
	ID         string    `json:"id"`
	ArticleID  string    `json:"articleId"`
	UserID     string    `json:"userId"`
	ParentID   *string   `json:"parentId,omitempty"`
	Body       string    `json:"body"`
	AuthorName string    `json:"authorName"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Deleted    bool      `json:"deleted"`
}

type Notification struct {
	ID        string    `json:"id"`
	UserID    *string   `json:"userId,omitempty"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Data      string    `json:"data,omitempty"`
	ArticleID *string   `json:"articleId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Read      bool      `json:"read"`
}

type Device struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"`
	Locale     string    `json:"locale"`
	Topics     []string  `json:"topics"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type IngestRun struct {
	ID         int64     `json:"id"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Status     string    `json:"status"`
	Fetched    int       `json:"fetched"`
	Inserted   int       `json:"inserted"`
	Updated    int       `json:"updated"`
	Errors     int       `json:"errors"`
	Details    string    `json:"details"`
}

// IngestItem is a normalized news item produced by an aggregator provider.
type IngestItem struct {
	SourceSlug  string
	ExternalID  string
	URL         string
	Title       string
	Summary     string
	Content     string
	ImageURL    string
	Author      string
	Lang        string
	Category    string
	PublishedAt time.Time
	Score       float64
}
