package crawler

import (
	"net/url"
	"time"

	"golang.org/x/net/context"
)

type ctxKey int

const (
	ckLoaded ctxKey = 1 + iota
	ckDepth
	ckVisitCount
	ckErrorCount
	ckLastVisit
	ckError
)

type Context struct {
	cw  *Crawler
	url *url.URL
	err error
	C   context.Context
}

func (cw *Crawler) newContext(u *url.URL, ctx context.Context) *Context {
	return &Context{
		cw:  cw,
		url: u,
		C:   ctx,
	}
}

func (c *Context) URL() *url.URL { return c.url }

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
func (c *Context) VisitCount() (cnt int, err error) {
	if err = c.fromStore(); err == nil {
		cnt = c.Value(ckVisitCount).(int)
	}
	return
}
func (c *Context) ErrorCount() (cnt int, err error) {
	if err = c.fromStore(); err == nil {
		cnt = c.Value(ckErrorCount).(int)
	}
	return
}
func (c *Context) LastVisit() (t time.Time, err error) {
	if err = c.fromStore(); err == nil {
		t = c.Value(ckLastVisit).(time.Time)
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
	c.WithValue(ckVisitCount, u.NumVisit)
	c.WithValue(ckErrorCount, u.NumError)
	c.WithValue(ckLastVisit, u.Last)
	c.WithValue(ckLoaded, true)
}

func (c *Context) WithValue(k, v interface{}) {
	c.C = context.WithValue(c.C, k, v)
}
func (c *Context) Value(k interface{}) interface{} {
	return c.C.Value(k)
}

var emptyContext = Context{}

func (c *Context) Reset() *Context {
	*c = emptyContext
	c.C = context.Background()
	return c
}

func (c *Context) Error(err error) {
	c.err = err
}
func (c *Context) Retry(err error) {
	c.err = wrapRetriable(err)
}
