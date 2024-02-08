// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/state"
)

// MockSender implements the metric sender interface.
type MockSender struct {
	UnackedBatches      map[string]struct{}
	Data                [][]*wireformat.MetricBatch
	MeterStatusResponse string
}

// Send implements the Send interface.
func (m *MockSender) Send(ctx context.Context, d []*wireformat.MetricBatch) (*wireformat.Response, error) {
	m.Data = append(m.Data, d)
	respUUID, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	var envResponses = make(wireformat.EnvironmentResponses)

	for _, batch := range d {
		if m.UnackedBatches != nil {
			_, ok := m.UnackedBatches[fmt.Sprintf("%s/%s", batch.ModelUUID, batch.UUID)]
			if ok {
				continue
			}
		}
		envResponses.Ack(batch.ModelUUID, batch.UUID)
		envResponses.SetModelStatus(batch.ModelUUID, m.MeterStatusResponse, "mocked response")
	}
	return &wireformat.Response{
		UUID:         respUUID.String(),
		EnvResponses: envResponses,
	}, nil
}

func (m *MockSender) IgnoreBatches(batches ...*state.MetricBatch) {
	if m.UnackedBatches == nil {
		m.UnackedBatches = make(map[string]struct{})
	}
	for _, batch := range batches {
		m.UnackedBatches[fmt.Sprintf("%s/%s", batch.ModelUUID(), batch.UUID())] = struct{}{}
	}
}

// ErrorSender implements the metric sender interface and is used
// to return errors during testing
type ErrorSender struct {
	Err error
}

// Send implements the Send interface returning errors specified in the ErrorSender.
func (e *ErrorSender) Send(ctx context.Context, d []*wireformat.MetricBatch) (*wireformat.Response, error) {
	return &wireformat.Response{}, e.Err
}
