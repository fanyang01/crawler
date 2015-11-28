package crawler

import (
	"sync"
	"sync/atomic"
	"time"
)

type Statistic struct {
	Uptime time.Time

	mutex  sync.Mutex
	URLs   int32
	Finish int32

	Ntimes int32
	Errors int32
}

func (s *Statistic) IncNtimes() { atomic.AddInt32(&s.Ntimes, 1) }
func (s *Statistic) IncErrors() { atomic.AddInt32(&s.Errors, 1) }

func (s *Statistic) IncURL() {
	s.mutex.Lock()
	s.URLs++
	s.mutex.Unlock()
}

func (s *Statistic) IncFinish() (alldone bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Finish++
	return s.Finish >= s.URLs
}
