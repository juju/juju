// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"context"

	"github.com/juju/juju/rpc/params"
)

// APIV1 serves payload-specific API methods.
type APIV1 struct{}

// NewAPIV1 builds a new facade for the given backend.
func NewAPIV1() *APIV1 {
	return &APIV1{}
}

// List builds the list of payloads being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (a APIV1) List(ctx context.Context, args params.PayloadListArgs) (params.PayloadListResults, error) {
	var r params.PayloadListResults
	return r, nil
}

// APIV2 blackholes the payload-specific API methods. This prevents any clients
// from accessing the API methods. A 3.x client will call the V1 API methods
// which returns an empty result.
type APIV2 struct{}

// NewAPIV1 builds a new facade for the given backend.
func NewAPIV2() *APIV2 {
	return &APIV2{}
}
