package bot

import "testing"

func TestVoteThreshold(t *testing.T) {
	tests := []struct {
		listeners int
		want      int
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{5, 3},
		{6, 4},
		{7, 4},
		{8, 5},
		{9, 5},
		{10, 6},
	}
	for _, tt := range tests {
		if got := voteThreshold(tt.listeners); got != tt.want {
			t.Errorf("voteThreshold(%d) = %d, want %d", tt.listeners, got, tt.want)
		}
	}
}
