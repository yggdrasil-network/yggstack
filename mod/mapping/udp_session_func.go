package mapping

// // // // // // // // // //

func (m *UDPSessionMapObj) Load(key string) (*UDPSessionObj, bool) {
	m.mu.RLock()
	v, ok := m.data[key]
	m.mu.RUnlock()
	return v, ok
}

func (m *UDPSessionMapObj) Store(key string, session *UDPSessionObj) {
	m.mu.Lock()
	m.data[key] = session
	m.mu.Unlock()
}

func (m *UDPSessionMapObj) Delete(key string) {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
}

func (m *UDPSessionMapObj) Range(fn func(key string, session *UDPSessionObj) bool) {
	m.mu.RLock()
	keys := make([]string, 0, len(m.data))
	vals := make([]*UDPSessionObj, 0, len(m.data))
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
