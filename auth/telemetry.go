package auth

import (
	"sync"
	"time"
)

type CookieTelemetry struct {
	SessionValid    bool      `json:"sessionValid"`
	LastCheckedAt   time.Time `json:"lastCheckedAt"`
	ResponseLatency int64     `json:"responseLatencyMs"`
}

type TelemetryRegistry struct {
	mu   sync.RWMutex
	data map[string]CookieTelemetry
}

var GlobalTelemetry = &TelemetryRegistry{
	data: make(map[string]CookieTelemetry),
}

func (r *TelemetryRegistry) Update(id string, valid bool, latencyMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data == nil {
		r.data = make(map[string]CookieTelemetry)
	}
	r.data[id] = CookieTelemetry{
		SessionValid:    valid,
		LastCheckedAt:   time.Now(),
		ResponseLatency: latencyMs,
	}
}

func (r *TelemetryRegistry) Get(id string) (CookieTelemetry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.data[id]
	return t, ok
}

func (r *TelemetryRegistry) GetAll() map[string]CookieTelemetry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]CookieTelemetry, len(r.data))
	for k, v := range r.data {
		out[k] = v
	}
	return out
}
