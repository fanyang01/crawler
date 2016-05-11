package main

import (
	"fmt"
	"net/http"
	"text/tabwriter"

	"github.com/fanyang01/crawler/sample/count"
)

func handleCount(rw http.ResponseWriter, req *http.Request) {
	w := new(tabwriter.Writer)
	// Format in tab-separated columns with a tab stop of 8.
	w.Init(rw, 0, 8, 0, '\t', 0)
	fmt.Fprintln(w, "Host\tURL\tResp\tSimilar\t.")
	ctrl.count.ForEach(func(host string, cnt *count.Count) {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t.\n",
			host, cnt.URL, cnt.Response.Count, cnt.KV["SIMILAR"])
	})
	fmt.Fprintln(w)
	w.Flush()
}
