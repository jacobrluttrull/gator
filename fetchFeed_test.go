package main

import (
	"context"
	"testing"
	"time"
)

func TestFetchFeed(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "TechCrunch", url: "https://techcrunch.com/feed/"},
		{name: "Hacker News", url: "https://news.ycombinator.com/rss"},
		{name: "Lane Wagner's Blog", url: "https://www.wagslane.dev/index.xml"},
		{name: "Ars Technica", url: "https://feeds.arstechnica.com/arstechnica/index"},
		{name: "NPR", url: "https://feeds.npr.org/1001/rss.xml"},
		{name: "NASA Breaking News", url: "https://www.nasa.gov/feed/"},
		{name: "Smashing Magazine", url: "https://www.smashingmagazine.com/feed/"},
		{name: "CSS-Tricks", url: "https://css-tricks.com/feed/"},
		{name: "Krebs on Security", url: "https://krebsonsecurity.com/feed/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			feed, err := fetchFeed(ctx, tt.url)
			if err != nil {
				t.Fatalf("fetchFeed(%q) returned error: %v", tt.url, err)
			}

			if feed.Channel.Title == "" {
				t.Error("expected channel title to be non-empty")
			}

			if len(feed.Channel.Item) == 0 {
				t.Error("expected at least one item in the feed")
			}

			for _, item := range feed.Channel.Item {
				if item.Title == "" {
					t.Error("expected item title to be non-empty")
				}
				if item.Link == "" {
					t.Error("expected item link to be non-empty")
				}
			}
		})
	}
}
