package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestHandleWS_SubscribeFilter(t *testing.T) {
	s := &Server{Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ws/stream", s.handleWS)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/stream"
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.WriteMessage(websocket.TextMessage, []byte(`not-json`)); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte(``)); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"subscribe":["keep"]}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	s.Hub.Broadcast(core.Sample{TagID: "skip", Quality: core.QualityGood})
	s.Hub.Broadcast(core.Sample{TagID: "keep", Quality: core.QualityGood, ValueNum: floatPtr(2)})

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var dto sampleDTOType
	if err := json.Unmarshal(msg, &dto); err != nil {
		t.Fatal(err)
	}
	if dto.TagID != "keep" {
		t.Fatalf("%#v", dto)
	}
	if dto.ValueNum == nil || *dto.ValueNum != 2 {
		t.Fatalf("filtered value: %#v", dto)
	}
	if dto.Quality != int(core.QualityGood) {
		t.Fatalf("quality: %#v", dto)
	}

	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"tag_ids":[]}`)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	s.Hub.Broadcast(core.Sample{TagID: "keep", Quality: core.QualityGood, ValueNum: floatPtr(3)})
	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("empty subscribe should block all")
	}
}
