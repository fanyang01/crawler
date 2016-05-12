package main

import (
	"bytes"
	"io"
	"net/url"
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
	c.count.Update(r.URL.Host, func(cnt *count.Count) {
		cnt.Response.Count++
		cnt.Response.Depth[depth]++
		if similar {
			cnt.KV["SIMILAR"]++
		}
	})
}

func (c *Controller) Accept(r *crawler.Response, u *url.URL) bool {
	return r.URL.Host == u.Host && c.trie.Add(u)
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
	d := c.limiter.Reserve(u)
	return crawler.Ticket{At: time.Now().Add(d)}
}

func (c *Controller) Prepare(req *crawler.Request) {
	var resp, similar int
	host := req.Context().URL().Host
	c.count.Update(host, func(cnt *count.Count) {
		resp = cnt.Response.Count
		similar = cnt.KV["SIMILAR"]
	})
	if resp > 400 && similar*2 > resp {
		req.Cancel()
	}
}
