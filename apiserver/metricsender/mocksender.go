// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"github.com/juju/juju/apiserver/metricsender/wireformat"
)

// MockSender implements the metric sender interface.
type MockSender struct {
	Data [][]*wireformat.MetricBatch
}

// Send implements the Send interface.
func (m *MockSender) Send(d []*wireformat.MetricBatch) error {
	m.Data = append(m.Data, d)
	return nil
}
