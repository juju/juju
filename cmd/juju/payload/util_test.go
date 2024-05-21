// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"fmt"

	"github.com/juju/juju/internal/charm"

	"github.com/juju/juju/core/payloads"
)

func NewPayload(name, application string, machine, unit int, labels ...string) payloads.FullPayloadInfo {
	if len(labels) == 0 {
		labels = nil
	}
	return payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: payloads.StateRunning,
			Labels: labels,
			Unit:   fmt.Sprintf("%s/%d", application, unit),
		},
		Machine: fmt.Sprintf("%d", machine),
	}
}

func Formatted(payloads ...payloads.FullPayloadInfo) []FormattedPayload {
	var formatted []FormattedPayload
	for _, payload := range payloads {
		formatted = append(formatted, FormatPayload(payload))
	}
	return formatted
}
