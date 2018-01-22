// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/bmizerany/pat"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

type authKey struct{}

var (
	errNoAuthFunc = errors.New("no authentication handler found")
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
	// p is accessed atomically.
	p *pat.PatternServeMux

	auth authFunc

	// mu protects added; added records the handlers
	// added by AddHandler, so we can recreate the
	// mux as necessary. The handlers are recorded
	// in the order they are added, per method, as
	// is done by pat.
	mu    sync.Mutex
	added map[string][]patternHandler
}

type authFunc func(*http.Request) (AuthInfo, error)

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

// WithAuth returns a Mux constructor option function,
// which, when applied to a Mux, will configure the Mux
// to use the given function for authentication.
func WithAuth(f func(req *http.Request) (AuthInfo, error)) muxOption {
	return func(m *Mux) {
		m.auth = f
	}
}

// ServeHTTP is part of the http.Handler interface.
//
// ServeHTTP routes the request to a handler registered with
// AddHandler, according to the rules defined by bmizerany/pat.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := (*pat.PatternServeMux)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&m.p))))
	if m.auth != nil {
		ctx := r.Context()
		ctx = context.WithValue(ctx, authKey{}, m.auth)
		r = r.WithContext(ctx)
	}
	p.ServeHTTP(w, r)
}

// Authenticate checks the request for Juju entity credentials, and
// returns the tag of the authenticated entity, or an error if the
// authentication failed.
//
// The request must be one being handled by an http.Handler
// registered with this mux's AddHandler.
func (m *Mux) Authenticate(req *http.Request) (AuthInfo, error) {
	ctx := req.Context()
	if v := ctx.Value(authKey{}); v != nil {
		f := v.(authFunc)
		return f(req)
	}
	return AuthInfo{}, errNoAuthFunc
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

func (m *Mux) recreate() {
	p := pat.New()
	for meth, phs := range m.added {
		for _, ph := range phs {
			p.Add(meth, ph.pat, ph.h)
		}
	}
	atomic.StorePointer((*unsafe.Pointer)((unsafe.Pointer)(&m.p)), unsafe.Pointer(p))
}

// Auth is returned by Mux.Authenticate to identify the
// authenticated entity.
type AuthInfo struct {
	// Tag holds the tag of the authenticated entity.
	Tag names.Tag

	// Controller holds a boolean value indicating
	// whether or not the authenticated entity is a
	// controller agent.
	Controller bool
}
