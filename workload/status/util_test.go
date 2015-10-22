// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"

	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

func NewPayload(name, service string, machine, unit int, labels ...string) workload.FullPayloadInfo {
	if len(labels) == 0 {
		labels = nil
	}
	return workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: workload.StateRunning,
			Labels: labels,
			Unit:   fmt.Sprintf("%s/%d", service, unit),
		},
		Machine: fmt.Sprintf("%d", machine),
	}
}

func Formatted(payloads ...workload.FullPayloadInfo) []FormattedPayload {
	var formatted []FormattedPayload
	for _, payload := range payloads {
		formatted = append(formatted, FormatPayload(payload))
	}
	return formatted
}
