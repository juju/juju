// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver/common import if possible.

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
)

// EnvPayloads exposes the State functionality for payloads in an env.
type EnvPayloads interface {
	// ListAll returns information on the workload with the id on the unit.
	ListAll() ([]workload.FullPayloadInfo, error)
}

// PublicAPI serves payload-specific API methods.
type PublicAPI struct {
	// State exposes the workload aspect of Juju's state.
	State EnvPayloads
}

// NewHookContextAPI builds a new facade for the given State.
func NewPublicAPI(st EnvPayloads) *PublicAPI {
	return &PublicAPI{State: st}
}

// List builds the list of payloads being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// workloads for the unit are returned.
func (a PublicAPI) List(args api.EnvListArgs) (api.EnvListResults, error) {
	var r api.EnvListResults

	payloads, err := a.State.ListAll()
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}

	filters, err := workload.BuildPredicatesFor(args.Patterns)
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}
	payloads = workload.Filter(payloads, filters...)

	for _, payload := range payloads {
		apiInfo := api.Payload2api(payload)
		r.Results = append(r.Results, apiInfo)
	}
	return r, nil
}
