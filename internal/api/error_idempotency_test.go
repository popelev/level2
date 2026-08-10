package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIErrorEnvelope_WriteDisabled(t *testing.T) {
	_, mux := writeTestServer(t, false, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":42.5,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d body %s", rr.Code, rr.Body.String())
	}
	var errBody apiErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("json: %v body=%s", err, rr.Body.String())
	}
	if errBody.Error != "opc_write_disabled" || errBody.Message == "" {
		t.Fatalf("%#v", errBody)
	}
}

func TestAPIErrorEnvelope_TagValueMissing(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/missing/value", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rr.Code)
	}
	var errBody apiErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error != "tag_not_found" {
		t.Fatalf("%#v", errBody)
	}
}

func TestAPIErrorEnvelope_AuthUnauthorized(t *testing.T) {
	s, mux := writeTestServer(t, true, &mockWriter{connected: true})
	s.APITokenWrite = func() string { return "write-secret" }
	handler := s.APIAuth(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1}`)))
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d body %s", rr.Code, rr.Body.String())
	}
	var errBody apiErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error != "unauthorized" {
		t.Fatalf("%#v", errBody)
	}
}

func TestIdempotencyKey_ReplayDoesNotDoubleWrite(t *testing.T) {
	w := &mockWriter{connected: true}
	_, mux := writeTestServer(t, true, w)
	body := []byte(`{"value":42.5,"device_id":"s7_1500"}`)

	req1 := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value", bytes.NewReader(body))
	req1.Header.Set("Idempotency-Key", "pulse-1")
	rr1 := httptest.NewRecorder()
	mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rr1.Code, rr1.Body.String())
	}
	if w.calls != 1 {
		t.Fatalf("calls after first=%d", w.calls)
	}

	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value", bytes.NewReader(body))
	req2.Header.Set("Idempotency-Key", "pulse-1")
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("replay: %d %s", rr2.Code, rr2.Body.String())
	}
	if rr2.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("missing Idempotent-Replayed header")
	}
	if w.calls != 1 {
		t.Fatalf("calls after replay=%d want 1", w.calls)
	}
}

func TestIdempotencyKey_ConflictDifferentBody(t *testing.T) {
	w := &mockWriter{connected: true}
	_, mux := writeTestServer(t, true, w)

	req1 := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`)))
	req1.Header.Set("Idempotency-Key", "same-key")
	rr1 := httptest.NewRecorder()
	mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rr1.Code, rr1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":2,"device_id":"s7_1500"}`)))
	req2.Header.Set("Idempotency-Key", "same-key")
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("want 409 got %d %s", rr2.Code, rr2.Body.String())
	}
	var errBody apiErrorBody
	if err := json.Unmarshal(rr2.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error != "idempotency_key_reuse" {
		t.Fatalf("%#v", errBody)
	}
	if w.calls != 1 {
		t.Fatalf("calls=%d", w.calls)
	}
}

func TestIdempotencyKey_BatchReplay(t *testing.T) {
	w := &mockWriter{connected: true}
	_, mux := writeTestServer(t, true, w)
	body := []byte(`{"writes":[{"tag_id":"Motor1.SpeedSP","value":7,"device_id":"s7_1500"}]}`)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", bytes.NewReader(body))
	req1.Header.Set("Idempotency-Key", "batch-1")
	rr1 := httptest.NewRecorder()
	mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rr1.Code, rr1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", bytes.NewReader(body))
	req2.Header.Set("Idempotency-Key", "batch-1")
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatal("expected replay")
	}
	if w.calls != 1 {
		t.Fatalf("calls=%d", w.calls)
	}
}