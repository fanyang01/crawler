package crawler

import (
	"sync/atomic"
	"time"
)

type Statistic struct {
	Uptime time.Time
	All    int32
	Times  int32
	Errors int32
	Done   int32
}

func (s *Statistic) IncAllCount()   { atomic.AddInt32(&s.All, 1) }
func (s *Statistic) IncTimesCount() { atomic.AddInt32(&s.Times, 1) }
func (s *Statistic) IncErrorCount() { atomic.AddInt32(&s.Errors, 1) }
func (s *Statistic) IncDoneCount() (alldone bool) {
	return atomic.LoadInt32(&s.All) <= atomic.AddInt32(&s.Done, 1)
}
