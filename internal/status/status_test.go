package status

import (
	"reflect"
	"testing"
	"time"
)

func TestAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	got := []string{
		Age(time.Time{}, now),
		Age(now.Add(-59*time.Second), now),
		Age(now.Add(-61*time.Second), now),
		Age(now.Add(-5*time.Hour-59*time.Minute), now),
		Age(now.Add(-49*time.Hour), now),
	}
	want := []string{"", "<1m", "1m", "5h", "2d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
