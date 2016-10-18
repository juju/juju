// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/payload"
)

type listFormatter struct {
	payloads []payload.FullPayloadInfo
}

func newListFormatter(payloads []payload.FullPayloadInfo) *listFormatter {
	lf := listFormatter{
		payloads: payloads,
	}
	return &lf
}

func (lf *listFormatter) format() []FormattedPayload {
	if lf.payloads == nil {
		return nil
	}

	var formatted []FormattedPayload
	for _, payload := range lf.payloads {
		formatted = append(formatted, FormatPayload(payload))
	}
	return formatted
}

// FormatPayload converts the Payload into a FormattedPayload.
func FormatPayload(payload payload.FullPayloadInfo) FormattedPayload {
	var labels []string
	if len(payload.Labels) > 0 {
		labels = make([]string, len(payload.Labels))
		copy(labels, payload.Labels)
	}
	return FormattedPayload{
		Unit:    payload.Unit,
		Machine: payload.Machine,
		ID:      payload.ID,
		Type:    payload.Type,
		Class:   payload.Name,
		Labels:  labels,
		// TODO(ericsnow) Explicitly convert to a string?
		Status: payload.Status,
	}
}
