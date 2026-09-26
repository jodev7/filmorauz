package repositories

import "testing"

func TestWatchedSeconds(t *testing.T) {
	cases := []struct {
		r    yearRecord
		want int64
	}{
		{yearRecord{Completed: true, DurationSec: 5400, LastPositionSec: 100}, 5400},
		{yearRecord{ProgressPercent: 95, DurationSec: 3000, LastPositionSec: 2850}, 3000},
		{yearRecord{DurationSec: 3000, LastPositionSec: 1200, ProgressPercent: 40}, 1200},
		{yearRecord{DurationSec: 3000, LastPositionSec: 9999}, 3000},
		{yearRecord{LastPositionSec: -5}, 0},
	}
	for i, c := range cases {
		if got := watchedSeconds(c.r); got != c.want {
			t.Errorf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestTopCounts(t *testing.T) {
	got := topCounts(map[string]int{"drama": 3, "action": 5, "comedy": 3, "horror": 1}, 3)
	if len(got) != 3 || got[0].Key != "action" || got[1].Key != "comedy" || got[2].Key != "drama" {
		t.Fatalf("unexpected order %+v", got)
	}
}
