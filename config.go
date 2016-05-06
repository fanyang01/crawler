package crawler

import (
	"net/url"

	"github.com/fanyang01/crawler/queue"
	"github.com/fanyang01/crawler/urlx"
	"github.com/inconshreveable/log15"
)

type Config struct {
	Controller   Controller
	Store        Store
	Queue        queue.WaitQueue
	Logger       log15.Logger
	NormalizeURL func(*url.URL) error
	Option       *Option
}

var (
	DefaultController = NopController{}
)

func initConfig(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Option == nil {
		cfg.Option = DefaultOption
	}
	if cfg.Store == nil {
		cfg.Store = NewMemStore()
	}
	if cfg.Queue == nil {
		cfg.Queue = NewMemQueue(64 << 10)
	}
	if cfg.Controller == nil {
		cfg.Controller = DefaultController
	}
	if cfg.Logger == nil {
		cfg.Logger = log15.New()
		cfg.Logger.SetHandler(log15.DiscardHandler())
	}
	if cfg.NormalizeURL == nil {
		cfg.NormalizeURL = urlx.Normalize
	}
	return cfg
}
