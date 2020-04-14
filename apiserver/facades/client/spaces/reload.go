package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"gopkg.in/juju/names.v3"
)

// ReloadSpacesState contains all the methods required to execute the API.
type ReloadSpacesState interface {
	space.ReloadSpacesState
}

// ReloadSpacesEnviron contains the methods for requesting environ data.
type ReloadSpacesEnviron interface {
	environs.EnvironConfigGetter
}

// ReloadSpacesAPI provides the reload spaces API facade for version.
type ReloadSpacesAPI struct {
	state     ReloadSpacesState
	environs  ReloadSpacesEnviron
	context   context.ProviderCallContext
	authorize ReloadSpacesAuthorizer
}

// NewReloadSpacesAPI creates a new ReloadSpacesAPI.
func NewReloadSpacesAPI(state ReloadSpacesState,
	environs ReloadSpacesEnviron,
	context context.ProviderCallContext,
	authorizer ReloadSpacesAuthorizer,
) *ReloadSpacesAPI {
	return &ReloadSpacesAPI{
		state:     state,
		environs:  environs,
		context:   context,
		authorize: authorizer,
	}
}

// ReloadSpaces refreshes spaces from the substrate.
func (api *ReloadSpacesAPI) ReloadSpaces() error {
	if err := api.authorize(); err != nil {
		return errors.Trace(err)
	}
	env, err := environs.GetEnviron(api.environs, environs.New)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(space.ReloadSpaces(api.context, api.state, env))
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
		canWrite, err := auth.HasPermission(permission.WriteAccess, state.ModelTag())
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if !canWrite {
			return common.ServerError(common.ErrPerm)
		}
		if err := check.ChangeAllowed(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
}

// DefaultReloadSpacesEnvirons creates a new ReloadSpacesEnviron from state.
func DefaultReloadSpacesEnvirons(st *state.State) (stateenvirons.EnvironConfigGetter, error) {
	m, err := st.Model()
	if err != nil {
		return stateenvirons.EnvironConfigGetter{}, errors.Trace(err)
	}
	return stateenvirons.EnvironConfigGetter{
		Model: m,
	}, nil
}
