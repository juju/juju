// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

// Payload2api converts a workload.FullPayloadInfo struct into
// a Payload struct.
func Payload2api(p workload.FullPayloadInfo) Payload {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return Payload{
		Class:   p.Name,
		Type:    p.Type,
		ID:      p.ID,
		Status:  p.Status,
		Tags:    tags,
		Unit:    names.NewUnitTag(p.Unit),
		Machine: names.NewMachineTag(p.Machine),
	}
}

// API2Payload converts an API Payload info struct into
// a workload.FullPayloadInfo struct.
func API2Payload(apiInfo Payload) workload.FullPayloadInfo {
	tags := make([]string, len(apiInfo.Tags))
	copy(tags, apiInfo.Tags)
	return workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: apiInfo.Class,
				Type: apiInfo.Type,
			},
			ID:     apiInfo.ID,
			Status: apiInfo.Status,
			Tags:   tags,
			Unit:   apiInfo.Unit.Id(),
		},
		Machine: apiInfo.Machine.Id(),
	}
}
