// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// ReloadSpacesState contains all the methods required to execute the API.
type ReloadSpacesState interface {
	space.ReloadSpacesState
}

// ReloadSpacesEnviron contains the methods for requesting environ data.
type ReloadSpacesEnviron interface {
	environs.EnvironConfigGetter

	// GetEnviron returns the environs.Environ ("provider") associated
	// with the model.
	GetEnviron(environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error)
}

// EnvironSpaces defines methods for handling spaces within a environ setting.
type EnvironSpaces interface {
	// ReloadSpaces loads spaces and subnets from provider specified by environ
	// into state.
	// Currently it's an append-only operation, no spaces/subnets are deleted.
	ReloadSpaces(context.ProviderCallContext, ReloadSpacesState, environs.BootstrapEnviron) error
}

// ReloadSpacesAPI provides the reload spaces API facade for version.
type ReloadSpacesAPI struct {
	state     ReloadSpacesState
	environs  ReloadSpacesEnviron
	spaces    EnvironSpaces
	context   context.ProviderCallContext
	authorize ReloadSpacesAuthorizer
}

// NewReloadSpacesAPI creates a new ReloadSpacesAPI.
func NewReloadSpacesAPI(state ReloadSpacesState,
	environs ReloadSpacesEnviron,
	spaces EnvironSpaces,
	context context.ProviderCallContext,
	authorizer ReloadSpacesAuthorizer,
) *ReloadSpacesAPI {
	return &ReloadSpacesAPI{
		state:     state,
		environs:  environs,
		spaces:    spaces,
		context:   context,
		authorize: authorizer,
	}
}

// ReloadSpaces refreshes spaces from the substrate.
func (api *ReloadSpacesAPI) ReloadSpaces() error {
	if err := api.authorize(); err != nil {
		return errors.Trace(err)
	}
	env, err := api.environs.GetEnviron(api.environs, environs.New)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(api.spaces.ReloadSpaces(api.context, api.state, env))
}

// ReloadSpacesAuthorizer represents a way to authorize reload spaces.
type ReloadSpacesAuthorizer func() error

// AuthorizerState contains the methods used from state to authorize API
// requests.
type AuthorizerState interface {
	// ModelTag returns the tag of this model.
	ModelTag() names.ModelTag
}

// DefaultReloadSpacesAuthorizer creates a new ReloadSpacesAuthorizer for
// handling reload spaces.
func DefaultReloadSpacesAuthorizer(auth facade.Authorizer,
	check BlockChecker,
	state AuthorizerState,
) ReloadSpacesAuthorizer {
	return func() error {
		err := auth.HasPermission(permission.WriteAccess, state.ModelTag())
		if err != nil {
			return err
		}
		if err := check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
}

// ReloadSpacesEnvirons returns a reload spaces environs type.
type ReloadSpacesEnvirons struct {
	stateenvirons.EnvironConfigGetter
}

// GetEnviron returns the environs.Environ ("provider") associated
// with the model.
func (ReloadSpacesEnvirons) GetEnviron(st environs.EnvironConfigGetter, fn environs.NewEnvironFunc) (environs.Environ, error) {
	return environs.GetEnviron(st, fn)
}

// DefaultReloadSpacesEnvirons creates a new ReloadSpacesEnviron from state.
func DefaultReloadSpacesEnvirons(st *state.State) (ReloadSpacesEnvirons, error) {
	m, err := st.Model()
	if err != nil {
		return ReloadSpacesEnvirons{}, errors.Trace(err)
	}
	return ReloadSpacesEnvirons{
		stateenvirons.EnvironConfigGetter{
			Model: m,
		},
	}, nil
}

// EnvironSpacesAdapter allows the calling of ReloadSpaces from a type level,
// instead of a package level construct.
type EnvironSpacesAdapter struct{}

// ReloadSpaces loads spaces and subnets from provider specified by environ
// into state.
// Currently it's an append-only operation, no spaces/subnets are deleted.
func (EnvironSpacesAdapter) ReloadSpaces(ctx context.ProviderCallContext, st ReloadSpacesState, env environs.BootstrapEnviron) error {
	return space.ReloadSpaces(ctx, st, env)
}
