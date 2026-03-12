package mapping

import (
	"sync"
	"testing"
)

// // // // // // // // // //

func BenchmarkUDPSessionLookup_MapMutex(b *testing.B) {
	var mu sync.RWMutex
	m := map[string]*UDPSessionObj{"192.168.1.1:5000": {}}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.RLock()
			_ = m["192.168.1.1:5000"]
			mu.RUnlock()
		}
	})
}

func BenchmarkUDPSessionLookup_MapMutex_MixedReadWrite(b *testing.B) {
	var mu sync.RWMutex
	m := map[string]*UDPSessionObj{}
	session := &UDPSessionObj{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "192.168.1.1:5000"
			if i%10 == 0 {
				mu.Lock()
				m[key] = session
				mu.Unlock()
			} else {
				mu.RLock()
				_ = m[key]
				mu.RUnlock()
			}
			i++
		}
	})
}
