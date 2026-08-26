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

func TestVoteThresholdFor(t *testing.T) {
	// Not in the bot's channel (or bot not connected) is always a denial.
	for _, listeners := range []int{0, 1, 2, 5, 10} {
		if got := voteThresholdFor(false, listeners); got != -1 {
			t.Errorf("voteThresholdFor(false, %d) = %d, want -1", listeners, got)
		}
	}
	if got := voteThresholdFor(true, 0); got != 0 {
		t.Errorf("voteThresholdFor(true, 0) = %d, want 0 (alone = instant)", got)
	}
	if got := voteThresholdFor(true, 1); got != 0 {
		t.Errorf("voteThresholdFor(true, 1) = %d, want 0", got)
	}
	if got := voteThresholdFor(true, 3); got != 2 {
		t.Errorf("voteThresholdFor(true, 3) = %d, want 2 (majority)", got)
	}
}
