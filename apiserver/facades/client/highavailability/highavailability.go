// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.highavailability")

// HighAvailability defines the methods on the highavailability API end point.
type HighAvailability interface {
	EnableHA(args params.ControllersSpecs) (params.ControllersChangeResults, error)
}

// HighAvailabilityAPI implements the HighAvailability interface and is the concrete
// implementation of the api end point.
type HighAvailabilityAPI struct {
	state      *state.State
	resources  facade.Resources
	authorizer facade.Authorizer
}

var _ HighAvailability = (*HighAvailabilityAPI)(nil)

// NewHighAvailabilityAPI creates a new server-side highavailability API end point.
func NewHighAvailabilityAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*HighAvailabilityAPI, error) {
	// Only clients can access the high availability facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeCAAS {
		return nil, errors.NotSupportedf("high availability on kubernetes controllers")
	}

	return &HighAvailabilityAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// EnableHA adds controller machines as necessary to ensure the
// controller has the number of machines specified.
func (api *HighAvailabilityAPI) EnableHA(args params.ControllersSpecs) (params.ControllersChangeResults, error) {
	results := params.ControllersChangeResults{}

	admin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	}
	if !admin {
		return results, common.ServerError(common.ErrPerm)
	}

	if len(args.Specs) == 0 {
		return results, nil
	}
	if len(args.Specs) > 1 {
		return results, errors.New("only one controller spec is supported")
	}

	result, err := api.enableHASingle(api.state, args.Specs[0])
	results.Results = make([]params.ControllersChangeResult, 1)
	results.Results[0].Result = result
	results.Results[0].Error = common.ServerError(err)
	return results, nil
}

