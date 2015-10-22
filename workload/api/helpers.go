// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

// Payload2api converts a workload.FullPayloadInfo struct into
// a Payload struct.
func Payload2api(p workload.FullPayloadInfo) Payload {
	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	unitTag := names.NewUnitTag(p.Unit)
	machineTag := names.NewMachineTag(p.Machine)

	return Payload{
		Class:   p.Name,
		Type:    p.Type,
		ID:      p.ID,
		Status:  p.Status,
		Labels:  labels,
		Unit:    unitTag.String(),
		Machine: machineTag.String(),
	}
}

// API2Payload converts an API Payload info struct into
// a workload.FullPayloadInfo struct.
func API2Payload(apiInfo Payload) (workload.FullPayloadInfo, error) {
	labels := make([]string, len(apiInfo.Labels))
	copy(labels, apiInfo.Labels)

	var unit, machine string
	var empty workload.FullPayloadInfo
	if apiInfo.Unit != "" {
		tag, err := names.ParseUnitTag(apiInfo.Unit)
		if err != nil {
			return empty, errors.Trace(err)
		}
		unit = tag.Id()
	}
	if apiInfo.Machine != "" {
		tag, err := names.ParseMachineTag(apiInfo.Machine)
		if err != nil {
			return empty, errors.Trace(err)
		}
		machine = tag.Id()
	}

	return workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: apiInfo.Class,
				Type: apiInfo.Type,
			},
			ID:     apiInfo.ID,
			Status: apiInfo.Status,
			Labels: labels,
			Unit:   unit,
		},
		Machine: machine,
	}, nil
}
