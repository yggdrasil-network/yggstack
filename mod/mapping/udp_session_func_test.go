package mapping

import (
	"sync"
	"testing"
)

// // // // // // // // // //

func TestUDPSessionMap_Delete(t *testing.T) {
	m := NewUDPSessionMap()
	m.Store("a", &UDPSessionObj{Conn: &mockConnObj{}})
	m.Store("b", &UDPSessionObj{Conn: &mockConnObj{}})

	m.Delete("a")

	if _, ok := m.Load("a"); ok {
		t.Error("key 'a' should have been deleted")
	}
	if _, ok := m.Load("b"); !ok {
		t.Error("key 'b' should still exist")
	}
}

func TestUDPSessionMap_DeleteNonExistent(t *testing.T) {
	m := NewUDPSessionMap()
	// Must not panic when deleting a non-existent key.
	m.Delete("missing")
}

func TestUDPSessionMap_Range_AllVisited(t *testing.T) {
	m := NewUDPSessionMap()
	keys := []string{"x", "y", "z"}
	for _, k := range keys {
		m.Store(k, &UDPSessionObj{Conn: &mockConnObj{}})
	}

	visited := make(map[string]bool)
	m.Range(func(k string, _ *UDPSessionObj) bool {
		visited[k] = true
		return true
	})

	for _, k := range keys {
		if !visited[k] {
			t.Errorf("key %q not visited by Range", k)
		}
	}
}

func TestUDPSessionMap_Range_EarlyExit(t *testing.T) {
	m := NewUDPSessionMap()
	for i := 0; i < 10; i++ {
		m.Store(string(rune('a'+i)), &UDPSessionObj{Conn: &mockConnObj{}})
	}

	count := 0
	m.Range(func(_ string, _ *UDPSessionObj) bool {
		count++
		return count < 3 // stop after 3rd element
	})

	if count != 3 {
		t.Errorf("Range visited %d elements, want 3", count)
	}
}

func TestUDPSessionMap_Range_Empty(t *testing.T) {
	m := NewUDPSessionMap()
	called := false
	m.Range(func(_ string, _ *UDPSessionObj) bool {
		called = true
		return true
	})
	if called {
		t.Error("Range callback should not be called on empty map")
	}
}

// //

func TestUDPSessionMap_ConcurrentStoreLoad(t *testing.T) {
	m := NewUDPSessionMap()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		key := string(rune(i))
		go func() {
			defer wg.Done()
			m.Store(key, &UDPSessionObj{Conn: &mockConnObj{}})
		}()
		go func() {
			defer wg.Done()
			m.Load(key)
		}()
	}
	wg.Wait()
}

func TestUDPSessionMap_ConcurrentDeleteRange(t *testing.T) {
	m := NewUDPSessionMap()
	for i := 0; i < 50; i++ {
		m.Store(string(rune(i)), &UDPSessionObj{Conn: &mockConnObj{}})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m.Delete(string(rune(i)))
		}
	}()
	go func() {
		defer wg.Done()
		m.Range(func(_ string, _ *UDPSessionObj) bool { return true })
	}()

	wg.Wait()
}

// //

func BenchmarkUDPSessionMap_Load(b *testing.B) {
	m := NewUDPSessionMap()
	m.Store("192.168.1.1:5000", &UDPSessionObj{})
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Load("192.168.1.1:5000")
		}
	})
}

func BenchmarkUDPSessionMap_MixedReadWrite(b *testing.B) {
	m := NewUDPSessionMap()
	session := &UDPSessionObj{}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "192.168.1.1:5000"
			if i%10 == 0 {
				m.Store(key, session)
			} else {
				m.Load(key)
			}
			i++
		}
	})
}
