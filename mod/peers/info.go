package peers

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// // // // // // // // // //

// InfoObj holds peer state for JSON export.
type InfoObj struct {
	URI       string  `json:"uri"`
	Up        bool    `json:"up"`
	Inbound   bool    `json:"inbound"`
	PublicKey string  `json:"public_key"`
	RXBytes   uint64  `json:"rx_bytes"`
	TXBytes   uint64  `json:"tx_bytes"`
	Uptime    float64 `json:"uptime_seconds"`
	Latency   float64 `json:"latency_ms"`
	LastError string  `json:"last_error,omitempty"`
}

// //

// GetPeers returns a snapshot of all configured peers.
func GetPeers(c CoreInterface) []InfoObj {
	if c == nil {
		return []InfoObj{}
	}
	raw := c.GetPeers()
	result := make([]InfoObj, 0, len(raw))
	for _, p := range raw {
		info := InfoObj{
			URI:     p.URI,
			Up:      p.Up,
			Inbound: p.Inbound,
			RXBytes: p.RXBytes,
			TXBytes: p.TXBytes,
			Uptime:  p.Uptime.Seconds(),
			Latency: float64(p.Latency.Microseconds()) / 1000.0,
		}
		if len(p.Key) > 0 {
			info.PublicKey = hex.EncodeToString(p.Key)
		}
		if p.LastError != nil {
			info.LastError = p.LastError.Error()
		}
		result = append(result, info)
	}
	return result
}

// GetPeersJSON returns peer stats as a JSON byte slice.
func GetPeersJSON(c CoreInterface) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("node is not running")
	}
	return json.Marshal(GetPeers(c))
}
