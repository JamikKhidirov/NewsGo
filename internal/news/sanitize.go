package news

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	reScript    = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>|<style[^>]*>.*?</style>|<noscript[^>]*>.*?</noscript>`)
	reTags      = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces    = regexp.MustCompile(`\s+`)
	reImgSrc    = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)
	trackingKey = map[string]bool{
		"utm_source": true, "utm_medium": true, "utm_campaign": true, "utm_term": true,
		"utm_content": true, "fbclid": true, "gclid": true, "yclid": true,
		"ref": true, "referrer": true, "source": true, "mc_cid": true, "mc_eid": true,
		"igshid": true, "s_kwcid": true, "spm": true, "_ga": true,
	}
)

// StripHTML removes tags/scripts and decodes entities.
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	s = reScript.ReplaceAllString(s, " ")
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

// Truncate limits a string by runes without breaking UTF-8.
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max])) + "…"
}

// CanonicalURL normalizes a URL for deduplication.
func CanonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return raw
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		if trackingKey[strings.ToLower(key)] || strings.HasPrefix(strings.ToLower(key), "utm_") {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	return u.String()
}

// URLHash returns a stable SHA-256 hex digest for an article URL.
func URLHash(canonical string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(canonical)))
	return hex.EncodeToString(sum[:])
}

// ArticleID builds the deterministic primary key from the dedup hash.
func ArticleID(hash string) string {
	if len(hash) > 20 {
		hash = hash[:20]
	}
	return "a_" + hash
}

// SourceID builds a deterministic source id.
func SourceID(slug string) string {
	sum := sha256.Sum256([]byte(slug))
	return "s_" + hex.EncodeToString(sum[:])[:12]
}

// TagID builds a deterministic tag id.
func TagID(slug string) string {
	sum := sha256.Sum256([]byte(slug))
	return "t_" + hex.EncodeToString(sum[:])[:12]
}

// FirstImage extracts an image URL from raw HTML or enclosures.
func FirstImage(htmlBody string, candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(html.UnescapeString(c))
		if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
			return c
		}
	}
	if m := reImgSrc.FindStringSubmatch(htmlBody); len(m) == 2 {
		c := html.UnescapeString(m[1])
		if strings.HasPrefix(c, "http") {
			return c
		}
	}
	return ""
}

// CleanTitle collapses whitespace and strips a trailing source suffix.
func CleanTitle(s string) string {
	s = StripHTML(s)
	s = strings.TrimSpace(s)
	return Truncate(s, 300)
}
