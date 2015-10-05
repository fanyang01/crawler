package task

import (
	"reflect"
	"testing"
	"time"
)

func TestTOML(t *testing.T) {
	cfg, err := ReadTask("test.toml")
	if err != nil {
		t.Fatal(err)
	}
	exp := &Task{
		Seed: []struct {
			URL       string
			AJAX      bool
			OtherHost bool
		}{
			{"http://localhost:6060", false, false},
		},
		Target: []struct {
			Pattern   string
			Frequence time.Duration
			Priority  float64
		}{
			{"http://localhost:6060/pkg/*", 20 * time.Second, 1},
		},
		Filter: []struct {
			Pattern string
			Score   int
			AJAX    bool
		}{
			{"http://localhost:6060/pkg*", 1024, false},
		},
	}
	if !reflect.DeepEqual(cfg, exp) {
		t.Errorf("expect %+v, got %+v", exp, cfg)
	}
}
