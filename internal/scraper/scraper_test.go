package scraper

import (
	"testing"
	"time"
)

func TestParsePublishedAt(t *testing.T) {
	tests := []struct {
		name    string
		pubDate string
		want    time.Time
	}{
		{
			name:    "RFC1123Z",
			pubDate: "Mon, 06 Sep 2021 12:00:00 +0000",
			want:    time.Date(2021, time.September, 6, 12, 0, 0, 0, time.UTC),
		},
		{
			name:    "RFC1123",
			pubDate: "Mon, 06 Sep 2021 12:00:00 UTC",
			want:    time.Date(2021, time.September, 6, 12, 0, 0, 0, time.UTC),
		},
		{
			name:    "RFC3339",
			pubDate: "2021-09-06T12:00:00Z",
			want:    time.Date(2021, time.September, 6, 12, 0, 0, 0, time.UTC),
		},
		{
			name:    "unrecognized format returns zero value",
			pubDate: "not a real date",
			want:    time.Time{},
		},
		{
			name:    "empty string returns zero value",
			pubDate: "",
			want:    time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePublishedAt(tt.pubDate)
			if !got.Equal(tt.want) {
				t.Errorf("ParsePublishedAt(%q) = %v, want %v", tt.pubDate, got, tt.want)
			}
		})
	}
}
