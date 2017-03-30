// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"sort"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/permission"
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
	// Only clients and model managers can access the high availability facade.
	if !authorizer.AuthClient() && !authorizer.AuthController() {
		return nil, common.ErrPerm
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

	if api.authorizer.AuthClient() {
		admin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
		if err != nil && !errors.IsNotFound(err) {
			return results, errors.Trace(err)
		}
		if !admin {
			return results, common.ServerError(common.ErrPerm)
		}
	}

	if len(args.Specs) == 0 {
		return results, nil
	}
	if len(args.Specs) > 1 {
		return results, errors.New("only one controller spec is supported")
	}

	result, err := enableHASingle(api.state, args.Specs[0])
	results.Results = make([]params.ControllersChangeResult, 1)
	results.Results[0].Result = result
	results.Results[0].Error = common.ServerError(err)
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

// Generate a ControllersChanges structure.
func controllersChanges(change state.ControllersChanges) params.ControllersChanges {
	return params.ControllersChanges{
		Added:      machineIdsToTags(change.Added...),
		Maintained: machineIdsToTags(change.Maintained...),
		Removed:    machineIdsToTags(change.Removed...),
		Promoted:   machineIdsToTags(change.Promoted...),
		Demoted:    machineIdsToTags(change.Demoted...),
		Converted:  machineIdsToTags(change.Converted...),
	}
}

func enableHASingle(st *state.State, spec params.ControllersSpec) (params.ControllersChanges, error) {
	if !st.IsController() {
		return params.ControllersChanges{}, errors.New("unsupported with hosted models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	series := spec.Series
	if series == "" {
		ssi, err := st.ControllerInfo()
		if err != nil {
			return params.ControllersChanges{}, err
		}

		// We should always have at least one voting machine
		// If we *really* wanted we could just pick whatever series is
		// in the majority, but really, if we always copy the value of
		// the first one, then they'll stay in sync.
		if len(ssi.VotingMachineIds) == 0 {
			// Better than a panic()?
			return params.ControllersChanges{}, errors.Errorf("internal error, failed to find any voting machines")
		}
		templateMachine, err := st.Machine(ssi.VotingMachineIds[0])
		if err != nil {
			return params.ControllersChanges{}, err
		}
		series = templateMachine.Series()
	}
	if constraints.IsEmpty(&spec.Constraints) {
		// No constraints specified, so we'll use the constraints off
		// a running controller.
		controllerInfo, err := st.ControllerInfo()
		if err != nil {
			return params.ControllersChanges{}, err
		}
		// We'll sort the controller ids to find the smallest.
		// This will typically give the initial bootstrap machine.
		var controllerIds []int
		for _, id := range controllerInfo.MachineIds {
			idNum, err := strconv.Atoi(id)
			if err != nil {
				logger.Warningf("ignoring non numeric controller id %v", id)
				continue
			}
			controllerIds = append(controllerIds, idNum)
		}
		if len(controllerIds) == 0 {
			errors.Errorf("internal error, failed to find any controllers")
		}
		sort.Ints(controllerIds)

		// Load the controller machine and get its constraints.
		controllerId := controllerIds[0]
		controller, err := st.Machine(strconv.Itoa(controllerId))
		if err != nil {
			return params.ControllersChanges{}, errors.Annotatef(err, "reading controller id %v", controllerId)
		}
		spec.Constraints, err = controller.Constraints()
		if err != nil {
			return params.ControllersChanges{}, errors.Annotatef(err, "reading constraints for controller id %v", controllerId)
		}
	}

	changes, err := st.EnableHA(spec.NumControllers, spec.Constraints, series, spec.Placement)
	if err != nil {
		return params.ControllersChanges{}, err
	}
	return controllersChanges(changes), nil
}

// StopHAReplicationForUpgrade will prompt the HA cluster to enter upgrade
// mongo mode.
func (api *HighAvailabilityAPI) StopHAReplicationForUpgrade(args params.UpgradeMongoParams) (params.MongoUpgradeResults, error) {
	ha, err := api.state.SetUpgradeMongoMode(mongo.Version{
		Major:         args.Target.Major,
		Minor:         args.Target.Minor,
		Patch:         args.Target.Patch,
		StorageEngine: mongo.StorageEngine(args.Target.StorageEngine),
	})
	if err != nil {
		return params.MongoUpgradeResults{}, errors.Annotate(err, "cannot stop HA for ugprade")
	}
	members := make([]params.HAMember, len(ha.Members))
	for i, m := range ha.Members {
		members[i] = params.HAMember{
			Tag:           m.Tag,
			PublicAddress: m.PublicAddress,
			Series:        m.Series,
		}
	}
	return params.MongoUpgradeResults{
		Master: params.HAMember{
			Tag:           ha.Master.Tag,
			PublicAddress: ha.Master.PublicAddress,
			Series:        ha.Master.Series,
		},
		Members:   members,
		RsMembers: ha.RsMembers,
	}, nil
}

// ResumeHAReplicationAfterUpgrade will add the upgraded members of HA
// cluster to the upgraded master.
func (api *HighAvailabilityAPI) ResumeHAReplicationAfterUpgrade(args params.ResumeReplicationParams) error {
	return api.state.ResumeReplication(args.Members)
}
