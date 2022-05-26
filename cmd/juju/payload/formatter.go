// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"github.com/juju/juju/core/payloads"
)

type listFormatter struct {
	payloads []payloads.FullPayloadInfo
}

func newListFormatter(payloads []payloads.FullPayloadInfo) *listFormatter {
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
func FormatPayload(payload payloads.FullPayloadInfo) FormattedPayload {
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
