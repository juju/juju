// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"launchpad.net/juju-core/charm"
)

const DefaultSeries = "precise"

// Server is an http.Handler that serves the HTTP API of juju
// so that juju clients can retrieve published charms.
type Server struct {
	store *Store
	mux   *http.ServeMux
}

// New returns a new *Server using store.
func NewServer(store *Store) (*Server, error) {
	s := &Server{
		store: store,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("/charm-info", func(w http.ResponseWriter, r *http.Request) {
		s.serveInfo(w, r)
	})
	s.mux.HandleFunc("/charm-event", func(w http.ResponseWriter, r *http.Request) {
		s.serveEvent(w, r)
	})
	s.mux.HandleFunc("/charm/", func(w http.ResponseWriter, r *http.Request) {
		s.serveCharm(w, r)
	})
	s.mux.HandleFunc("/stats/counter/", func(w http.ResponseWriter, r *http.Request) {
		s.serveStats(w, r)
	})

	// This is just a validation key to allow blitz.io to run
	// performance tests against the site.
	s.mux.HandleFunc("/mu-35700a31-6bf320ca-a800b670-05f845ee", func(w http.ResponseWriter, r *http.Request) {
		s.serveBlitzKey(w, r)
	})
	return s, nil
}

// ServeHTTP serves an http request.
// This method turns *Server into an http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.Redirect(w, r, "https://juju.ubuntu.com", http.StatusSeeOther)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func statsEnabled(req *http.Request) bool {
	// It's fine to parse the form more than once, and it avoids
	// bugs from not parsing it.
	req.ParseForm()
	return req.Form.Get("stats") != "0"
}

func charmStatsKey(curl *charm.URL, kind string) []string {
	if curl.User == "" {
		return []string{kind, curl.Series, curl.Name}
	}
	return []string{kind, curl.Series, curl.Name, curl.User}
}

func (s *Server) resolveURL(url string) (*charm.URL, error) {
	ref, series, err := charm.ParseReference(url)
	if err != nil {
		return nil, err
	}
	if series == "" {
		prefSeries, err := s.store.Series(ref)
		if err != nil {
			return nil, err
		}
		if len(prefSeries) == 0 {
			return nil, ErrNotFound
		}
		return &charm.URL{Reference: ref, Series: prefSeries[0]}, nil
	}
	return &charm.URL{Reference: ref, Series: series}, nil
}

