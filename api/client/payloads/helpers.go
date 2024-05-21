// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
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

// Payload2api converts a payloads.FullPayloadInfo struct into
// a Payload struct.
func Payload2api(p payloads.FullPayloadInfo) params.Payload {
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
// a payloads.FullPayloadInfo struct.
func API2Payload(apiInfo params.Payload) (payloads.FullPayloadInfo, error) {
	labels := make([]string, len(apiInfo.Labels))
	copy(labels, apiInfo.Labels)

	var unit, machine string
	var empty payloads.FullPayloadInfo
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

	return payloads.FullPayloadInfo{
		Payload: payloads.Payload{
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
