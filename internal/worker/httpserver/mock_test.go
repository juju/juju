// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/internal/testhelpers"
)

type stubPrometheusRegisterer struct {
	testhelpers.Stub
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