func (s *Server) serveInfo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/charm-info" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	r.ParseForm()
	response := map[string]*charm.InfoResponse{}
	for _, url := range r.Form["charms"] {
		c := &charm.InfoResponse{}
		response[url] = c
		curl, err := s.resolveURL(url)
		var info *CharmInfo
		if err == nil {
			info, err = s.store.CharmInfo(curl)
		}
		var skey []string
		if err == nil {
			skey = charmStatsKey(curl, "charm-info")
			c.CanonicalURL = curl.String()
			c.Sha256 = info.BundleSha256()
			c.Revision = info.Revision()
			c.Digest = info.Digest()
		} else {
			if err == ErrNotFound && curl != nil {
				skey = charmStatsKey(curl, "charm-missing")
			}
			c.Errors = append(c.Errors, err.Error())
		}
		if skey != nil && statsEnabled(r) {
			go s.store.IncCounter(skey)
		}
	}
	data, err := json.Marshal(response)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(data)
	}
	if err != nil {
		logger.Errorf("cannot write content: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *Server) serveEvent(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/charm-event" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	r.ParseForm()
	response := map[string]*charm.EventResponse{}
	for _, url := range r.Form["charms"] {
		digest := ""
		if i := strings.Index(url, "@"); i >= 0 && i+1 < len(url) {
			digest = url[i+1:]
			url = url[:i]
		}
		c := &charm.EventResponse{}
		response[url] = c
		curl, err := s.resolveURL(url)
		var event *CharmEvent
		if err == nil {
			event, err = s.store.CharmEvent(curl, digest)
		}
		var skey []string
		if err == nil {
			skey = charmStatsKey(curl, "charm-event")
			c.Kind = event.Kind.String()
			c.Revision = event.Revision
			c.Digest = event.Digest
			c.Errors = event.Errors
			c.Warnings = event.Warnings
			c.Time = event.Time.UTC().Format(time.RFC3339)
		} else {
			c.Errors = append(c.Errors, err.Error())
		}
		if skey != nil && statsEnabled(r) {
			go s.store.IncCounter(skey)
		}
	}
	data, err := json.Marshal(response)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(data)
	}
	if err != nil {
		logger.Errorf("cannot write content: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *Server) serveCharm(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/charm/") {
		panic("serveCharm: bad url")
	}
	curl, err := s.resolveURL("cs:" + r.URL.Path[len("/charm/"):])
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	info, rc, err := s.store.OpenCharm(curl)
	if err == ErrNotFound {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Errorf("cannot open charm %q: %v", curl, err)
		return
	}
	if statsEnabled(r) {
		go s.store.IncCounter(charmStatsKey(curl, "charm-bundle"))
	}
	defer rc.Close()
	w.Header().Set("Connection", "close") // No keep-alive for now.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.BundleSize(), 10))
	_, err = io.Copy(w, rc)
	if err != nil {
		logger.Errorf("failed to stream charm %q: %v", curl, err)
	}
}

func (s *Server) serveStats(w http.ResponseWriter, r *http.Request) {
	// TODO: Adopt a smarter mux that simplifies this logic.
	const dir = "/stats/counter/"
	if !strings.HasPrefix(r.URL.Path, dir) {
		panic("bad url")
	}
	base := r.URL.Path[len(dir):]
	if strings.Index(base, "/") > 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if base == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	r.ParseForm()
	var by CounterRequestBy
	switch v := r.Form.Get("by"); v {
	case "":
		by = ByAll
	case "day":
		by = ByDay
	case "week":
		by = ByWeek
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Invalid 'by' value: %q", v)))
		return
	}
	req := CounterRequest{
		Key:  strings.Split(base, ":"),
		List: r.Form.Get("list") == "1",
		By:   by,
	}
	if v := r.Form.Get("start"); v != "" {
		var err error
		req.Start, err = time.Parse("2006-01-02", v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("Invalid 'start' value: %q", v)))
			return
		}
	}
	if v := r.Form.Get("stop"); v != "" {
		var err error
		req.Stop, err = time.Parse("2006-01-02", v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("Invalid 'stop' value: %q", v)))
			return
		}
		// Cover all timestamps within the stop day.
		req.Stop = req.Stop.Add(24*time.Hour - 1*time.Second)
	}
	if req.Key[len(req.Key)-1] == "*" {
		req.Prefix = true
		req.Key = req.Key[:len(req.Key)-1]
		if len(req.Key) == 0 {
			// No point in counting something unknown.
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	var format func([]formatItem) []byte
	switch v := r.Form.Get("format"); v {
	case "":
		if !req.List && req.By == ByAll {
			format = formatCount
		} else {
			format = formatText
		}
	case "text":
		format = formatText
	case "csv":
		format = formatCSV
	case "json":
		format = formatJSON
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Invalid 'format' value: %q", v)))
		return
	}

	entries, err := s.store.Counters(&req)
	if err != nil {
		logger.Errorf("cannot query counters: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var buf []byte
	var items []formatItem
	for i := range entries {
		entry := &entries[i]
		if req.List {
			for j := range entry.Key {
				buf = append(buf, entry.Key[j]...)
				buf = append(buf, ':')
			}
			if entry.Prefix {
				buf = append(buf, '*')
			} else {
				buf = buf[:len(buf)-1]
			}
		}
		items = append(items, formatItem{string(buf), entry.Count, entry.Time})
		buf = buf[:0]
	}

	buf = format(items)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	_, err = w.Write(buf)
	if err != nil {
		logger.Errorf("cannot write content: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) serveBlitzKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", "2")
	w.Write([]byte("42"))
}

type formatItem struct {
	key   string
	count int64
	time  time.Time
}

func (fi *formatItem) hasKey() bool {
	return fi.key != ""
}

func (fi *formatItem) hasTime() bool {
	return !fi.time.IsZero()
}

func (fi *formatItem) formatTime() string {
	return fi.time.Format("2006-01-02")
}

func formatCount(items []formatItem) []byte {
	return strconv.AppendInt(nil, items[0].count, 10)
}

func formatText(items []formatItem) []byte {
	var maxKeyLength int
	for i := range items {
		if l := len(items[i].key); maxKeyLength < l {
			maxKeyLength = l
		}
	}
	spaces := make([]byte, maxKeyLength+2)
	for i := range spaces {
		spaces[i] = ' '
	}
	var buf []byte
	for i := range items {
		item := &items[i]
		if item.hasKey() {
			buf = append(buf, item.key...)
			buf = append(buf, spaces[len(item.key):]...)
		}
		if item.hasTime() {
			buf = append(buf, item.formatTime()...)
			buf = append(buf, ' ', ' ')
		}
		buf = strconv.AppendInt(buf, item.count, 10)
		buf = append(buf, '\n')
	}
	return buf
}

func formatCSV(items []formatItem) []byte {
	var buf []byte
	for i := range items {
		item := &items[i]
		if item.hasKey() {
			buf = append(buf, item.key...)
			buf = append(buf, ',')
		}
		if item.hasTime() {
			buf = append(buf, item.formatTime()...)
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, item.count, 10)
		buf = append(buf, '\n')
	}
	return buf
}

func formatJSON(items []formatItem) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}
	var buf []byte
	buf = append(buf, '[')
	for i := range items {
		item := &items[i]
		if i == 0 {
			buf = append(buf, '[')
		} else {
			buf = append(buf, ',', '[')
		}
		if item.hasKey() {
			buf = append(buf, '"')
			buf = append(buf, item.key...)
			buf = append(buf, '"', ',')
		}
		if item.hasTime() {
			buf = append(buf, '"')
			buf = append(buf, item.formatTime()...)
			buf = append(buf, '"', ',')
		}
		buf = strconv.AppendInt(buf, item.count, 10)
		buf = append(buf, ']')
	}
	buf = append(buf, ']')
	return buf
}
