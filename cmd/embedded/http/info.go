package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

type peerJSON struct {
	URI           string     `json:"uri"`
	Up            bool       `json:"up"`
	RxBytes       uint64     `json:"rx_bytes"`
	TxBytes       uint64     `json:"tx_bytes"`
	RxRate        uint64     `json:"rx_rate"`
	TxRate        uint64     `json:"tx_rate"`
	LatencyMs     float64    `json:"latency_ms"`
	LastError     string     `json:"last_error,omitempty"`
	LastErrorTime *time.Time `json:"last_error_time,omitempty"`
}

type bandwidthJSON struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
	RxRate  uint64 `json:"rx_rate"`
	TxRate  uint64 `json:"tx_rate"`
}

type infoJSON struct {
	PublicKey     string        `json:"public_key"`
	YggAddress    string        `json:"ygg_address"`
	Addresses     []string      `json:"addresses"`
	YggPorts      []int         `json:"ygg_ports"`
	IsYggdrasil   bool          `json:"is_yggdrasil"`
	UptimeSeconds float64       `json:"uptime_seconds"`
	Sessions      int           `json:"sessions"`
	Bandwidth     bandwidthJSON `json:"bandwidth"`
	Peers         []peerJSON    `json:"peers"`
	CachedAt      time.Time     `json:"cached_at"`
}

// //

// cachedMetricsObj holds the expensive metrics refreshed every 10s.
type cachedMetricsObj struct {
	peers    []peerJSON
	sessions int
	bw       bandwidthJSON
	at       time.Time
}

type InfoHandlerObj struct {
	c         *core.Core
	cfg       *ConfigObj
	log       core.Logger
	startTime time.Time
	mu        sync.Mutex
	cached    *cachedMetricsObj
}

// //

func newInfoHandler(c *core.Core, cfg *ConfigObj, log core.Logger) *InfoHandlerObj {
	return &InfoHandlerObj{c: c, cfg: cfg, log: log, startTime: time.Now()}
}

func (h *InfoHandlerObj) refreshMetrics() *cachedMetricsObj {
	peers := h.c.GetPeers()
	sessions := h.c.GetSessions()

	peerList := make([]peerJSON, len(peers))
	var bw bandwidthJSON
	for i, p := range peers {
		entry := peerJSON{
			URI:       p.URI,
			Up:        p.Up,
			RxBytes:   p.RXBytes,
			TxBytes:   p.TXBytes,
			RxRate:    p.RXRate,
			TxRate:    p.TXRate,
			LatencyMs: float64(p.Latency.Microseconds()) / 1000.0,
		}
		if !p.Up && p.LastError != nil {
			msg := p.LastError.Error()
			entry.LastError = msg
			entry.LastErrorTime = &p.LastErrorTime
			h.log.Warnf("peer down: %s — %s", p.URI, msg)
		}
		peerList[i] = entry
		bw.RxBytes += p.RXBytes
		bw.TxBytes += p.TXBytes
		bw.RxRate += p.RXRate
		bw.TxRate += p.TXRate
	}
	return &cachedMetricsObj{
		peers:    peerList,
		sessions: len(sessions),
		bw:       bw,
		at:       time.Now(),
	}
}

func (h *InfoHandlerObj) getMetrics() *cachedMetricsObj {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cached == nil || time.Since(h.cached.at) >= 10*time.Second {
		h.cached = h.refreshMetrics()
	}
	return h.cached
}

func (h *InfoHandlerObj) buildAddresses() []string {
	if h.cfg.Hostname == "localhost" {
		addrs := make([]string, len(h.cfg.HTTPPorts))
		for i, p := range h.cfg.HTTPPorts {
			addrs[i] = fmt.Sprintf("localhost:%d", p)
		}
		return addrs
	}
	return []string{h.cfg.Hostname}
}

// Handler returns an http.Handler that serves /yggdrasil-server.json.
// isYggdrasil controls the value of the is_yggdrasil field in the response.
func (h *InfoHandlerObj) Handler(isYggdrasil bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := h.getMetrics()
		resp := infoJSON{
			PublicKey:     hex.EncodeToString(h.c.PublicKey()),
			YggAddress:    h.c.Address().String(),
			Addresses:     h.buildAddresses(),
			YggPorts:      h.cfg.YggPorts,
			IsYggdrasil:   isYggdrasil,
			UptimeSeconds: time.Since(h.startTime).Seconds(),
			Sessions:      m.sessions,
			Bandwidth:     m.bw,
			Peers:         m.peers,
			CachedAt:      m.at,
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=10")
		w.Write(data)
	})
}
