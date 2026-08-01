package handlers

import "testing"

func TestFormatEpisodeNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want string
	}{
		{"empty", nil, ""},
		{"single", []int{7}, "7"},
		{"pair stays listed", []int{4, 5}, "4, 5"},
		{"run collapses", []int{7, 8, 9, 10}, "7-10"},
		{"unsorted input", []int{22, 4, 9, 8, 7, 10}, "4, 7-10, 22"},
		{"duplicates ignored", []int{3, 3, 4, 4, 5}, "3-5"},
		{"disjoint singles", []int{1, 3, 5}, "1, 3, 5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatEpisodeNumbers(tt.in); got != tt.want {
				t.Errorf("formatEpisodeNumbers(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The real skipped-episode list that triggered this code path (uzmovi
// "Ruxshunos", 63 of 139 episodes lost) must stay readable in the admin UI.
func TestFormatEpisodeNumbersTruncatesLongLists(t *testing.T) {
	skipped := []int{4, 7, 11, 22, 26, 57, 58, 60, 61, 62, 63, 65, 66, 67, 68, 69, 72, 73, 74,
		76, 79, 80, 81, 83, 85, 86, 87, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100, 101,
		103, 104, 105, 106, 107, 108, 109, 110, 111, 113, 115, 116, 117, 118, 119, 120, 121,
		122, 123, 125, 126, 127, 128, 130}

	got := formatEpisodeNumbers(skipped)
	if len(got) > 200 {
		t.Errorf("message too long for the admin UI (%d chars): %s", len(got), got)
	}
	if want := "4, 7, 11, 22, 26, 57, 58, 60-63, 65-69, 72-74, 76, 79-81, 83, 85-87, 89-101"; len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("unexpected prefix:\n got %q\nwant prefix %q", got, want)
	}
}
