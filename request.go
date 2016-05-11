package crawler

import "net/http"

// Request is a HTTP request to be made.
type Request struct {
	*http.Request
	Client Client
	ctx    *Context
	cancel bool
}

func (r *Request) Context() *Context {
	return r.ctx
}
func (r *Request) Use(c Client) {
	r.Client = c
}
func (r *Request) AddCookie(c *http.Cookie) {
	r.Request.AddCookie(c)
}
func (r *Request) AddHeader(key, value string) {
	r.Header.Add(key, value)
}
func (r *Request) SetHeader(key, value string) {
	r.Header.Set(key, value)
}
func (r *Request) SetBasicAuth(usr, pwd string) {
	r.Request.SetBasicAuth(usr, pwd)
}
func (r *Request) SetReferer(url string) {
	r.Header.Set("Referer", url)
}
func (r *Request) SetUserAgent(agent string) {
	r.Header.Set("User-Agent", agent)
}

func (r *Request) Cancel() { r.cancel = true }
