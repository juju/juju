// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/blockcommand"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// NodeService describes the maintenance of controller entries.
type NodeService interface {
	// CurateNodes modifies the control place by adding and
	// removing node entries based on the input slices.
	CurateNodes(context.Context, []string, []string) error
}

// MachineService instances save a machine to dqlite state.
type MachineService interface {
	CreateMachine(context.Context, machine.Name) (string, error)
}

// ApplicationService instances add units to an application in dqlite state.
type ApplicationService interface {
	AddUnits(
		ctx context.Context,
		storageParentDir, name string,
		units ...applicationservice.AddUnitArg,
	) error
}

// ControllerConfigService instances read the controller config.
type ControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// HighAvailabilityAPI implements the HighAvailability interface and is the concrete
// implementation of the api end point.
type HighAvailabilityAPI struct {
	st                      *state.State
	nodeService             NodeService
	machineService          MachineService
	applicationService      ApplicationService
	controllerConfigService ControllerConfigService
	networkService          NetworkService
	blockCommandService     BlockCommandService
	authorizer              facade.Authorizer
	logger                  corelogger.Logger
}

// HighAvailabilityAPIV2 implements v2 of the high availability facade.
type HighAvailabilityAPIV2 struct {
	HighAvailabilityAPI
}

// EnableHA adds controller machines as necessary to ensure the
// controller has the number of machines specified.
func (api *HighAvailabilityAPI) EnableHA(
	ctx context.Context, args params.ControllersSpecs,
) (params.ControllersChangeResults, error) {
	results := params.ControllersChangeResults{}

	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.st.ControllerTag())
	if err != nil {
		return results, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	if len(args.Specs) == 0 {
		return results, nil
	}
	if len(args.Specs) > 1 {
		return results, errors.New("only one controller spec is supported")
	}

	result, err := api.enableHASingle(ctx, args.Specs[0])
	results.Results = make([]params.ControllersChangeResult, 1)
	results.Results[0].Result = result
	results.Results[0].Error = apiservererrors.ServerError(err)
	return results, nil
}

func (api *HighAvailabilityAPI) enableHASingle(ctx context.Context, spec params.ControllersSpec) (
	params.ControllersChanges, error,
) {
	st := api.st

	if !st.IsController() {
		return params.ControllersChanges{}, errors.New("unsupported with workload models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(api.blockCommandService)
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	controllerIds, err := st.ControllerIds()
	if err != nil {
		return params.ControllersChanges{}, err
	}

	referenceMachine, err := getReferenceController(st, controllerIds, api.logger)
	if err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}
	// If there were no supplied constraints, use the original bootstrap
	// constraints.
	if constraints.IsEmpty(&spec.Constraints) {
		if constraints.IsEmpty(&spec.Constraints) {
			cons, err := referenceMachine.Constraints()
			if err != nil {
				return params.ControllersChanges{}, errors.Trace(err)
			}
			spec.Constraints = cons
		}
	}

	// Retrieve the controller configuration and merge any implied space
	// constraints into the spec constraints.
	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.ControllersChanges{}, errors.Annotate(err, "retrieving controller config")
	}
	if err = validateCurrentControllers(st, cfg, controllerIds); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}
	// Check if the object store is backed by filesystem storage.
	if cfg.ObjectStoreType() == objectstore.FileBackend {
		return params.ControllersChanges{}, errors.NewNotSupported(nil, "cannot enable-ha with filesystem backed object store")
	}

	spec.Constraints.Spaces = cfg.AsSpaceConstraints(spec.Constraints.Spaces)

	if err = validatePlacementForSpaces(ctx, st, api.networkService, spec.Constraints.Spaces, spec.Placement); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	// Might be nicer to pass the spec itself to this method.
	changes, addedUnits, err := st.EnableHA(spec.NumControllers, spec.Constraints, referenceMachine.Base(), spec.Placement)
	if err != nil {
		return params.ControllersChanges{}, err
	}

	// TODO (manadart 2023-06-12): This is the lightest touch to represent the
	// control plane in Dqlite. It expected to change significantly when
	// Mongo concerns are removed altogether.
	err = api.nodeService.CurateNodes(ctx, append(changes.Added, changes.Converted...), changes.Removed)
	if err != nil {
		return params.ControllersChanges{}, err
	}

	// Add the dqlite records for new machines.
	for _, m := range changes.Added {
		if _, err := api.machineService.CreateMachine(ctx, machine.Name(m)); err != nil {
			return params.ControllersChanges{}, err
		}
	}
	if len(addedUnits) > 0 {
		addUnitArgs := make([]applicationservice.AddUnitArg, len(addedUnits))
		for i := range addedUnits {
			// Try and get the placement for this unit. If it doesn't exist,
			// then the default behaviour is to create a new machine for the
			// unit.
			var placement *instance.Placement
			if i < len(spec.Placement) {
				var err error
				placement, err = instance.ParsePlacement(spec.Placement[i])
				if err != nil {
					return params.ControllersChanges{}, errors.Annotate(err, "parsing placement")
				}
			}

			unitName, err := coreunit.NewName(addedUnits[i])
			if err != nil {
				return params.ControllersChanges{}, internalerrors.Errorf("parsing unit name %q: %v", addedUnits[i], err)
			}
			addUnitArgs[i] = applicationservice.AddUnitArg{
				UnitName:  unitName,
				Placement: placement,
			}
		}
		if err := api.applicationService.AddUnits(
			ctx,
			domainapplication.StorageParentDir,
			application.ControllerApplicationName,
			addUnitArgs...,
		); err != nil {
			return params.ControllersChanges{}, err
		}
	}

	return controllersChanges(changes), nil
}

