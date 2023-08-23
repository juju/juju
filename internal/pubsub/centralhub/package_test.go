// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package centralhub -destination gauge_mock_test.go github.com/juju/juju/internal/pubsub/centralhub GaugeVec
//go:generate go run go.uber.org/mock/mockgen -package centralhub -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Gauge

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
