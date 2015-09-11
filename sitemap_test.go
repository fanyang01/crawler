package crawler

import (
	"encoding/xml"
	"reflect"
	"testing"
	"time"
)

func TestSiteMap(t *testing.T) {
	XML := `
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">

<url>

	<loc>http://www.example.com/</loc>

	<lastmod>2005-01-01</lastmod>

	<changefreq>monthly</changefreq>

	<priority>0.8</priority>

</url>

<url>

	<loc>http://www.example.com/catalog?item=12&amp;desc=vacation_hawaii</loc>

	<changefreq>weekly</changefreq>

</url>

<url>

	<loc>http://www.example.com/catalog?item=73&amp;desc=vacation_new_zealand</loc>

	<lastmod>2004-12-23</lastmod>

	<changefreq>weekly</changefreq>

</url>

<url>

	<loc>http://www.example.com/catalog?item=74&amp;desc=vacation_newfoundland</loc>

	<lastmod>2004-12-23T18:00:15+00:00</lastmod>

	<priority>0.3</priority>

</url>

<url>

	<loc>http://www.example.com/catalog?item=83&amp;desc=vacation_usa</loc>

	<lastmod>2004-11-23</lastmod>

</url>

</urlset>
`
	expected := SiteMap{
		URLSet: []SiteURL{
			{
				Loc:          "http://www.example.com/",
				LastModified: Time{time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)},
				ChangeFreq:   Freq{30 * 24 * time.Hour},
				Priority:     0.8,
			},
			{
				Loc:        "http://www.example.com/catalog?item=12&desc=vacation_hawaii",
				ChangeFreq: Freq{7 * 24 * time.Hour},
			},
			{
				Loc:          "http://www.example.com/catalog?item=73&desc=vacation_new_zealand",
				LastModified: Time{time.Date(2004, 12, 23, 0, 0, 0, 0, time.UTC)},
				ChangeFreq:   Freq{7 * 24 * time.Hour},
			},
			{
				Loc:          "http://www.example.com/catalog?item=74&desc=vacation_newfoundland",
				LastModified: Time{mustParseTime(time.RFC3339, "2004-12-23T18:00:15+00:00")},
				Priority:     0.3,
			},
			{
				Loc:          "http://www.example.com/catalog?item=83&desc=vacation_usa",
				LastModified: Time{time.Date(2004, 11, 23, 0, 0, 0, 0, time.UTC)},
			},
		},
	}
	var sm SiteMap
	err := xml.Unmarshal([]byte(XML), &sm)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sm, expected) {
		t.Fatalf("expect:\n%v\n, got:\n%v\n", expected, sm)
	}
}

func mustParseTime(layout, v string) time.Time {
	if t, err := time.Parse(layout, v); err != nil {
		panic(err)
	} else {
		return t
	}
}
