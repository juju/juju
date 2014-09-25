// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsender

import (
	"github.com/juju/juju/state"
)

// NopSender is a sender that acts like everything worked fine
// But doesn't do anything.
type NopSender struct {
}

// Implement the send interface, act like everything is fine.
func (n *NopSender) Send([]*state.MetricBatch) error {
	return nil
}
