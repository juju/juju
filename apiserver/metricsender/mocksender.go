// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/metricsender/wireformat"
)

// MockSender implements the metric sender interface.
type MockSender struct {
	Data [][]*wireformat.MetricBatch
}

// Send implements the Send interface.
func (m *MockSender) Send(d []*wireformat.MetricBatch) (*wireformat.Response, error) {
	m.Data = append(m.Data, d)
	respUUID, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	var envResponses = make(wireformat.EnvironmentResponses)

	for _, batch := range d {
		envResponses.Ack(batch.EnvUUID, batch.UUID)
	}
	return &wireformat.Response{
		UUID:         respUUID.String(),
		EnvResponses: envResponses,
	}, nil
}
