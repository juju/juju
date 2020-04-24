// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

// API2ID converts the given payload tag string into a payload ID.
// Example: "payload-foobar" -> "foobar"
func API2ID(tagStr string) (string, error) {
	if tagStr == "" {
		return tagStr, nil
	}
	tag, err := names.ParsePayloadTag(tagStr)
	if err != nil {
		return "", errors.Trace(err)
	}
	return tag.Id(), nil
}

// Payload2api converts a payload.FullPayloadInfo struct into
// a Payload struct.
func Payload2api(p payload.FullPayloadInfo) params.Payload {
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

// API2Payload converts an API Payload info struct into
// a payload.FullPayloadInfo struct.
func API2Payload(apiInfo params.Payload) (payload.FullPayloadInfo, error) {
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
