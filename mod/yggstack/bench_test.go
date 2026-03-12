package yggstack

import (
	"sync"
	"testing"
)

// // // // // // // // // //

func BenchmarkGenerateConnId(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = generateConnId("socks", "127.0.0.1:1080")
	}
}

func BenchmarkGenerateConnId_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = generateConnId("socks", "127.0.0.1:1080")
		}
	})
}

// //

func BenchmarkConnectionCounterObj(b *testing.B) {
	counter := &connectionCounterObj{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.increment()
			counter.decrement()
		}
	})
}

// //

func BenchmarkUDPSessionLookup_SyncMap(b *testing.B) {
	m := new(sync.Map)
	session := &udpSessionObj{}
	m.Store("192.168.1.1:5000", session)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v, ok := m.Load("192.168.1.1:5000")
			if ok {
				_ = v.(*udpSessionObj)
			}
		}
	})
}

func BenchmarkUDPSessionLookup_SyncMap_MixedReadWrite(b *testing.B) {
	m := new(sync.Map)
	session := &udpSessionObj{}
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
					_ = v.(*udpSessionObj)
				}
			}
			i++
		}
	})
}

func BenchmarkUDPSessionLookup_MapMutex(b *testing.B) {
	var mu sync.RWMutex
	m := map[string]*udpSessionObj{"192.168.1.1:5000": {}}
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
	m := map[string]*udpSessionObj{}
	session := &udpSessionObj{}
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
