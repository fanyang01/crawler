package crawler

import (
	"errors"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

func NewProxyClient(addr string) (*http.Client, error) {
	return parseProxy(addr)
}

func parseProxy(addr string) (*http.Client, error) {
	u, err := ParseURL(addr)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		transport := &http.Transport{
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		return &http.Client{
			Transport: transport,
		}, nil
	case "http", "https":
		transport := &http.Transport{
			Proxy: http.ProxyURL(u),
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		return &http.Client{
			Transport: transport,
		}, nil
	default:
		return nil, errors.New("unsupported proxy type")
	}
}
