// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2"

	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
	appver "gopkg.in/juju/charmstore.v5-unstable/version"
)

// GET /debug/info .
func serveDebugInfo(http.Header, *http.Request) (interface{}, error) {
	return appver.VersionInfo, nil
}

// GET /debug/check.
func debugCheck(checks map[string]func() error) http.Handler {
	return router.HandleJSON(func(http.Header, *http.Request) (interface{}, error) {
		n := len(checks)
		type result struct {
			name string
			err  error
		}
		c := make(chan result)
		for name, check := range checks {
			name, check := name, check
			go func() {
				c <- result{name: name, err: check()}
			}()
		}
		results := make(map[string]string, n)
		var failed bool
		for ; n > 0; n-- {
			res := <-c
			if res.err == nil {
				results[res.name] = "OK"
			} else {
				failed = true
				results[res.name] = res.err.Error()
			}
		}
		if failed {
			keys := make([]string, 0, len(results))
			for k := range results {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			msgs := make([]string, len(results))
			for i, k := range keys {
				msgs[i] = fmt.Sprintf("[%s: %s]", k, results[k])
			}
			return nil, errgo.Newf("check failure: %s", strings.Join(msgs, " "))
		}
		return results, nil
	})
}

func checkDB(db *mgo.Database) func() error {
	return func() error {
		s := db.Session.Copy()
		s.SetSyncTimeout(500 * time.Millisecond)
		defer s.Close()
		return s.Ping()
	}
}

func checkES(si *SearchIndex) func() error {
	if si == nil || si.Database == nil {
		return func() error {
			return nil
		}
	}
	return func() error {
		_, err := si.Health()
		return err
	}
}

// GET /debug/fullcheck
func debugFullCheck(hnd http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		code := http.StatusInternalServerError
		resp := new(bytes.Buffer)
		defer func() {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(code)
			resp.WriteTo(w)
		}()

		fmt.Fprintln(resp, "Testing v4...")

		// test search
		fmt.Fprintln(resp, "performing search...")
		var sr params.SearchResponse
		if err := get(hnd, "/v4/search?limit=2000", &sr); err != nil {
			fmt.Fprintf(resp, "ERROR: search failed %s.\n", err)
			return
		}
		if len(sr.Results) < 1 {
			fmt.Fprintln(resp, "ERROR: no search results found.")
			return
		}
		fmt.Fprintf(resp, "%d results found.\n", len(sr.Results))

		// pick random charm
		id := sr.Results[rand.Intn(len(sr.Results))].Id
		fmt.Fprintf(resp, "using %s.\n", id)

		// test content
		fmt.Fprintln(resp, "reading manifest...")
		url := "/v4/" + id.Path() + "/meta/manifest"
		fmt.Fprintln(resp, url)
		var files []params.ManifestFile
		if err := get(hnd, url, &files); err != nil {
			fmt.Fprintf(resp, "ERROR: cannot retrieve manifest: %s.\n", err)
			return
		}
		if len(files) == 0 {
			fmt.Fprintln(resp, "ERROR: manifest empty.")
			return
		}
		fmt.Fprintf(resp, "%d files found.\n", len(files))

		// Choose a file to access
		expectFile := "metadata.yaml"
		if id.Series == "bundle" {
			expectFile = "bundle.yaml"
		}
		var file params.ManifestFile
		// default to metadata.yaml
		for _, f := range files {
			if f.Name == expectFile {
				file = f
				break
			}
		}
		// find a random file
		for i := 0; i < 5; i++ {
			f := files[rand.Intn(len(files))]
			if f.Size <= 16*1024 {
				file = f
				break
			}
		}
		fmt.Fprintf(resp, "using %s.\n", file.Name)

		// read the file
		fmt.Fprintln(resp, "reading file...")
		url = "/v4/" + id.Path() + "/archive/" + file.Name
		fmt.Fprintln(resp, url)
		var buf []byte
		if err := get(hnd, url, &buf); err != nil {
			fmt.Fprintf(resp, "ERROR: cannot retrieve file: %s.\n", err)
			return
		}
		if int64(len(buf)) != file.Size {
			fmt.Fprintf(resp, "ERROR: incorrect file size, expected: %d, received %d.\n", file.Size, len(buf))
			return
		}
		fmt.Fprintf(resp, "%d bytes received.\n", len(buf))

		// check if the charm is promulgated
		fmt.Fprintln(resp, "checking promulgated...")
		url = "/v4/" + id.Path() + "/meta/promulgated"
		fmt.Fprintln(resp, url)
		var promulgated params.PromulgatedResponse
		if err := get(hnd, url, &promulgated); err != nil {
			fmt.Fprintf(resp, "ERROR: cannot retrieve promulgated: %s.\n", err)
			return
		}
		if promulgated.Promulgated != (id.User == "") {
			fmt.Fprintf(resp, "ERROR: incorrect promulgated response, expected: %v, received %v.\n", (id.User == ""), promulgated.Promulgated)
			return
		}
		fmt.Fprintf(resp, "promulgated: %v.\n", promulgated.Promulgated)

		// check expand-id
		fmt.Fprintln(resp, "checking expand-id...")
		url = "/v4/" + id.Path() + "/expand-id"
		fmt.Fprintln(resp, url)
		var expanded []params.ExpandedId
		if err := get(hnd, url, &expanded); err != nil {
			fmt.Fprintf(resp, "ERROR: cannot expand-id: %s.\n", err)
			return
		}
		if len(expanded) == 0 {
			fmt.Fprintln(resp, "ERROR: expand-id returned 0 results")
			return
		}
		fmt.Fprintf(resp, "%d ids found.\n", len(expanded))

		code = http.StatusOK
	})
}

func newServiceDebugHandler(p *Pool, c ServerParams, hnd http.Handler) http.Handler {
	mux := router.NewServeMux()
	mux.Handle("/info", router.HandleJSON(serveDebugInfo))
	mux.Handle("/check", debugCheck(map[string]func() error{
		"mongodb":       checkDB(p.db.Database),
		"elasticsearch": checkES(p.es),
	}))
	mux.Handle("/fullcheck", authorized(c, debugFullCheck(hnd)))
	return mux
}

func authorized(c ServerParams, h http.Handler) http.Handler {
	return router.HandleErrors(func(w http.ResponseWriter, r *http.Request) error {
		u, p, err := utils.ParseBasicAuthHeader(r.Header)
		if err != nil {
			return errgo.WithCausef(err, params.ErrUnauthorized, "")
		}
		if u != c.AuthUsername || p != c.AuthPassword {
			return errgo.WithCausef(nil, params.ErrUnauthorized, "username or password mismatch")
		}
		h.ServeHTTP(w, r)
		return nil
	})
}

func get(h http.Handler, url string, body interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return errgo.Notef(err, "cannot create request")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		if w.HeaderMap.Get("Content-Type") != "application/json" {
			return errgo.Newf("bad status %d", w.Code)
		}
		var e params.Error
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			return errgo.Notef(err, "cannot decode error")
		}
		return &e
	}
	if body == nil {
		return nil
	}
	if bytes, ok := body.(*[]byte); ok {
		*bytes = w.Body.Bytes()
		return nil
	}
	if w.HeaderMap.Get("Content-Type") == "application/json" {
		if err := json.Unmarshal(w.Body.Bytes(), body); err != nil {
			return errgo.Notef(err, "cannot decode body")
		}
		return nil
	}
	return errgo.Newf("cannot decode body")
}
