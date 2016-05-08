package crawler

import (
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context"
)

type ctxKey int

const (
	ckDepth ctxKey = 1 + iota
	ckLoaded
	ckNumVisit
	ckNumError
	ckLastTime
	ckError
)

type Context struct {
	cw  *Crawler
	url *url.URL
	err error
	C   context.Context
}

var (
	ctxFreeList = &sync.Pool{
		New: func() interface{} {
			return &Context{}
		},
	}
	emptyContext = Context{}
)

func (cw *Crawler) newContext(u *url.URL, ctx context.Context) *Context {
	c := ctxFreeList.Get().(*Context)
	c.cw = cw
	c.url = u
	c.C = ctx
	return c
}

func (c *Context) free() {
	*c = emptyContext
	ctxFreeList.Put(c)
}

func (c *Context) URL() *url.URL { return c.url }

func (c *Context) With(ctx context.Context) { c.C = ctx }

func (c *Context) WithValue(k, v interface{}) {
	c.C = context.WithValue(c.C, k, v)
}
func (c *Context) Value(k interface{}) interface{} {
	return c.C.Value(k)
}

func (c *Context) Retry(err error) {
	c.err = RetryableError{err}
}
func (c *Context) Error(err error) {
	c.err = err
}
func (c *Context) Fatal(err error) {
	c.err = FatalError{err}
}

func (c *Context) Depth() (depth int, err error) {
	depth, ok := c.Value(ckDepth).(int)
	if !ok {
		if depth, err = c.cw.store.GetDepth(c.url); err == nil {
			c.WithValue(ckDepth, depth)
		}
		return
	}
	return depth, nil
}
func (c *Context) NumVisit() (cnt int, err error) {
	if err = c.fromStore(); err == nil {
		cnt = c.Value(ckNumVisit).(int)
	}
	return
}
func (c *Context) NumError() (cnt int, err error) {
	if err = c.fromStore(); err == nil {
		cnt = c.Value(ckNumError).(int)
	}
	return
}
func (c *Context) LastTime() (t time.Time, err error) {
	if err = c.fromStore(); err == nil {
		t = c.Value(ckLastTime).(time.Time)
	}
	return
}

func (c *Context) fromStore() error {
	if loaded, ok := c.Value(ckLoaded).(bool); ok && loaded {
		return nil
	}
	u, err := c.cw.store.Get(c.url)
	if err != nil {
		return err
	}
	c.fromURL(u)
	return nil
}

func (c *Context) fromURL(u *URL) {
	c.WithValue(ckDepth, u.Depth)
	c.WithValue(ckNumVisit, u.NumVisit)
	c.WithValue(ckNumError, u.NumRetry)
	c.WithValue(ckLastTime, u.Last)
	c.WithValue(ckLoaded, true)
}
