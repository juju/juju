// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
)

type MockStore struct {
	mux            *http.ServeMux
	Lis            net.Listener
	bundleBytes    []byte
	bundleSha256   string
	Downloads      []*charm.URL
	Authorizations []string

	charms map[string]int
}

func NewMockStore(c *gc.C, charms map[string]int) *MockStore {
	s := &MockStore{charms: charms}
	bytes, err := ioutil.ReadFile(testing.Charms.BundlePath(c.MkDir(), "dummy"))
	c.Assert(err, gc.IsNil)
	s.bundleBytes = bytes
	h := sha256.New()
	h.Write(bytes)
	s.bundleSha256 = hex.EncodeToString(h.Sum(nil))
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/charm-info", func(w http.ResponseWriter, r *http.Request) {
		s.ServeInfo(w, r)
	})
	s.mux.HandleFunc("/charm-event", func(w http.ResponseWriter, r *http.Request) {
		s.ServeEvent(w, r)
	})
	s.mux.HandleFunc("/charm/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeCharm(w, r)
	})
	lis, err := net.Listen("tcp", "127.0.0.1:4444")
	c.Assert(err, gc.IsNil)
	s.Lis = lis
	go http.Serve(s.Lis, s)
	return s
}

func (s *MockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *MockStore) ServeInfo(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charm.InfoResponse{}
	for _, url := range r.Form["charms"] {
		cr := &charm.InfoResponse{}
		response[url] = cr
		charmURL := charm.MustParseURL(url)
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
				cr.Sha256 = s.bundleSha256
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

func (s *MockStore) ServeEvent(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charm.EventResponse{}
	for _, url := range r.Form["charms"] {
		digest := ""
		if i := strings.Index(url, "@"); i >= 0 {
			digest = url[i+1:]
			url = url[:i]
		}
		er := &charm.EventResponse{}
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

func (s *MockStore) ServeCharm(w http.ResponseWriter, r *http.Request) {
	charmURL := charm.MustParseURL("cs:" + r.URL.Path[len("/charm/"):])
	s.Downloads = append(s.Downloads, charmURL)

	if auth := r.Header.Get("Authorization"); auth != "" {
		s.Authorizations = append(s.Authorizations, auth)
	}

	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(s.bundleBytes)))
	_, err := w.Write(s.bundleBytes)
	if err != nil {
		panic(err)
	}
}
