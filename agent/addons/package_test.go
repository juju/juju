// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addons_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package addons_test -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer
//go:generate go run github.com/golang/mock/mockgen -package addons_test -destination engine_mock_test.go github.com/juju/juju/cmd/jujud/agent/engine MetricSink

func Test(t *testing.T) {
	gc.TestingT(t)
}
