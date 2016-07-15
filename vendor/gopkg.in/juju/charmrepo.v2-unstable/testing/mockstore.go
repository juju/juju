// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing // import "gopkg.in/juju/charmrepo.v2-unstable/testing"

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"gopkg.in/juju/charmrepo.v2-unstable"
)

var logger = loggo.GetLogger("juju.charm.testing.mockstore")

// MockStore provides a mock charm store implementation useful when testing.
type MockStore struct {
	mux          *http.ServeMux
	listener     net.Listener
	archiveBytes []byte
	// ArchiveSHA256 holds the hex-encoded SHA256 checksum
	// of the charm archive served by the mock store.
	ArchiveSHA256           string
	Downloads               []*charm.URL
	DownloadsNoStats        []*charm.URL
	Authorizations          []string
	Metadata                []string
	InfoRequestCount        int
	InfoRequestCountNoStats int
	DefaultSeries           string

	charms map[string]int
}

// NewMockStore creates a mock charm store containing the specified charms.
func NewMockStore(c *gc.C, repo *Repo, charms map[string]int) *MockStore {
	s := &MockStore{charms: charms, DefaultSeries: "precise"}
	f, err := os.Open(repo.CharmArchivePath(c.MkDir(), "dummy"))
	c.Assert(err, gc.IsNil)
	defer f.Close()
	buf := &bytes.Buffer{}
	s.ArchiveSHA256, _, err = utils.ReadSHA256(io.TeeReader(f, buf))
	c.Logf("ArchiveSHA256: %v", s.ArchiveSHA256)

	c.Assert(err, gc.IsNil)
	s.archiveBytes = buf.Bytes()
	c.Assert(err, gc.IsNil)
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/charm-info", s.serveInfo)
	s.mux.HandleFunc("/charm-event", s.serveEvent)
	s.mux.HandleFunc("/charm/", s.serveCharm)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, gc.IsNil)
	s.listener = lis
	go http.Serve(s.listener, s)
	return s
}

// Close closes the mock store's socket.
func (s *MockStore) Close() {
	s.listener.Close()
}

// Address returns the URL used to make requests to the mock store.
func (s *MockStore) Address() string {
	return "http://" + s.listener.Addr().String()
}

// UpdateStoreRevision sets the revision of the specified charm to rev.
func (s *MockStore) UpdateStoreRevision(ch string, rev int) {
	s.charms[ch] = rev
}

// ServeHTTP implements http.ServeHTTP
func (s *MockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *MockStore) serveInfo(w http.ResponseWriter, r *http.Request) {
	if metadata := r.Header.Get("Juju-Metadata"); metadata != "" {
		s.Metadata = append(s.Metadata, metadata)
		logger.Infof("Juju metadata: " + metadata)
	}

	r.ParseForm()
	if r.Form.Get("stats") == "0" {
		s.InfoRequestCountNoStats += 1
	} else {
		s.InfoRequestCount += 1
	}

	response := map[string]*charmrepo.InfoResponse{}
	for _, url := range r.Form["charms"] {
		cr := &charmrepo.InfoResponse{}
		response[url] = cr
		charmURL, err := charm.ParseURL(url)
		if err != nil {
			panic(err)
		}
		if charmURL.Series == "" {
			charmURL.Series = s.DefaultSeries
		}
		switch charmURL.Name {
		case "borken":
			cr.Errors = append(cr.Errors, "badness")
		case "terracotta":
			cr.Errors = append(cr.Errors, "cannot get revision")
		case "unwise":
			cr.Warnings = append(cr.Warnings, "foolishness")
			fallthrough
		default:
			if rev, ok := s.charms[charmURL.WithRevision(-1).String()]; ok {
				if charmURL.Revision == -1 {
					cr.Revision = rev
				} else {
					cr.Revision = charmURL.Revision
				}
				cr.Sha256 = s.ArchiveSHA256
				cr.CanonicalURL = charmURL.String()
			} else {
				cr.Errors = append(cr.Errors, "entry not found")
			}
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		panic(err)
	}
}

func (s *MockStore) serveEvent(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charmrepo.EventResponse{}
	for _, url := range r.Form["charms"] {
		digest := ""
		if i := strings.Index(url, "@"); i >= 0 {
			digest = url[i+1:]
			url = url[:i]
		}
		er := &charmrepo.EventResponse{}
		response[url] = er
		if digest != "" && digest != "the-digest" {
			er.Kind = "not-found"
			er.Errors = []string{"entry not found"}
			continue
		}
		charmURL := charm.MustParseURL(url)
		switch charmURL.Name {
		case "borken":
			er.Kind = "publish-error"
			er.Errors = append(er.Errors, "badness")
		case "unwise":
			er.Warnings = append(er.Warnings, "foolishness")
			fallthrough
		default:
			if rev, ok := s.charms[charmURL.WithRevision(-1).String()]; ok {
				er.Kind = "published"
				er.Revision = rev
				er.Digest = "the-digest"
			} else {
				er.Kind = "not-found"
				er.Errors = []string{"entry not found"}
			}
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		panic(err)
	}
}

func (s *MockStore) serveCharm(w http.ResponseWriter, r *http.Request) {
	charmURL := charm.MustParseURL("cs:" + r.URL.Path[len("/charm/"):])

	r.ParseForm()
	if r.Form.Get("stats") == "0" {
		s.DownloadsNoStats = append(s.DownloadsNoStats, charmURL)
	} else {
		s.Downloads = append(s.Downloads, charmURL)
	}

	if auth := r.Header.Get("Authorization"); auth != "" {
		s.Authorizations = append(s.Authorizations, auth)
	}

	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(s.archiveBytes)))
	_, err := w.Write(s.archiveBytes)
	if err != nil {
		panic(err)
	}
}
