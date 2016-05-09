package urlx

import "net/url"

// TODO: copy from net/url?
// var (
// 	freelist = &sync.Pool{
// 		New: func() interface{} {
// 			return &url.URL{}
// 		},
// 	}
// 	emptyURL = url.URL{}
// )

// func New() *url.URL {
// 	return freelist.Get().(*url.URL)
// }
// func Free(u *url.URL) {
// 	*u = emptyURL
// 	freelist.Put(u)
// }

func Parse(s string, f ...func(*url.URL) error) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	for _, ff := range f {
		if err = ff(u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func ParseRef(base *url.URL, s string, f ...func(*url.URL) error) (*url.URL, error) {
	u, err := base.Parse(s)
	if err != nil {
		return nil, err
	}
	for _, ff := range f {
		if err = ff(u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

// URL is a wrapper of url.URL. Some URLs contains non UTF-8 escaped
// characters, which will be changed after JSON marshalling/unmarshalling.
// This helper type intends to be used to marshal/unmarshal URLs safely.
type URL struct {
	*url.URL
}

func (u *URL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}
func (u *URL) UnmarshalText(b []byte) error {
	var err error
	u.URL, err = Parse(string(b))
	return err
}
