// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addons_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package addons_test -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer
//go:generate go run go.uber.org/mock/mockgen -typed -package addons_test -destination engine_mock_test.go github.com/juju/juju/agent/engine MetricSink

func Test(t *testing.T) {
	tc.TestingT(t)
}
