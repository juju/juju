// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

func init() {
	// TODO(dimitern): Uncomment once *state.State implements Backing.
	// common.RegisterStandardFacade("Spaces", 1, NewAPI)
}

// API defines the methods the Spaces API facade implements.
type API interface {
}

// spacesAPI implements the API interface.
type spacesAPI struct {
	backing    common.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ API = (*spacesAPI)(nil)

// NewAPI creates a new server-side Subnets API facade.
func NewAPI(backing common.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &spacesAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}
