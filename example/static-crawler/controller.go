package main

import (
	"bytes"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/download"
	"github.com/fanyang01/crawler/extract"
	"github.com/fanyang01/crawler/media"
	"github.com/fanyang01/crawler/ratelimit"
	"github.com/fanyang01/crawler/sample/count"
	"github.com/fanyang01/crawler/sample/fingerprint"
	"github.com/fanyang01/crawler/sample/urltrie"
	"gopkg.in/inconshreveable/log15.v2"
)

type Controller struct {
	crawler.NopController
	trie        *urltrie.Hosts
	count       *count.Hosts
	limiter     *ratelimit.Limiter
	extractor   *extract.Extractor
	downloader  *download.Downloader
	fingerprint *fingerprint.Store

	complete struct {
		hosts map[string]bool
		sync.RWMutex
	}
	logger log15.Logger
}

var freelist = download.NewFreeList(1<<20, 32)

func (c *Controller) Handle(r *crawler.Response, ch chan<- *url.URL) {
	var (
		html   = media.IsHTML(r.ContentType)
		body   = io.LimitReader(r.Body, 1<<20)
		logger = c.logger.New("url", r.URL)
		err    error
		buf    *bytes.Buffer
		tee    io.Reader
		uniq   bool
	)

	if !html {
		uniq = true
		if err = c.downloader.Handle(r.URL, body); err != nil {
			logger.Error("download", "err", err)
		}
		goto LOG
	}

	buf = freelist.Get()
	defer freelist.Put(buf)
	tee = io.TeeReader(body, buf)

	if uniq, err = c.fingerprint.Add(tee); err != nil {
		logger.Error("compute fingerprint", "err", err)
		goto LOG
	} else if uniq {
		if err = c.extractor.Extract(
			r, bytes.NewReader(buf.Bytes()), ch,
		); err != nil {
			logger.Error("extract links", "err", err)
			goto LOG
		}
		if err = c.downloader.Handle(r.URL, buf); err != nil {
			c.logger.Error("download", "err", err)
		}
	} else {
		if err = crawler.ExtractHref(r.NewURL, buf, ch); err != nil {
			logger.Error("extract hrefs", "err", err)
		}
	}

LOG:
	depth := r.Context().Depth()
	similar := !uniq
	c.logger.Info("",
		"url", r.URL, "depth", depth,
		"html", html, "similar", similar,
		"error", err != nil,
	)
	if err != nil {
		r.Context().Retry(err)
		return
	}

	var nresp, nsimilar int
	c.count.Update(r.URL.Host, func(cnt *count.Count) {
		cnt.Response.Count++
		cnt.Response.Depth[depth]++
		if similar {
			cnt.KV["SIMILAR"]++
		}
		nresp = cnt.Response.Count
		nsimilar = cnt.KV["SIMILAR"]
	})
	if nresp > 500 && nsimilar*2 > nresp {
		c.complete.Lock()
		c.complete.hosts[r.URL.Host] = true
		c.complete.Unlock()
	}
}

var htmlMatcher = extract.MustCompile(&extract.Pattern{
	File: []string{
		"", "*.?htm?", `/[^\.]*/`,
		`/.*\.(php|jsp|aspx|asp|cgi|do)/`,
	},
})

func (c *Controller) Accept(r *crawler.Response, u *url.URL) bool {
	if htmlMatcher.Match(u) {
		return c.trie.Add(u)
	}
	return true
}

func (c *Controller) Sched(r *crawler.Response, u *url.URL) crawler.Ticket {
	var depth int
	if r == nil {
		depth = 0
	} else {
		depth = r.Context().Depth() + 1
	}
	c.count.Update(u.Host, func(cnt *count.Count) {
		cnt.URL++
		cnt.Depth[depth]++
	})
	// d := c.limiter.Reserve(u)
	return crawler.Ticket{
		// rate limit queue takes over time scheduling.
		// At:    time.Now().Add(d),
		Score: 1000 - depth*100,
	}
}

func (c *Controller) Prepare(req *crawler.Request) {
	c.complete.RLock()
	complete := c.complete.hosts[req.Context().URL().Host]
	c.complete.RUnlock()
	if complete {
		req.Cancel()
	}
}

func (c *Controller) Interval(host string) time.Duration {
	c.complete.RLock()
	complete := c.complete.hosts[host]
	c.complete.RUnlock()
	if complete {
		return 0
	}
	return time.Second
}
