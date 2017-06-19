// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadscommon

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

// PayloadInfoToParams converts a juju/payload into
// an apiserver/params struct.
func PayloadInfoToParams(p payload.FullPayloadInfo) params.Payload {
	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	var unitTag string
	if p.Unit != "" {
		unitTag = names.NewUnitTag(p.Unit).String()
	}
	var machineTag string
	if p.Machine != "" {
		machineTag = names.NewMachineTag(p.Machine).String()
	}

	return params.Payload{
		Class:   p.Name,
		Type:    p.Type,
		ID:      p.ID,
		Status:  p.Status,
		Labels:  labels,
		Unit:    unitTag,
		Machine: machineTag,
	}
}
