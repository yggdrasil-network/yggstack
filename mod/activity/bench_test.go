package activity

import (
	"testing"
)

// // // // // // // // // //

func BenchmarkGenerateConnId(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GenerateConnId("socks", "127.0.0.1:1080")
	}
}

func BenchmarkGenerateConnId_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = GenerateConnId("socks", "127.0.0.1:1080")
		}
	})
}

// //

func BenchmarkCounterObj(b *testing.B) {
	counter := &CounterObj{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Increment()
			counter.Decrement()
		}
	})
}
