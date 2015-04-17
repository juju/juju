// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("HighAvailability", 1, NewHighAvailabilityAPI)
}

// HighAvailability defines the methods on the highavailability API end point.
type HighAvailability interface {
	EnsureAvailability(args params.StateServersSpecs) (params.StateServersChangeResults, error)
}

// HighAvailabilityAPI implements the HighAvailability interface and is the concrete
// implementation of the api end point.
type HighAvailabilityAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ HighAvailability = (*HighAvailabilityAPI)(nil)

// NewHighAvailabilityAPI creates a new server-side highavailability API end point.
func NewHighAvailabilityAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*HighAvailabilityAPI, error) {
	// Only clients and environment managers can access the high availability service.
	if !authorizer.AuthClient() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &HighAvailabilityAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

func (api *HighAvailabilityAPI) EnsureAvailability(args params.StateServersSpecs) (params.StateServersChangeResults, error) {
	results := params.StateServersChangeResults{Results: make([]params.StateServersChangeResult, len(args.Specs))}
	for i, stateServersSpec := range args.Specs {
		result, err := EnsureAvailabilitySingle(api.state, stateServersSpec)
		results.Results[i].Result = result
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// Convert machine ids to tags.
func machineIdsToTags(ids ...string) []string {
	var result []string
	for _, id := range ids {
		result = append(result, names.NewMachineTag(id).String())
	}
	return result
}

// Generate a StateServersChanges structure.
func stateServersChanges(change state.StateServersChanges) params.StateServersChanges {
	return params.StateServersChanges{
		Added:      machineIdsToTags(change.Added...),
		Maintained: machineIdsToTags(change.Maintained...),
		Removed:    machineIdsToTags(change.Removed...),
		Promoted:   machineIdsToTags(change.Promoted...),
		Demoted:    machineIdsToTags(change.Demoted...),
		Converted:  machineIdsToTags(change.Converted...),
	}
}

// EnsureAvailabilitySingle applies a single StateServersSpec specification to the current environment.
// Exported so it can be called by the legacy client API in the client package.
func EnsureAvailabilitySingle(st *state.State, spec params.StateServersSpec) (params.StateServersChanges, error) {
	if !st.IsStateServer() {
		return params.StateServersChanges{}, errors.New("unsupported with hosted environments")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.StateServersChanges{}, errors.Trace(err)
	}
	// Validate the environment tag if present.
	if spec.EnvironTag != "" {
		tag, err := names.ParseEnvironTag(spec.EnvironTag)
		if err != nil {
			return params.StateServersChanges{}, errors.Errorf("invalid environment tag: %v", err)
		}
		if _, err := st.FindEntity(tag); err != nil {
			return params.StateServersChanges{}, err
		}
	}

	series := spec.Series
	if series == "" {
		ssi, err := st.StateServerInfo()
		if err != nil {
			return params.StateServersChanges{}, err
		}

		// We should always have at least one voting machine
		// If we *really* wanted we could just pick whatever series is
		// in the majority, but really, if we always copy the value of
		// the first one, then they'll stay in sync.
		if len(ssi.VotingMachineIds) == 0 {
			// Better than a panic()?
			return params.StateServersChanges{}, fmt.Errorf("internal error, failed to find any voting machines")
		}
		templateMachine, err := st.Machine(ssi.VotingMachineIds[0])
		if err != nil {
			return params.StateServersChanges{}, err
		}
		series = templateMachine.Series()
	}
	changes, err := st.EnsureAvailability(spec.NumStateServers, spec.Constraints, series, spec.Placement)
	if err != nil {
		return params.StateServersChanges{}, err
	}
	return stateServersChanges(changes), nil
}
