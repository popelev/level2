package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultIdempotencyTTL   = 10 * time.Minute
	maxIdempotencyEntries   = 4096
	idempotencyHeader       = "Idempotency-Key"
	idempotencyReplayHeader = "Idempotent-Replayed"
)

type idempotencyCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]*idempotencyEntry
}

type idempotencyEntry struct {
	fp      string
	status  int
	ct      string
	body    []byte
	expires time.Time
	done    bool
	wait    chan struct{}
}

func newIdempotencyCache(ttl time.Duration) *idempotencyCache {
	if ttl <= 0 {
		ttl = defaultIdempotencyTTL
	}
	return &idempotencyCache{ttl: ttl, entries: make(map[string]*idempotencyEntry)}
}

func (s *Server) idemCache() *idempotencyCache {
	s.idemOnce.Do(func() {
		s.idempotency = newIdempotencyCache(defaultIdempotencyTTL)
	})
	return s.idempotency
}

func fingerprintRequest(method, path string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(method))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

type captureResponseWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	ct     string
}

func (c *captureResponseWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureResponseWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if c.ct == "" {
		c.ct = c.Header().Get("Content-Type")
	}
	_, _ = c.buf.Write(p)
	return c.ResponseWriter.Write(p)
}

func (s *Server) withIdempotency(w http.ResponseWriter, r *http.Request, body []byte, run func(http.ResponseWriter)) {
	key := strings.TrimSpace(r.Header.Get(idempotencyHeader))
	if key == "" {
		run(w)
		return
	}
	fp := fingerprintRequest(r.Method, r.URL.Path, body)
	cache := s.idemCache()

	cache.mu.Lock()
	cache.purgeExpiredLocked(time.Now())
	if ent, ok := cache.entries[key]; ok {
		if !ent.done {
			wait := ent.wait
			sameFP := ent.fp == fp
			cache.mu.Unlock()
			if !sameFP {
				writeAPIError(w, http.StatusConflict, "idempotency_key_reuse",
					"Idempotency-Key is already in use with a different request body")
				return
			}
			<-wait
			cache.mu.Lock()
			ent = cache.entries[key]
			if ent != nil && ent.done && ent.fp == fp && time.Now().Before(ent.expires) {
				replayIdempotent(w, ent)
				cache.mu.Unlock()
				return
			}
			cache.mu.Unlock()
			cache.mu.Lock()
		} else if time.Now().Before(ent.expires) {
			if ent.fp != fp {
				cache.mu.Unlock()
				writeAPIError(w, http.StatusConflict, "idempotency_key_reuse",
					"Idempotency-Key was already used with a different request body")
				return
			}
			replayIdempotent(w, ent)
			cache.mu.Unlock()
			return
		} else {
			delete(cache.entries, key)
		}
	}

	if len(cache.entries) >= maxIdempotencyEntries {
		cache.evictOldestLocked()
	}
	ent := &idempotencyEntry{fp: fp, wait: make(chan struct{}), expires: time.Now().Add(cache.ttl)}
	cache.entries[key] = ent
	cache.mu.Unlock()

	cw := &captureResponseWriter{ResponseWriter: w}
	run(cw)

	cache.mu.Lock()
	ent.status = cw.status
	if ent.status == 0 {
		ent.status = http.StatusOK
	}
	ent.ct = cw.ct
	if ent.ct == "" {
		ent.ct = cw.Header().Get("Content-Type")
	}
	ent.body = append([]byte(nil), cw.buf.Bytes()...)
	ent.done = true
	ent.expires = time.Now().Add(cache.ttl)
	close(ent.wait)
	cache.mu.Unlock()
}

func replayIdempotent(w http.ResponseWriter, ent *idempotencyEntry) {
	if ent.ct != "" {
		w.Header().Set("Content-Type", ent.ct)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set(idempotencyReplayHeader, "true")
	w.WriteHeader(ent.status)
	_, _ = w.Write(ent.body)
}

func (c *idempotencyCache) purgeExpiredLocked(now time.Time) {
	for k, e := range c.entries {
		if e.done && now.After(e.expires) {
			delete(c.entries, k)
		}
	}
}

func (c *idempotencyCache) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, e := range c.entries {
		if !e.done {
			continue
		}
		if first || e.expires.Before(oldest) {
			oldest = e.expires
			oldestKey = k
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}