func (api *HighAvailabilityAPI) enableHASingle(st *state.State, spec params.ControllersSpec) (
	params.ControllersChanges, error,
) {
	if !st.IsController() {
		return params.ControllersChanges{}, errors.New("unsupported with hosted models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	controllerIds, err := st.ControllerIds()
	if err != nil {
		return params.ControllersChanges{}, err
	}

	// If there were no supplied constraints, use the original bootstrap
	// constraints.
	if constraints.IsEmpty(&spec.Constraints) || spec.Series == "" {
		referenceMachine, err := getReferenceController(st, controllerIds)
		if err != nil {
			return params.ControllersChanges{}, errors.Trace(err)
		}
		if constraints.IsEmpty(&spec.Constraints) {
			cons, err := referenceMachine.Constraints()
			if err != nil {
				return params.ControllersChanges{}, errors.Trace(err)
			}
			spec.Constraints = cons
		}
		if spec.Series == "" {
			spec.Series = referenceMachine.Series()
		}
	}

	// Retrieve the controller configuration and merge any implied space
	// constraints into the spec constraints.
	cfg, err := st.ControllerConfig()
	if err != nil {
		return params.ControllersChanges{}, errors.Annotate(err, "retrieving controller config")
	}
	if err = validateCurrentControllers(st, cfg, controllerIds); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}
	spec.Constraints.Spaces = cfg.AsSpaceConstraints(spec.Constraints.Spaces)

	if err = validatePlacementForSpaces(st, spec.Constraints.Spaces, spec.Placement); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	// Might be nicer to pass the spec itself to this method.
	changes, err := st.EnableHA(spec.NumControllers, spec.Constraints, spec.Series, spec.Placement)
	if err != nil {
		return params.ControllersChanges{}, err
	}
	return controllersChanges(changes), nil
}

// getReferenceController looks up the ideal controller to use as a reference for Constraints and Series
func getReferenceController(st *state.State, controllerIds []string) (*state.Machine, error) {
	// Sort the controller IDs from low to high and take the first.
	// This will typically give the initial bootstrap machine.
	var controllerNumbers []int
	for _, id := range controllerIds {
		idNum, err := strconv.Atoi(id)
		if err != nil {
			logger.Warningf("ignoring non numeric controller id %v", id)
			continue
		}
		controllerNumbers = append(controllerNumbers, idNum)
	}
	if len(controllerNumbers) == 0 {
		return nil, errors.Errorf("internal error; failed to find any controllers")
	}
	sort.Ints(controllerNumbers)
	controllerId := controllerNumbers[0]

	// Load the controller machine and get its constraints.
	cm, err := st.Machine(strconv.Itoa(controllerId))
	if err != nil {
		return nil, errors.Annotatef(err, "reading controller id %v", controllerId)
	}
	return cm, nil
}

// validateCurrentControllers checks for a scenario where there is no HA space
// in controller configuration and more than one machine-local address on any
// of the controller machines. An error is returned if it is detected.
// When HA space is set, there are other code paths that ensure controllers
// have at least one address in the space.
func validateCurrentControllers(st *state.State, cfg controller.Config, machineIds []string) error {
	if cfg.JujuHASpace() != "" {
		return nil
	}

	var badIds []string
	for _, id := range machineIds {
		cm, err := st.Machine(id)
		if err != nil {
			return errors.Annotatef(err, "reading controller id %v", id)
		}
		addresses := cm.Addresses()
		if len(addresses) == 0 {
			// machines without any address are essentially not started yet
			continue
		}
		internal := addresses.AllMatchingScope(network.ScopeMatchCloudLocal)
		if len(internal) != 1 {
			badIds = append(badIds, id)
		}
	}
	if len(badIds) > 0 {
		return errors.Errorf(
			"juju-ha-space is not set and a unique usable address was not found for machines: %s"+
				"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication",
			strings.Join(badIds, ", "),
		)
	}
	return nil
}

// validatePlacementForSpaces checks whether there are both space constraints
// and machine placement directives.
// If there are, checks are made to ensure that the machines specified have at
// least one address in all of the spaces.
func validatePlacementForSpaces(st *state.State, spaceNames *[]string, placement []string) error {
	if spaceNames == nil || len(*spaceNames) == 0 || len(placement) == 0 {
		return nil
	}

	for _, v := range placement {
		p, err := instance.ParsePlacement(v)
		if err != nil {
			if err == instance.ErrPlacementScopeMissing {
				// Where an unscoped placement is not parsed as a machine ID,
				// such as for a MaaS node name, just allow it through.
				// TODO (manadart 2018-03-27): Possible work at the provider
				// level to accommodate placement and space constraints during
				// instance pre-check may be entertained in the future.
				continue
			}
			return errors.Annotate(err, "parsing placement")
		}
		if p.Directive == "" {
			continue
		}

		m, err := st.Machine(p.Directive)
		if err != nil {
			if errors.IsNotFound(err) {
				// Don't throw out of here when the machine does not exist.
				// Validate others if required and leave it handled downstream.
				continue
			}
			return errors.Annotate(err, "retrieving machine")
		}

		spaceInfos, err := st.AllSpaceInfos()
		if err != nil {
			return errors.Trace(err)
		}

		for _, name := range *spaceNames {
			spaceInfo := spaceInfos.GetByName(name)
			if spaceInfo == nil {
				return errors.NotFoundf("space with name %q", name)
			}

			inSpace := false
			for _, addr := range m.Addresses() {
				if addr.SpaceID == spaceInfo.ID {
					inSpace = true
					break
				}
			}
			if !inSpace {
				return fmt.Errorf("machine %q has no addresses in space %q", p.Directive, name)
			}
		}
	}
	return nil
}

// controllersChanges generates a new params instance from the state instance.
func controllersChanges(change state.ControllersChanges) params.ControllersChanges {
	return params.ControllersChanges{
		Added:      machineIdsToTags(change.Added...),
		Maintained: machineIdsToTags(change.Maintained...),
		Removed:    machineIdsToTags(change.Removed...),
		Converted:  machineIdsToTags(change.Converted...),
	}
}

// machineIdsToTags returns a slice of machine tag strings created from the
// input machine IDs.
func machineIdsToTags(ids ...string) []string {
	var result []string
	for _, id := range ids {
		result = append(result, names.NewMachineTag(id).String())
	}
	return result
}
