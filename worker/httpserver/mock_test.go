// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"

	"github.com/juju/testing"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/state"
)

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

type stubPrometheusRegisterer struct {
	testing.Stub
}

func (s *stubPrometheusRegisterer) MustRegister(...prometheus.Collector) {
	panic("should not be called")
}

func (s *stubPrometheusRegisterer) Register(c prometheus.Collector) error {
	s.MethodCall(s, "Register", c)
	return s.NextErr()
}

func (s *stubPrometheusRegisterer) Unregister(c prometheus.Collector) bool {
	s.MethodCall(s, "Unregister", c)
	return false
}

type stubCertWatcher struct {
	testing.Stub
	cert tls.Certificate
}

func (w *stubCertWatcher) get() *tls.Certificate {
	w.MethodCall(w, "get")
	return &w.cert
}

type mockLocalMacaroonAuthenticator struct {
	httpcontext.LocalMacaroonAuthenticator
}
