// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"

	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

func NewPayload(name, service string, machine, unit int, tags ...string) workload.Payload {
	if len(tags) == 0 {
		tags = nil
	}
	return workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: "docker",
		},
		ID:      "id" + name,
		Status:  workload.StateRunning,
		Tags:    tags,
		Machine: fmt.Sprintf("%d", machine),
		Unit:    fmt.Sprintf("unit-%s-%d", service, unit),
	}
}

func Formatted(payloads ...workload.Payload) []FormattedPayload {
	var formatted []FormattedPayload
	for _, payload := range payloads {
		formatted = append(formatted, FormatPayload(payload))
	}
	return formatted
}

func FormatPayload(payload workload.Payload) FormattedPayload {
	return FormattedPayload{
		Unit:    payload.Unit,
		Machine: payload.Machine,
		ID:      payload.ID,
		Type:    payload.Type,
		Class:   payload.Name,
		Tags:    payload.Tags,
		Status:  payload.Status,
	}
}
