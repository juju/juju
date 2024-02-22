// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// ReloadSpacesState contains all the methods required to execute the API.
type ReloadSpacesState interface {
	space.ReloadSpacesState
	Model() (Model, error)
}

type reloadSpacesStateShim struct {
	*state.State
}

func (s reloadSpacesStateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return &modelShim{Model: m}, nil
}

type modelShim struct {
	*state.Model
}

func (s modelShim) Config() (*config.Config, error) {
	return s.Config()
}

// ReloadSpacesEnviron contains the methods for requesting environ data.
type ReloadSpacesEnviron interface {
	environs.EnvironConfigGetter

	// GetEnviron returns the environs.Environ ("provider") associated
	// with the model.
	GetEnviron(stdcontext.Context, environs.EnvironConfigGetter, environs.NewEnvironFunc) (environs.Environ, error)
}

// EnvironSpaces defines methods for handling spaces within a environ setting.
type EnvironSpaces interface {
	// ReloadSpaces loads spaces and subnets from provider specified by environ
	// into state.
	// Currently it's an append-only operation, no spaces/subnets are deleted.
	ReloadSpaces(envcontext.ProviderCallContext, ReloadSpacesState, space.SpaceService, environs.BootstrapEnviron, network.FanConfig) error
}

// ReloadSpacesAPI provides the reload spaces API facade for version.
type ReloadSpacesAPI struct {
	state                       ReloadSpacesState
	environs                    ReloadSpacesEnviron
	spaces                      EnvironSpaces
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter
	authorize                   ReloadSpacesAuthorizer
	spaceService                common.SpaceService
}

// NewReloadSpacesAPI creates a new ReloadSpacesAPI.
func NewReloadSpacesAPI(
	state ReloadSpacesState,
	environs ReloadSpacesEnviron,
	spaces EnvironSpaces,
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter,
	authorizer ReloadSpacesAuthorizer,
	spaceService common.SpaceService,
) *ReloadSpacesAPI {
	return &ReloadSpacesAPI{
		state:                       state,
		environs:                    environs,
		spaces:                      spaces,
		credentialInvalidatorGetter: credentialInvalidatorGetter,
		authorize:                   authorizer,
		spaceService:                spaceService,
	}
}

// ReloadSpaces refreshes spaces from the substrate.
func (api *ReloadSpacesAPI) ReloadSpaces(ctx stdcontext.Context) error {
	if err := api.authorize(ctx); err != nil {
		return errors.Trace(err)
	}
	env, err := api.environs.GetEnviron(ctx, api.environs, environs.New)
	if err != nil {
		return errors.Trace(err)
	}
	invalidatorFunc, err := api.credentialInvalidatorGetter()
	if err != nil {
		return errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)

	// TODO(nvinuesa): Fans should be retrieved from the model config
	// service once it's finished instead of legacy state.
	model, err := api.state.Model()
	if err != nil {
		return errors.Trace(err)
	}
	modelConfig, err := model.Config()
	if err != nil {
		return errors.Trace(err)
	}
	fanConfig, err := modelConfig.FanConfig()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(api.spaces.ReloadSpaces(callCtx, api.state, api.spaceService, env, fanConfig))
}

// ReloadSpacesAuthorizer represents a way to authorize reload spaces.
type ReloadSpacesAuthorizer func(stdcontext.Context) error

// AuthorizerState contains the methods used from state to authorize API
// requests.
type AuthorizerState interface {
	// ModelTag returns the tag of this model.
	ModelTag() names.ModelTag
}

// DefaultReloadSpacesAuthorizer creates a new ReloadSpacesAuthorizer for
// handling reload spaces.
func DefaultReloadSpacesAuthorizer(
	auth facade.Authorizer,
	check BlockChecker,
	state AuthorizerState,
) ReloadSpacesAuthorizer {
	return func(ctx stdcontext.Context) error {
		err := auth.HasPermission(permission.WriteAccess, state.ModelTag())
		if err != nil {
			return err
		}
		if err := check.ChangeAllowed(ctx); err != nil {
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
func (ReloadSpacesEnvirons) GetEnviron(ctx stdcontext.Context, st environs.EnvironConfigGetter, fn environs.NewEnvironFunc) (environs.Environ, error) {
	return environs.GetEnviron(ctx, st, fn)
}

// DefaultReloadSpacesEnvirons creates a new ReloadSpacesEnviron from state.
func DefaultReloadSpacesEnvirons(st *state.State, cloudService common.CloudService, credentialService common.CredentialService) (ReloadSpacesEnvirons, error) {
	m, err := st.Model()
	if err != nil {
		return ReloadSpacesEnvirons{}, errors.Trace(err)
	}
	return ReloadSpacesEnvirons{
		EnvironConfigGetter: stateenvirons.EnvironConfigGetter{
			Model:             m,
			CloudService:      cloudService,
			CredentialService: credentialService,
		},
	}, nil
}

// EnvironSpacesAdaptor allows the calling of ReloadSpaces from a type level,
// instead of a package level construct.
type EnvironSpacesAdaptor struct{}

// ReloadSpaces loads spaces and subnets from provider specified by environ
// into state.
// Currently it's an append-only operation, no spaces/subnets are deleted.
func (EnvironSpacesAdaptor) ReloadSpaces(
	ctx envcontext.ProviderCallContext,
	st ReloadSpacesState,
	spaceService space.SpaceService,
	env environs.BootstrapEnviron,
	fanConfig network.FanConfig,
) error {
	return space.ReloadSpaces(ctx, st, spaceService, env, fanConfig)
}
