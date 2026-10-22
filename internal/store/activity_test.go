package store

import (
	"testing"
	"time"
)

// HourlyActivity is the 24h counterpart of DailyActivity: same gap-free
// contract, same UTC basis, so the two chart ranges can never disagree about
// what a bucket is or where the series ends.
func TestHourlyActivityIsGapFreeAndSummarizes(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	now := time.Now().UTC()
	thisHour := now.Format("2006-01-02 15:04:05")
	twoAgo := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	stale := now.Add(-30 * time.Hour).Format("2006-01-02 15:04:05")

	_, err := st.db.Exec(`
		INSERT INTO observations (type, title, content, created_at)
		VALUES
			('learning', 'h-1', 'x', ?),
			('learning', 'h-2', 'x', ?),
			('learning', 'h-3', 'x', ?),
			('learning', 'stale', 'x', ?);
	`, thisHour, thisHour, twoAgo, stale)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	series, err := st.HourlyActivity(24)
	if err != nil {
		t.Fatalf("HourlyActivity: %v", err)
	}
	if len(series.Hours) != 24 {
		t.Fatalf("len(Hours) = %d, want 24 (gap-free)", len(series.Hours))
	}
	if series.Total != 3 {
		t.Errorf("Total = %d, want 3 (the 30-hour-old row is out of window)", series.Total)
	}
	if series.PeakCount != 2 {
		t.Errorf("PeakCount = %d, want 2", series.PeakCount)
	}
	wantPeak := now.Format("2006-01-02 15") + ":00"
	if series.PeakHour != wantPeak {
		t.Errorf("PeakHour = %q, want %q", series.PeakHour, wantPeak)
	}
	if diff := series.AvgPerHour - 3.0/24.0; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("AvgPerHour = %v, want %v", series.AvgPerHour, 3.0/24.0)
	}

	// The last bucket must be the hour being written to now.
	last := series.Hours[len(series.Hours)-1]
	if last.Hour != wantPeak {
		t.Errorf("last hour = %q, want %q", last.Hour, wantPeak)
	}
	if last.Count != 2 {
		t.Errorf("current hour count = %d, want 2", last.Count)
	}
}

func TestHourlyActivityClampsLongRequests(t *testing.T) {
	st := createTestDB(t)
	defer st.Close()

	series, err := st.HourlyActivity(1000)
	if err != nil {
		t.Fatalf("HourlyActivity: %v", err)
	}
	if len(series.Hours) != 72 {
		t.Errorf("len(Hours) = %d, want the 72-hour clamp", len(series.Hours))
	}
}
