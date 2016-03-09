// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils"
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
		envResponses.Ack(batch.ModelUUID, batch.UUID)
	}
	return &wireformat.Response{
		UUID:         respUUID.String(),
		EnvResponses: envResponses,
	}, nil
}

// ErrorSender implements the metric sender interface and is used
// to return errors during testing
type ErrorSender struct {
	Err error
}

// Send implements the Send interface returning errors specified in the ErrorSender.
func (e *ErrorSender) Send(d []*wireformat.MetricBatch) (*wireformat.Response, error) {
	return &wireformat.Response{}, e.Err
}
