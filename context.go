package crawler

import (
	"net/url"
	"time"

	"golang.org/x/net/context"
)

type ctxKey int

const (
	ckPreview ctxKey = 1 + iota
	ckLoaded
	ckDepth
	ckVisitCount
	ckErrorCount
	ckLastVisit
)

type Context struct {
	C        context.Context
	url      *url.URL
	response *Response
	cw       *Crawler
}

func newContext(cw *Crawler, u *url.URL) *Context {
	return &Context{
		url: u,
		cw:  cw,
		C:   context.Background(),
	}
}

func (c *Context) preview() []byte {
	return c.C.Value(ckPreview).([]byte)
}

func (c *Context) URL() *url.URL { return c.url }

func (c *Context) Depth() (depth int, err error) {
	err = c.fromStore()
	return c.Value(ckDepth).(int), err
}
func (c *Context) VisitCount() (cnt int, err error) {
	err = c.fromStore()
	return c.Value(ckVisitCount).(int), err
}
func (c *Context) ErrorCount() (cnt int, err error) {
	err = c.fromStore()
	return c.Value(ckErrorCount).(int), err
}
func (c *Context) LastVisit() (t time.Time, err error) {
	err = c.fromStore()
	return c.Value(ckLastVisit).(time.Time), err
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
	c.WithValue(ckVisitCount, u.VisitCount)
	c.WithValue(ckErrorCount, u.ErrorCount)
	c.WithValue(ckLastVisit, u.Last)
	c.WithValue(ckLoaded, true)
}

func (c *Context) WithValue(k, v interface{}) {
	c.C = context.WithValue(c.C, k, v)
}
func (c *Context) Value(k interface{}) interface{} {
	return c.C.Value(k)
}

func (c *Context) Reset() *Context {
	c.url = nil
	c.cw = nil
	c.C = context.Background()
	return c
}

func (c *Context) Response() *Response { return c.response }
