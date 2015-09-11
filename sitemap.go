package crawler

import (
	"encoding/xml"
	"errors"
	"time"
)

type Freq struct {
	time.Duration
}

func (freq *Freq) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	switch s {
	case "":
		*freq = Freq{0}
	case "always":
		// Use second as the minimum unit of change frequence
		*freq = Freq{time.Second}
	case "hourly":
		*freq = Freq{time.Hour}
	case "daily":
		*freq = Freq{time.Hour * 24}
	case "weekly":
		*freq = Freq{time.Hour * 24 * 7}
	case "monthly":
		*freq = Freq{time.Hour * 24 * 30}
	case "yearly":
		*freq = Freq{time.Hour * 24 * 365}
	case "never":
		// time.Duration is int64
		*freq = Freq{1<<63 - 1}
	default:
		return errors.New("invalid frequence: " + s)
	}
	return nil
}

type Time struct {
	time.Time
}

func (t *Time) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04Z07:00",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01",
		"2006",
	}
	var s string
	var err error
	if err = d.DecodeElement(&s, &start); err != nil {
		return err
	}
	for _, layout := range layouts {
		if t.Time, err = time.Parse(layout, s); err == nil {
			break
		}
	}
	return err
}

type SiteURL struct {
	Loc          string  `xml:"loc"`
	LastModified Time    `xml:"lastmod"`
	ChangeFreq   Freq    `xml:"changefreq"`
	Priority     float64 `xml:"priority"`
}

type SiteMap struct {
	URLSet []SiteURL `xml:"url"`
}
