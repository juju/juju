// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/state"
)

// MockSender implements the metric sender interface.
type MockSender struct {
	Data []*state.MetricBatch
}

func (m *MockSender) Send(d []*state.MetricBatch) error {
	m.Data = append(m.Data, d...)
	return nil
}
