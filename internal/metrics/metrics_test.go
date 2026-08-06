package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerExposesCollectors(t *testing.T) {
	SamplesWritten.Add(1)
	SamplesSuppressedUnchanged.Add(1)
	SamplesSpooled.Add(1)
	WriteErrors.Add(1)
	OPCConnected.Set(1)
	SpoolDepth.Set(3)
	CapacityHalts.Add(1)
	CapacityDrops.Add(2)

	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	for _, name := range []string{
		"level2_samples_written_total",
		"level2_samples_suppressed_unchanged_total",
		"level2_samples_spooled_total",
		"level2_historian_write_errors_total",
		"level2_opc_connected",
		"level2_spool_depth",
		"level2_historian_capacity_halts_total",
		"level2_historian_capacity_drops_total",
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("missing metric %s in body (len=%d)", name, len(body))
		}
	}
}
