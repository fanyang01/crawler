package count

import "sync"

type Count struct {
	URL      int
	Depth    map[int]int
	KV       map[string]int
	Response struct {
		Count int
		Depth map[int]int
	}
}

type Hosts struct {
	mu sync.RWMutex
	m  map[string]*Count
}

func NewHosts() *Hosts {
	return &Hosts{
		m: make(map[string]*Count),
	}
}

func (h *Hosts) Update(host string, f func(*Count)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var entry *Count
	if entry = h.m[host]; entry == nil {
		entry = &Count{
			Depth: make(map[int]int),
			KV:    make(map[string]int),
			Response: struct {
				Count int
				Depth map[int]int
			}{
				Depth: make(map[int]int),
			},
		}
		h.m[host] = entry
	}
	f(entry)
}

func (h *Hosts) ForEach(f func(host string, cnt *Count)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for host, cnt := range h.m {
		f(host, cnt)
	}
}
