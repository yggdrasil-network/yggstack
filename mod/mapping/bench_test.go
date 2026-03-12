package mapping

import (
	"sync"
	"testing"
)

// // // // // // // // // //

func BenchmarkUDPSessionLookup_SyncMap(b *testing.B) {
	m := new(sync.Map)
	session := &UDPSessionObj{}
	m.Store("192.168.1.1:5000", session)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v, ok := m.Load("192.168.1.1:5000")
			if ok {
				_ = v.(*UDPSessionObj)
			}
		}
	})
}

func BenchmarkUDPSessionLookup_SyncMap_MixedReadWrite(b *testing.B) {
	m := new(sync.Map)
	session := &UDPSessionObj{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "192.168.1.1:5000"
			if i%10 == 0 {
				m.Store(key, session)
			} else {
				v, ok := m.Load(key)
				if ok {
					_ = v.(*UDPSessionObj)
				}
			}
			i++
		}
	})
}

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
