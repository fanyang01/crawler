package crawler

import (
	"bytes"
	"log"
	"net/url"

	"golang.org/x/net/html"
)

var (
	LinkBufSize = 64
)

func newURL(url *url.URL) *URL {
	return &URL{
		URL: url,
		Str: url.String(),
	}
}

func (base URL) parse(rawurl string) (*URL, error) {
	url, err := base.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	return &URL{
		URL: url,
		Str: url.String(),
	}, nil
}

func ParseLink(docs <-chan *Doc) <-chan *URL {
	urls := make(chan *URL, LinkBufSize)
	go parseLink(docs, urls)
	return urls
}

func parseLink(docs <-chan *Doc, urls chan<- *URL) {
	for doc := range docs {
		// NOTE: naive implemention, may result in too many goroutines
		go extractLink(doc, urls)
	}
}

func extractLink(doc *Doc, ch chan<- *URL) {
	tree, err := html.Parse(bytes.NewReader(doc.HTML))
	if err != nil {
		log.Printf("extractLink: %v\n", err)
		return
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if url, err := doc.baseURL.parse(a.Val); err == nil {
						ch <- url
					} else {
						log.Printf("extractLink: %v\n", err)
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(tree)
}
