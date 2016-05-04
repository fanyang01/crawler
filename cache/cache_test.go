package cache

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()
	data := []struct {
		r  *http.Response
		cc *Control
	}{
		{
			r:  &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": []string{"no-store"}}},
			cc: nil,
		}, {
			r:  &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": []string{"max-age=100"}}},
			cc: &Control{MaxAge: 100 * time.Second, CacheType: CacheNormal, Date: now, Timestamp: now},
		}, {
			r:  &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": []string{"no-cache, max-age=100"}}},
			cc: &Control{MaxAge: 0, CacheType: CacheNeedValidate, Date: now, Timestamp: now},
		},
	}
	for _, v := range data {
		cc := Parse(v.r, now)
		if v.cc == nil {
			assert.Nil(cc)
			continue
		}
		assert.NotNil(cc)
		assert.Equal(v.cc.Timestamp, cc.Timestamp)
		assert.Equal(v.cc.Date, cc.Date)
		assert.Equal(v.cc.MaxAge, cc.MaxAge)
		assert.Equal(v.cc.CacheType, cc.CacheType)
	}
}
