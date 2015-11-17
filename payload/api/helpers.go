// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
)

// Payload2api converts a payload.FullPayloadInfo struct into
// a Payload struct.
func Payload2api(p payload.FullPayloadInfo) Payload {
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

	return Payload{
		Class:   p.Name,
		Type:    p.Type,
		ID:      p.ID,
		Status:  p.Status,
		Labels:  labels,
		Unit:    unitTag,
		Machine: machineTag,
	}
}

// API2Payload converts an API Payload info struct into
// a payload.FullPayloadInfo struct.
func API2Payload(apiInfo Payload) (payload.FullPayloadInfo, error) {
	labels := make([]string, len(apiInfo.Labels))
	copy(labels, apiInfo.Labels)

	var unit, machine string
	var empty payload.FullPayloadInfo
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

	return payload.FullPayloadInfo{
		Payload: payload.Payload{
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
