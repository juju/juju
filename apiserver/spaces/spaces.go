// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

func init() {
	common.RegisterStandardFacade("Spaces", 1, NewAPI)
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// Name returns the space name.
	Name() string

	// TODO(dooferlad): Not sure if this should be subnets.BackingSubnetInfo.
	// It seems highly likely that the user will want to list the subnets in a
	// space, not just the CIDRs, but I guess that is a higher level operation?
	CIDRs() []string

	Type() string // TODO: convert to an enumerated type of IPv4 / IPv6. I can't find a standard go type or juju type for this
	ProviderID() string
	Zones() []string
	Status() string // TODO: convert to an enumerated type of InUse / NotInUse / Terminating

}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type Backing interface {
	// EnvironConfig returns the current environment config.
	EnvironConfig() (*config.Config, error)
}

// API defines the methods the Subnets API facade implements.
type API interface {
}

// subnetsAPI implements the API interface.
type subnetsAPI struct {
	backing    Backing
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ API = (*subnetsAPI)(nil)

// NewAPI creates a new server-side Subnets API facade.
func NewAPI(backing Backing, resources *common.Resources, authorizer common.Authorizer) (API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &subnetsAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}
