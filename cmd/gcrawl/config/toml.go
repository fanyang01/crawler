package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Seed []struct {
		URL       string
		AJAX      bool // dynamic page?
		OtherHost bool
	}
	Target []struct {
		Pattern   string        // [ wildcard | /regexp/ ]
		Frequence time.Duration // [ once | 30s |  1h10m | ... ]
		Priority  float64       // 0 - 1.0
	}
	Filter []struct {
		Pattern string
		Score   int // 0 - 1024
		AJAX    bool
	}
	Store     string        // [ file | db | none ]
	Frequence time.Duration // default frequence
}

func readConfig(fpath string) (*Config, error) {
	// For error message
	type (
		seed struct {
			URL       string
			AJAX      bool
			OtherHost bool
		}
		target struct {
			Pattern   pattern
			Frequence duration
			Priority  priority
		}
		filter struct {
			Pattern pattern
			Score   score
			AJAX    bool
		}
	)
	type conf struct {
		Seed      []seed
		Target    []target
		Filter    []filter
		Store     store
		Frequence duration
	}
	var cf conf
	md, err := toml.DecodeFile(fpath, &cf)
	if err != nil {
		return nil, err
	}
	if !md.IsDefined("store") || !md.IsDefined("Store") {
		cf.Store = store("none")
	}
	if !md.IsDefined("frequence") || !md.IsDefined("Frequence") {
		cf.Frequence.Duration = 0
	}

	var cfg Config
	for _, seed := range cf.Seed {
		cfg.Seed = append(cfg.Seed, seed)
	}
	for _, target := range cf.Target {
		cfg.Target = append(cfg.Target, struct {
			Pattern   string
			Frequence time.Duration
			Priority  float64
		}{
			Pattern:   string(target.Pattern),
			Frequence: target.Frequence.Duration,
			Priority:  float64(target.Priority),
		})
	}
	for _, ft := range cf.Filter {
		cfg.Filter = append(cfg.Filter, struct {
			Pattern string
			Score   int
			AJAX    bool
		}{
			Pattern: string(ft.Pattern),
			Score:   int(ft.Score),
			AJAX:    ft.AJAX,
		})
	}
	return &cfg, nil
}

type pattern string
type store string
type duration struct{ time.Duration }
type score int
type priority float64

func (p *pattern) UnmarshalText(s []byte) error {
	if bytes.HasPrefix(s, []byte{'/'}) && bytes.HasSuffix(s, []byte{'/'}) {
		if _, err := regexp.Compile(string(s[1 : len(s)-1])); err != nil {
			return err
		}
	}
	*p = pattern(string(s))
	return nil
}
func (d *duration) UnmarshalText(s []byte) error {
	if bytes.Equal(s, []byte("once")) {
		d.Duration = 0
		return nil
	}
	var err error
	d.Duration, err = time.ParseDuration(string(s))
	return err
}
func (s *score) UnmarshalTOML(v interface{}) error {
	var i int64
	var ok bool
	if i, ok = v.(int64); !ok {
		return fmt.Errorf("can't convert '%+v' to int64", v)
	}
	if i < 0 || i > 1024 {
		return errors.New("integer out of range [0, 1024]")
	}
	*s = score(i)
	return nil
}
func (p *priority) UnmarshalTOML(v interface{}) error {
	var f float64
	var ok bool
	if f, ok = v.(float64); !ok {
		return fmt.Errorf("can't convert '%+v' to float64", v)
	}
	if f < 0 || f > 1 {
		return errors.New("float number out of range [0, 1]")
	}
	*p = priority(f)
	return nil
}
func (st *store) UnmarshalText(s []byte) error {
	switch string(s) {
	case "db", "file", "none":
		*st = store(string(s))
	case "":
		*st = "db"
	default:
		return errors.New("invalid store: " + string(s))
	}
	return nil
}
