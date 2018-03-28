// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp

import (
	"net/http"
	"sync"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
)

// Mux is a pattern-based HTTP muxer, based on top of
// bmizerany/pat, adding support for dynamic registration
// and deregistration of handlers. When a handler is
// added or removed, the underlying pat mux is swapped
// out with a new one.
//
// Adding and removing handlers is expensive: each of those
// calls will create a new mux underneath. These operations
// are not expected to be frequently occurring.
type Mux struct {
	pmu sync.Mutex
	p   *pat.PatternServeMux

	// mu protects added; added records the handlers
	// added by AddHandler, so we can recreate the
	// mux as necessary. The handlers are recorded
	// in the order they are added, per method, as
	// is done by pat.
	mu    sync.Mutex
	added map[string][]patternHandler

	// Clients who are using the mux can add themselves to prevent the
	// httpserver from stopping until they're done.
	clients sync.WaitGroup
}

type patternHandler struct {
	pat string
	h   http.Handler
}

// NewMux returns a new, empty mux.
func NewMux(opts ...muxOption) *Mux {
	m := &Mux{
		p:     pat.New(),
		added: make(map[string][]patternHandler),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

type muxOption func(*Mux)

// ServeHTTP is part of the http.Handler interface.
//
// ServeHTTP routes the request to a handler registered with
// AddHandler, according to the rules defined by bmizerany/pat.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.pmu.Lock()
	p := m.p
	m.pmu.Unlock()
	p.ServeHTTP(w, r)
}

// AddHandler adds an http.Handler for the given method and pattern.
// AddHandler returns an error if there already exists a handler for
// the method and pattern.
//
// This is safe to call concurrently with m.ServeHTTP and m.RemoveHandler.
func (m *Mux) AddHandler(meth, pat string, h http.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ph := range m.added[meth] {
		if ph.pat == pat {
			return errors.AlreadyExistsf("handler for %s %q", meth, pat)
		}
	}
	m.added[meth] = append(m.added[meth], patternHandler{pat, h})
	m.recreate()
	return nil
}

// RemoveHandler removes the http.Handler for the given method and pattern,
// if any. If there is no handler registered with the method and pattern,
// this is a no-op.
//
// This is safe to call concurrently with m.ServeHTTP and m.AddHandler.
func (m *Mux) RemoveHandler(meth, pat string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	phs, ok := m.added[meth]
	if !ok {
		return
	}
	for i, ph := range phs {
		if ph.pat != pat {
			continue
		}
		head, tail := phs[:i], phs[i+1:]
		m.added[meth] = append(head, tail...)
		m.recreate()
		return
	}
}

// AddClient tells the mux there's another client that should keep it
// alive.
func (m *Mux) AddClient() {
	m.clients.Add(1)
}

// ClientDone indicates that a client has finished and no longer needs
// the mux.
func (m *Mux) ClientDone() {
	m.clients.Done()
}

// Wait will block until all of the clients have indicated that
// they're done.
func (m *Mux) Wait() {
	m.clients.Wait()
}

func (m *Mux) recreate() {
	p := pat.New()
	for meth, phs := range m.added {
		for _, ph := range phs {
			p.Add(meth, ph.pat, ph.h)
		}
	}
	m.pmu.Lock()
	m.p = p
	m.pmu.Unlock()
}
