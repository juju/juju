package store

import (
	"encoding/json"
	"io"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"net/http"
	"strconv"
	"strings"
)

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
		curl, err := charm.ParseURL(url)
		var info *CharmInfo
		if err == nil {
			info, err = s.store.CharmInfo(curl)
		}
		var skey []string
		if err == nil {
			skey = charmStatsKey(curl, "charm-info")
			c.Sha256 = info.BundleSha256()
			c.Revision = info.Revision()
			c.Digest = info.Digest()
		} else {
			if err == ErrNotFound {
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
		log.Errorf("store: cannot write content: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *Server) serveCharm(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/charm/") {
		panic("serveCharm: bad url")
	}
	curl, err := charm.ParseURL("cs:" + r.URL.Path[len("/charm/"):])
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
		log.Errorf("store: cannot open charm %q: %v", curl, err)
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
		log.Errorf("store: failed to stream charm %q: %v", curl, err)
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
	req := CounterRequest{
		Key:  strings.Split(base, ":"),
		List: r.Form.Get("list") == "1",
	}
	if req.Key[len(req.Key)-1] == "*" {
		req.Prefix = true
		req.Key = req.Key[:len(req.Key)-1]
		if len(req.Key) == 0 && !req.List {
			// No point in counting something unknown.
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	entries, err := s.store.Counters(&req)
	if err != nil {
		log.Errorf("store: cannot query counters: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// First build keys and figure max key length.
	var buf []byte
	var maxKeyLength int
	type resultItem struct {
		key   string
		count int64
	}
	var result []resultItem
	for i := range entries {
		entry := &entries[i]
		for j := range entry.Key {
			buf = append(buf, entry.Key[j]...)
			buf = append(buf, ':')
		}
		if entry.Prefix {
			buf = append(buf, '*')
		} else {
			buf = buf[:len(buf)-1]
		}
		if maxKeyLength < len(buf) {
			maxKeyLength = len(buf)
		}
		result = append(result, resultItem{string(buf), entry.Count})
		buf = buf[:0]
	}

	// Then join all keys and counts in a single formatted buffer.
	spaces := make([]byte, maxKeyLength+2)
	for i := range spaces {
		spaces[i] = ' '
	}
	for i := range result {
		item := &result[i]
		if req.List {
			buf = append(buf, item.key...)
			buf = append(buf, spaces[len(item.key):]...)
		}
		buf = strconv.AppendInt(buf, item.count, 10)
		if req.List {
			buf = append(buf, '\n')
		}
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", strconv.Itoa(len(buf)))
	_, err = w.Write(buf)
	if err != nil {
		log.Errorf("store: cannot write content: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *Server) serveBlitzKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", "2")
	w.Write([]byte("42"))
}
