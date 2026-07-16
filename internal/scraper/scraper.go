package scraper

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/feed"
)

var publishedAtLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	time.RFC822Z,
	time.RFC822,
}

func ParsePublishedAt(pubDate string) time.Time {
	for _, layout := range publishedAtLayouts {
		if t, err := time.Parse(layout, pubDate); err == nil {
			return t
		}
	}
	return time.Time{}
}

func Scrape(s *cli.State) error {
	nextFeed, err := s.DB.GetNextFeedToFetch(context.Background())
	if err != nil {
		return err
	}

	_, err = s.DB.MarkFeedFetched(context.Background(), database.MarkFeedFetchedParams{
		ID:        nextFeed.ID,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	rssFeed, err := feed.Fetch(context.Background(), nextFeed.Url)
	if err != nil {
		return err
	}

	for _, item := range rssFeed.Channel.Item {
		_, err := s.DB.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
			Title:       item.Title,
			Url:         item.Link,
			Description: item.Description,
			PublishedAt: ParsePublishedAt(item.PubDate),
			FeedID:      nextFeed.ID,
		})
		if err != nil {
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
				continue
			}
			log.Printf("couldn't create post %q: %v", item.Title, err)
		}
	}

	return nil
}
