package yggstack

// // // // // // // // // //

func (m *udpSessionMapObj) Load(key string) (*udpSessionObj, bool) {
	m.mu.RLock()
	v, ok := m.data[key]
	m.mu.RUnlock()
	return v, ok
}

func (m *udpSessionMapObj) Store(key string, session *udpSessionObj) {
	m.mu.Lock()
	m.data[key] = session
	m.mu.Unlock()
}

func (m *udpSessionMapObj) Delete(key string) {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
}

func (m *udpSessionMapObj) Range(fn func(key string, session *udpSessionObj) bool) {
	m.mu.RLock()
	// Copy keys to iterate without holding the lock
	keys := make([]string, 0, len(m.data))
	vals := make([]*udpSessionObj, 0, len(m.data))
	for k, v := range m.data {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	m.mu.RUnlock()
	for i, k := range keys {
		if !fn(k, vals[i]) {
			return
		}
	}
}