// getReferenceController looks up the ideal controller to use as a reference for Constraints and Release
func getReferenceController(st *state.State, controllerIds []string, logger corelogger.Logger) (*state.Machine, error) {
	// Sort the controller IDs from low to high and take the first.
	// This will typically give the initial bootstrap machine.
	var controllerNumbers []int
	for _, id := range controllerIds {
		idNum, err := strconv.Atoi(id)
		if err != nil {
			logger.Warningf(context.TODO(), "ignoring non numeric controller id %v", id)
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
		if len(addresses) > 1 {
			// ignore /32 and /128 addresses, which can be associated with a virtual IP (see https://bugs.launchpad.net/juju/+bug/2073986)
			addresses = slices.DeleteFunc(addresses, func(addr network.SpaceAddress) bool {
				if addr.AddressType() == network.IPv6Address {
					return strings.HasSuffix(addr.AddressCIDR(), "/128")
				}
				return strings.HasSuffix(addr.AddressCIDR(), "/32")
			})
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
func validatePlacementForSpaces(ctx context.Context, st *state.State, networkService NetworkService, spaceNames *[]string, placement []string) error {
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
			if errors.Is(err, errors.NotFound) {
				// Don't throw out of here when the machine does not exist.
				// Validate others if required and leave it handled downstream.
				continue
			}
			return errors.Annotate(err, "retrieving machine")
		}

		spaceInfos, err := networkService.GetAllSpaces(ctx)
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

// ControllerDetails is only available on V3 or later.
func (api *HighAvailabilityAPIV2) ControllerDetails(_ struct{}) {}

// ControllerDetails returns details about each controller node.
func (api *HighAvailabilityAPI) ControllerDetails(
	ctx context.Context,
) (params.ControllerDetailsResults, error) {
	results := params.ControllerDetailsResults{}

	err := api.authorizer.HasPermission(ctx, permission.LoginAccess, api.st.ControllerTag())
	if err != nil {
		return results, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	apiPort := cfg.APIPort()

	nodes, err := api.st.ControllerNodes()
	if err != nil {
		return results, apiservererrors.ServerError(err)
	}

	for _, n := range nodes {
		m, err := api.st.Machine(n.Id())
		if err != nil {
			return results, apiservererrors.ServerError(err)
		}
		addr, err := m.PublicAddress()
		if err != nil {
			// Usually this indicates that no addresses have been set on the
			// machine yet.
			addr = network.SpaceAddress{}
		}
		mAddrs := m.Addresses()
		if len(mAddrs) == 0 {
			// At least give it the newly created DNSName address, if it exists.
			if addr.Value != "" {
				mAddrs = append(mAddrs, addr)
			}
		}
		hp := make(network.HostPorts, len(mAddrs))
		for i, addr := range mAddrs {
			hp[i] = network.MachineHostPort{
				MachineAddress: addr.MachineAddress,
				NetPort:        network.NetPort(apiPort),
			}
		}
		hp = hp.FilterUnusable().Unique()
		results.Results = append(results.Results, params.ControllerDetails{
			ControllerId: m.Id(),
			APIAddresses: hp.Strings(),
		})
		// Sort for testing.
		sort.Slice(results.Results, func(i, j int) bool {
			return results.Results[i].ControllerId < results.Results[j].ControllerId
		})
	}
	return results, nil
}
