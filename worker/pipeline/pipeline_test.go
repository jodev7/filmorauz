package pipeline

import (
	"fmt"
	"strconv"
	"testing"
)

func TestCanonicalizeEpisodeID(t *testing.T) {
	parentID := "6954"
	seasonStr := "01"

	tests := []struct {
		requested string
		fetched   string
		pass      bool
	}{
		{"6954:s01e09", "9", true},
		{"6954:s01e10", "10", true},
		{"6954:s01e13", "13", true},
		{"6954:s01e09", "10", false},
	}

	for _, tt := range tests {
		epNum := tt.fetched
		if len(epNum) < 2 {
			if val, err := strconv.Atoi(epNum); err == nil {
				epNum = fmt.Sprintf("%02d", val)
			}
		}

		canonical := fmt.Sprintf("%s:s%se%s", parentID, seasonStr, epNum)
		
		isPass := canonical == tt.requested
		if isPass != tt.pass {
			t.Errorf("For requested=%s, fetched=%s: expected pass=%v, got pass=%v (canonical=%s)", tt.requested, tt.fetched, tt.pass, isPass, canonical)
		}
	}
}
