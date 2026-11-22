package httpapi

import (
	"encoding/json"
	"time"

	"github.com/gonews/gonews/internal/model"
)

type sourceDTO struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Homepage string `json:"homepage"`
	Kind     string `json:"kind"`
	Lang     string `json:"lang"`
}

type tagDTO struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	NameRU string `json:"nameRu"`
	NameEN string `json:"nameEn"`
}

type articleStatsDTO struct {
	Likes    int `json:"likes"`
	Comments int `json:"comments"`
	Views    int `json:"views"`
}

type translationDTO struct {
	Status   string `json:"status"`
	Provider string `json:"provider,omitempty"`
}

type articleDTO struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Content      string          `json:"content,omitempty"`
	URL          string          `json:"url"`
	ImageURL     string          `json:"imageUrl,omitempty"`
	Author       string          `json:"author,omitempty"`
	Lang         string          `json:"lang"`
	OriginalLang string          `json:"originalLang"`
	Category     string          `json:"category"`
	Source       *sourceDTO      `json:"source,omitempty"`
	Tags         []tagDTO        `json:"tags"`
	PublishedAt  time.Time       `json:"publishedAt"`
	FetchedAt    time.Time       `json:"fetchedAt"`
	Stats        articleStatsDTO `json:"stats"`
	Liked        bool            `json:"liked"`
	Bookmarked   bool            `json:"bookmarked"`
	Translation  translationDTO  `json:"translation"`
	Related      []articleDTO    `json:"related,omitempty"`
}

type metaDTO struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type listResponse struct {
	Data any     `json:"data"`
	Meta metaDTO `json:"meta"`
}

type dataResponse struct {
	Data any `json:"data"`
}

func toSourceDTO(src *model.Source) *sourceDTO {
	if src == nil {
		return nil
	}
	return &sourceDTO{ID: src.ID, Slug: src.Slug, Name: src.Name, Homepage: src.Homepage, Kind: src.Kind, Lang: src.Lang}
}

func tagName(t model.Tag, lang string) string {
	if lang == "en" && t.NameEN != "" {
		return t.NameEN
	}
	if t.NameRU != "" {
		return t.NameRU
	}
	return t.NameEN
}

func toTagDTO(t model.Tag, lang string) tagDTO {
	return tagDTO{Slug: t.Slug, Name: tagName(t, lang), NameRU: t.NameRU, NameEN: t.NameEN}
}

func toTagDTOs(tags []model.Tag, lang string) []tagDTO {
	out := make([]tagDTO, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagDTO(t, lang))
	}
	return out
}

func toArticleDTO(a model.LocalizedArticle, lang string, withContent bool) articleDTO {
	dto := articleDTO{
		ID:           a.ID,
		Title:        a.DisplayTitle,
		Summary:      a.DisplaySummary,
		URL:          a.URL,
		ImageURL:     a.ImageURL,
		Author:       a.Author,
		Lang:         lang,
		OriginalLang: a.Lang,
		Category:     a.Category,
		Source:       toSourceDTO(a.Source),
		Tags:         toTagDTOs(a.Tags, lang),
		PublishedAt:  a.PublishedAt,
		FetchedAt:    a.FetchedAt,
		Stats: articleStatsDTO{
			Likes:    a.LikesCount,
			Comments: a.CommentsCount,
			Views:    a.ViewsCount,
		},
		Liked:       a.Liked,
		Bookmarked:  a.Bookmarked,
		Translation: translationDTO{Status: a.TranslationStatus, Provider: a.TranslationProvider},
	}
	if withContent {
		dto.Content = a.DisplayContent
	}
	return dto
}

func toArticleDTOs(items []model.LocalizedArticle, lang string, withContent bool) []articleDTO {
	out := make([]articleDTO, 0, len(items))
	for _, a := range items {
		out = append(out, toArticleDTO(a, lang, withContent))
	}
	return out
}

func newMeta(page, limit, total int) metaDTO {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	return metaDTO{Page: page, Limit: limit, Total: total, TotalPages: pages}
}

type notificationDTO struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data,omitempty"`
	ArticleID *string         `json:"articleId,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Read      bool            `json:"read"`
}

func toNotificationDTO(n model.Notification) notificationDTO {
	dto := notificationDTO{
		ID:        n.ID,
		Kind:      n.Kind,
		Title:     n.Title,
		Body:      n.Body,
		ArticleID: n.ArticleID,
		CreatedAt: n.CreatedAt,
		Read:      n.Read,
	}
	if n.Data != "" && json.Valid([]byte(n.Data)) {
		dto.Data = json.RawMessage(n.Data)
	}
	return dto
}
