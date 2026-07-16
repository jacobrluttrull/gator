package supervisor

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{name: "doubles below cap", current: time.Second, want: 2 * time.Second},
		{name: "doubles again", current: 4 * time.Second, want: 8 * time.Second},
		{name: "caps at max", current: maxBackoff, want: maxBackoff},
		{name: "caps when doubling would exceed max", current: 40 * time.Second, want: maxBackoff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBackoff(tt.current)
			if got != tt.want {
				t.Errorf("nextBackoff(%s) = %s, want %s", tt.current, got, tt.want)
			}
		})
	}
}
