// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"

	"github.com/juju/charm/v7"

	"github.com/juju/juju/payload"
)

func NewPayload(name, application string, machine, unit int, labels ...string) payload.FullPayloadInfo {
	if len(labels) == 0 {
		labels = nil
	}
	return payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: payload.StateRunning,
			Labels: labels,
			Unit:   fmt.Sprintf("%s/%d", application, unit),
		},
		Machine: fmt.Sprintf("%d", machine),
	}
}

func Formatted(payloads ...payload.FullPayloadInfo) []FormattedPayload {
	var formatted []FormattedPayload
	for _, payload := range payloads {
		formatted = append(formatted, FormatPayload(payload))
	}
	return formatted
}
