package crawler

import (
	"bufio"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inconshreveable/log15"
)

type godocController struct {
	NopController
	t   *testing.T
	pkg map[string]int
	sync.Mutex
}

func (c *godocController) Handle(r *Response, ch chan<- *Link) {
	if strings.HasPrefix(r.URL.Path, "/pkg/") {
		pkg := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/pkg/"), "/")
		if pkg != "" { // http://localhost:6060/pkg/
			c.Lock()
			c.pkg[pkg]++
			c.Unlock()
		}
	}
	err := ExtractHref(r.NewURL, r.Body, ch)
	if err != nil {
		c.t.Error(err)
	}
}

func (c *godocController) Accept(_ *Response, l *Link) bool {
	return l.URL.Host == "localhost:34567" && strings.HasPrefix(l.URL.Path, "/pkg/")
}

func TestGodoc(t *testing.T) {
	godoc := exec.Command("godoc", "-http", ":34567")
	godoc.Env = []string{}
	if err := godoc.Start(); err != nil {
		t.Fatal(err)
	}
	defer godoc.Process.Kill()

	cmd := exec.Command("go", "list", "std")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	pkg := map[string]int{}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		pkg[scanner.Text()]++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	var cnt int
	for {
		if _, err := http.Get("http://localhost:34567"); err == nil {
			break
		}
		time.Sleep(time.Second)
		if cnt++; cnt > 5 {
			t.Fatal("godoc timeout")
		}
	}

	g := &godocController{
		t:   t,
		pkg: make(map[string]int),
	}
	cw := NewCrawler(&Config{
		Controller: g,
	})
	cw.Logger().SetHandler(log15.StdoutHandler)
	if err := cw.Crawl("http://localhost:34567"); err != nil {
		t.Fatal(err)
	}
	cw.Wait()

	// remove internal and vendor packages
	f := func(m map[string]int) {
		for k := range m {
			if strings.Contains(k, "internal") ||
				strings.Contains(k, "vendor") {
				delete(m, k)
			}
		}
	}
	f(pkg)
	f(g.pkg)
	for _, s := range []string{
		"container",
		"index",
		"runtime/msan",
		"text",
		"database",
		"compress",
		"archive",
		"go",
		"builtin",
		"debug",
	} {
		// if _, ok := g.pkg[s]; !ok {
		// 	t.Errorf("expect %q in crawled packages", s)
		// }
		delete(g.pkg, s)
	}
	if !reflect.DeepEqual(pkg, g.pkg) {
		t.Errorf("packages: expect\n%v,\ngot\n%v\n", pkg, g.pkg)
		for k := range pkg {
			if _, ok := g.pkg[k]; !ok {
				t.Log("+go list std:", k)
			}
		}
		for k := range g.pkg {
			if _, ok := pkg[k]; !ok {
				t.Log("+crawled packages:", k)
			}
		}
	}
}
