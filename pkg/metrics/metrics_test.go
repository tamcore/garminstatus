package metrics

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func TestUpdateStatus(t *testing.T) {
	UpdateStatus(garminstatus.GarminStatus{
		Platforms: garminstatus.ServiceMap{"Connect": {Status: garminstatus.Up}},
		Features:  garminstatus.ServiceMap{"Upload": {Status: garminstatus.Down}},
	})
	if got := testutil.ToFloat64(platformStatus.WithLabelValues("Connect")); got != 1 {
		t.Errorf("platform up = %v, want 1", got)
	}
	if got := testutil.ToFloat64(featureStatus.WithLabelValues("Upload")); got != 0 {
		t.Errorf("feature down = %v, want 0", got)
	}
}

func TestRecordFetch(t *testing.T) {
	RecordFetch(true)
	if testutil.ToFloat64(fetchSuccess) != 1 {
		t.Error("fetch success should be 1")
	}
	RecordFetch(false)
	if testutil.ToFloat64(fetchSuccess) != 0 {
		t.Error("fetch success should be 0")
	}
}

func TestRecordSync(t *testing.T) {
	RecordSync("data", "pull", nil)
	if testutil.ToFloat64(syncSuccess.WithLabelValues("data", "pull")) != 1 {
		t.Error("sync success should be 1 on nil error")
	}
	if testutil.ToFloat64(syncTimestamp.WithLabelValues("data", "pull")) == 0 {
		t.Error("timestamp should be stamped on success")
	}
	RecordSync("gh-pages", "push", errors.New("boom"))
	if testutil.ToFloat64(syncSuccess.WithLabelValues("gh-pages", "push")) != 0 {
		t.Error("sync success should be 0 on error")
	}
	if testutil.ToFloat64(syncErrors.WithLabelValues("gh-pages", "push")) != 1 {
		t.Error("error counter should increment")
	}
}

func TestRecordCycle(t *testing.T) {
	RecordCycle()
	if testutil.ToFloat64(cycleTimestamp) == 0 {
		t.Error("cycle timestamp should be set")
	}
}

func TestBoolToFloat(t *testing.T) {
	if boolToFloat(true) != 1 || boolToFloat(false) != 0 {
		t.Error("boolToFloat mapping wrong")
	}
}
