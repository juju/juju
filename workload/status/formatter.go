// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/workload"
)

type listFormatter struct {
	payloads      []workload.Payload
	compatVersion int
}

func newListFormatter(payloads []workload.Payload, compatVersion int) *listFormatter {
	lf := listFormatter{
		payloads:      payloads,
		compatVersion: compatVersion,
	}
	return &lf
}

func (lf *listFormatter) format() []formattedPayload {
	if lf.payloads == nil {
		return nil
	}

	var formatted []formattedPayload
	for _, payload := range lf.payloads {
		formatted = append(formatted, lf.formatPayload(payload))
	}
	return formatted
}

func (lf *listFormatter) formatPayload(payload workload.Payload) formattedPayload {
	var tags []string
	if len(payload.Tags) > 0 {
		tags = make([]string, len(payload.Tags))
		copy(tags, payload.Tags)
	}
	return formattedPayload{
		Unit:    payload.Unit,
		Machine: payload.Machine,
		ID:      payload.ID,
		Type:    payload.Type,
		Class:   payload.Name,
		Tags:    tags,
		// TODO(ericsnow) Explicitly convert to a string?
		Status: payload.Status,
	}
}
