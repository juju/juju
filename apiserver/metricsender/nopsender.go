// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/metricsender/wireformat"
)

// NopSender is a sender that acts like everything worked fine
// But doesn't do anything.
type NopSender struct {
}

// Implement the send interface, act like everything is fine.
func (n NopSender) Send(batches []*wireformat.MetricBatch) (*wireformat.Response, error) {
	var resp = make(wireformat.EnvironmentResponses)
	for _, batch := range batches {
		resp.Ack(batch.EnvUUID, batch.UUID)
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	return &wireformat.Response{UUID: uuid.String(), EnvResponses: resp}, nil
}
