package bulkdl

import (
	"testing"
)

func TestStatsSnapshot(t *testing.T) {
	stats := newStats()
	stats.MarkScanned()
	stats.MarkScanned()
	stats.MarkSkipped()
	stats.AddSaved(3)
	stats.AddFailed(1)

	snap := stats.Snapshot()
	if snap.Scanned != 2 || snap.Saved != 3 || snap.Skipped != 1 || snap.Failed != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if stats.Saved() != 3 {
		t.Fatalf("Saved() = %d, want 3", stats.Saved())
	}
}
