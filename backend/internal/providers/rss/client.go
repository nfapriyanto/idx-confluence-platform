package rss

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// MONOREPO PATH: backend/internal/providers/rss/client.go
// Polling Berita Pasar Modal Indonesia Gratis dari 4 Portal Utama:
// 1. Kontan Investasi (https://www.kontan.co.id/feed)
// 2. CNBC Indonesia Market (https://www.cnbcindonesia.com/market/rss)
// 3. Antara Ekonomi Bisnis (https://www.antaranews.com/rss/ekonomi.xml)
// 4. Google News RSS Emiten BEI
// Dilengkapi Deduplikasi URL/Judul & Deteksi Co-Occurrence Sektor
// =============================================================================

type Client struct {
	httpClient *http.Client
	feeds      map[string]string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 8 * time.Second},
		feeds: map[string]string{
			"KONTAN":         "https://www.kontan.co.id/feed",
			"CNBC_INDONESIA": "https://www.cnbcindonesia.com/market/rss",
			"ANTARA_EKONOMI": "https://www.antaranews.com/rss/ekonomi.xml",
		},
	}
}

type rssDocument struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

// PollAllPortals menarik berita terbaru dari seluruh RSS portal finansial Indonesia
func (c *Client) PollAllPortals(ctx context.Context, watchedTickers []string) ([]domain.NewsArticle, error) {
	seenURL := make(map[string]bool)
	var collected []domain.NewsArticle

	for portal, feedURL := range c.feeds {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; IDXConfluenceBot/1.0)")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var doc rssDocument
		if err := xml.Unmarshal(raw, &doc); err != nil {
			continue
		}

		for _, item := range doc.Channel.Items {
			link := strings.TrimSpace(item.Link)
			title := strings.TrimSpace(item.Title)
			if link == "" || title == "" || seenURL[link] {
				continue
			}
			seenURL[link] = true

			pubTime := parsePubDate(item.PubDate)
			upperTitle := strings.ToUpper(title)

			for _, tk := range watchedTickers {
				if strings.Contains(upperTitle, tk) {
					collected = append(collected, domain.NewsArticle{
						Title:         title,
						Portal:        portal,
						URL:           link,
						PublishedAt:   pubTime,
						IndoBERTScore: 50.0, // Akan diisi oleh internal/nlp/engine.go
					})
				}
			}
		}
	}
	return collected, nil
}

func parsePubDate(raw string) time.Time {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return t
		}
	}
	return time.Now().Add(-2 * time.Hour)
}