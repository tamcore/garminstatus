package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func up(name string) garminstatus.ServiceMap {
	return garminstatus.ServiceMap{name: {Status: garminstatus.Up}}
}

// down builds a single-service map for service "A" in the down state.
func down(reasons ...string) garminstatus.ServiceMap {
	return garminstatus.ServiceMap{"A": {Status: garminstatus.Down, StatusReason: reasons}}
}

func TestAppendAndReadAll(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "nested", "snapshots.jsonl")
	t0 := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	recs := []Snapshot{
		{TS: t0, Kind: KindChange, Platforms: up("Garmin Connect"), Features: garminstatus.ServiceMap{}},
		{TS: t0.Add(time.Hour), Kind: KindHeartbeat, Platforms: up("Garmin Connect"), Features: garminstatus.ServiceMap{}},
	}

	// Act
	for _, r := range recs {
		if err := Append(path, r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got, err := ReadAll(path)

	// Assert
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if !got[0].TS.Equal(t0) || got[1].Kind != KindHeartbeat {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestReadAllMissingFileIsEmpty(t *testing.T) {
	got, err := ReadAll(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if _, ok, _ := Last(path); ok {
		t.Fatal("expected ok=false on empty log")
	}
	t0 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	_ = Append(path, Snapshot{TS: t0, Kind: KindChange, Platforms: up("A")})
	_ = Append(path, Snapshot{TS: t0.Add(time.Hour), Kind: KindChange, Platforms: down()})
	last, ok, err := Last(path)
	if err != nil || !ok {
		t.Fatalf("Last: ok=%v err=%v", ok, err)
	}
	if last.Platforms["A"].Status != garminstatus.Down {
		t.Errorf("expected last=down, got %v", last.Platforms["A"].Status)
	}
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cur := garminstatus.GarminStatus{Platforms: up("A"), Features: garminstatus.ServiceMap{}}

	t.Run("no prior record writes change", func(t *testing.T) {
		rec, write := Decide(nil, cur, now, 3*time.Hour)
		if !write || rec.Kind != KindChange {
			t.Fatalf("expected change write, got write=%v kind=%v", write, rec.Kind)
		}
	})

	t.Run("status change writes change", func(t *testing.T) {
		last := &Snapshot{TS: now.Add(-time.Minute), Platforms: down(), Features: garminstatus.ServiceMap{}}
		rec, write := Decide(last, cur, now, 3*time.Hour)
		if !write || rec.Kind != KindChange {
			t.Fatalf("expected change write, got write=%v kind=%v", write, rec.Kind)
		}
	})

	t.Run("reason-only change does not write", func(t *testing.T) {
		last := &Snapshot{TS: now.Add(-time.Minute), Platforms: down("old reason"), Features: garminstatus.ServiceMap{}}
		curDown := garminstatus.GarminStatus{Platforms: down("new reason"), Features: garminstatus.ServiceMap{}}
		_, write := Decide(last, curDown, now, 3*time.Hour)
		if write {
			t.Fatal("reason-only change should not write")
		}
	})

	t.Run("unchanged within heartbeat does not write", func(t *testing.T) {
		last := &Snapshot{TS: now.Add(-time.Hour), Platforms: up("A"), Features: garminstatus.ServiceMap{}}
		_, write := Decide(last, cur, now, 3*time.Hour)
		if write {
			t.Fatal("expected no write within heartbeat window")
		}
	})

	t.Run("unchanged past heartbeat writes heartbeat", func(t *testing.T) {
		last := &Snapshot{TS: now.Add(-4 * time.Hour), Platforms: up("A"), Features: garminstatus.ServiceMap{}}
		rec, write := Decide(last, cur, now, 3*time.Hour)
		if !write || rec.Kind != KindHeartbeat {
			t.Fatalf("expected heartbeat write, got write=%v kind=%v", write, rec.Kind)
		}
	})
}
