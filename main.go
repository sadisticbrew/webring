package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	_ "embed"

	"github.com/syumai/workers"
)

type WebringEntry struct {
	Name string `json:"name"`
	Url  string `json:"url"`
	Gh   string `json:"gh"`
}

//go:embed webring.json
var webringRaw []byte

//go:embed index.html
var indexHTML []byte

var hostsToIgnore = []string{"ring.seggs.lol", "seggs.lol", "www.seggs.lol"}

// initial returns the uppercased first character of s (for the letter avatar).
func initial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}

// host strips the scheme and a leading "www." from a url for display.
func host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// renderIndex injects the member cards into the embedded page once, at startup,
// so each request just writes static bytes (no per-request templating).
func renderIndex(webring []WebringEntry) []byte {
	var cards strings.Builder
	for _, e := range webring {
		name := html.EscapeString(e.Name)
		cards.WriteString(`<a class="card" href="`)
		cards.WriteString(html.EscapeString(e.Url))
		cards.WriteString(`" rel="noopener" data-name="`)
		cards.WriteString(name)
		cards.WriteString(`"><span class="avatar">`)
		cards.WriteString(html.EscapeString(initial(e.Name)))
		if e.Gh != "" {
			cards.WriteString(`<img src="https://avatars.githubusercontent.com/`)
			cards.WriteString(html.EscapeString(e.Gh))
			cards.WriteString(`?size=96" alt="" loading="lazy" onerror="this.setAttribute('data-failed','')" />`)
		}
		cards.WriteString(`</span><span class="meta"><span class="name">`)
		cards.WriteString(name)
		cards.WriteString(`</span><span class="host">`)
		cards.WriteString(html.EscapeString(host(e.Url)))
		cards.WriteString(`</span></span><span class="arrow">&rarr;</span></a>`)
	}

	out := strings.Replace(string(indexHTML), "<!--MEMBERS-->", cards.String(), 1)
	out = strings.Replace(out, "<!--COUNT-->", fmt.Sprintf("%d members", len(webring)), 1)
	return []byte(out)
}

func main() {
	var webring []WebringEntry
	if err := json.Unmarshal(webringRaw, &webring); err != nil {
		slog.Error("failed to unmarshal webring json file", "error", err)
		os.Exit(1)
	}

	page := renderIndex(webring)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reqHost := r.Host

		if strings.HasSuffix(reqHost, ".seggs.lol") && !slices.Contains(hostsToIgnore, reqHost) {
			sub := strings.TrimSuffix(reqHost, ".seggs.lol")
			for _, entry := range webring {
				if entry.Name == sub {
					http.Redirect(w, r, entry.Url, http.StatusFound)
					return
				}
			}

			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})

	http.HandleFunc("/webring", func(w http.ResponseWriter, r *http.Request) {
		buildJsonResponse(w, http.StatusOK, webring)
	})

	http.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		if from == "" {
			buildJsonResponse(w, http.StatusBadRequest, map[string]string{
				"error": "missing `from` query parameter",
			})
			return
		}

		dir := r.URL.Query().Get("dir")
		if dir == "" {
			buildJsonResponse(w, http.StatusBadRequest, map[string]string{
				"error": "missing `dir` query parameter",
			})
			return
		}

		if dir != "next" && dir != "prev" {
			buildJsonResponse(w, http.StatusBadRequest, map[string]string{
				"error": "invalid `dir` query parameter. it can be either `next` or `prev` only",
			})
			return
		}

		index := -1
		for i, v := range webring {
			if v.Name == from {
				index = i
				break
			}
		}

		if index == -1 {
			buildJsonResponse(w, http.StatusBadRequest, map[string]string{
				"error": "invalid `from` query parameter. can't find any webring entry's name as `from`",
			})
			return
		}

		url := ""

		if dir == "prev" {
			if index == 0 {
				url = webring[len(webring)-1].Url
			} else {
				url = webring[index-1].Url
			}
		} else {
			if index == len(webring)-1 {
				url = webring[0].Url
			} else {
				url = webring[index+1].Url
			}
		}

		setCorsHeaders(w)
		http.Redirect(w, r, url, http.StatusFound)
	})

	http.HandleFunc("/random", func(w http.ResponseWriter, r *http.Request) {
		setCorsHeaders(w)
		index := rand.Intn(len(webring))
		http.Redirect(w, r, webring[index].Url, http.StatusFound)
	})

	workers.Serve(nil)
	fmt.Println("server is up and running at :8080")
}

func setCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func buildJsonResponse(w http.ResponseWriter, statusCode int, v any) {
	setCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(v)
}
