package crawler

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

type Proxy struct {
	direct, proxy *http.Client
}

func NewProxy(addr string) (*Proxy, error) {
	this := &Proxy{
		direct: DefaultHTTPClient,
	}
	if err := this.parse(addr); err != nil {
		return nil, err
	}
	return this, nil
}

func (p *Proxy) parse(addr string) error {
	u, err := url.Parse(addr)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "socks5":
		forward := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		var auth *proxy.Auth
		if u.User != nil {
			auth = &proxy.Auth{}
			auth.User = u.User.Username()
			auth.Password, _ = u.User.Password()
		}
		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, forward)
		if err != nil {
			return err
		}
		transport := &http.Transport{
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		p.proxy = &http.Client{
			Transport: transport,
		}
	case "http", "https":
		transport := &http.Transport{
			Proxy: http.ProxyURL(u),
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		p.proxy = &http.Client{
			Transport: transport,
		}
	default:
		return errors.New("unsupported proxy type")
	}
	return nil
}
