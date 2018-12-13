// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

func hasJob(jobs []MachineJob, job MachineJob) bool {
	for _, j := range jobs {
		if j == job {
			return true
		}
	}
	return false
}

var errControllerNotAllowed = errors.New("controller jobs specified but not allowed")

func (st *State) getVotingMachineCount(info *ControllerInfo) (int, error) {
	machinesCollection, closer := st.db().GetCollection(machinesC)
	defer closer()

	hasJobManageModel := bson.M{"$in": []MachineJob{JobManageModel}}
	return machinesCollection.Find(
		bson.M{
			"_id":    bson.M{"$in": info.MachineIds},
			"jobs":   hasJobManageModel,
			"novote": false,
		},
	).Count()
}

// maintainControllersOps returns a set of operations that will maintain
// the controller information when the given machine documents
// are added to the machines collection. If currentInfo is nil,
// there can be only one machine document and it must have
// id 0 (this is a special case to allow adding the bootstrap machine)
func (st *State) maintainControllersOps(mdocs []*machineDoc, currentInfo *ControllerInfo) ([]txn.Op, error) {
	var newIds []string
	for _, doc := range mdocs {
		if !hasJob(doc.Jobs, JobManageModel) {
			continue
		}
		newIds = append(newIds, doc.Id)
	}
	if len(newIds) == 0 {
		return nil, nil
	}
	if currentInfo == nil {
		// Allow bootstrap machine only.
		if len(mdocs) != 1 || mdocs[0].Id != "0" {
			return nil, errControllerNotAllowed
		}
		var err error
		currentInfo, err = st.ControllerInfo()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get controller info")
		}
		if len(currentInfo.MachineIds) > 0 {
			return nil, errors.New("controllers already exist")
		}
	}
	ops := []txn.Op{{
		C:  controllersC,
		Id: modelGlobalKey,
		Assert: bson.D{
			{"machineids", bson.D{{"$size", len(currentInfo.MachineIds)}}},
		},
		Update: bson.D{
			{"$addToSet",
				bson.D{
					{"machineids", bson.D{{"$each", newIds}}},
				},
			},
		},
	}}
	return ops, nil
}

// EnableHA adds controller machines as necessary to make
// the number of live controllers equal to numControllers. The given
// constraints and series will be attached to any new machines.
// If placement is not empty, any new machines which may be required are started
// according to the specified placement directives until the placement list is
// exhausted; thereafter any new machines are started according to the constraints and series.
// MachineID is the id of the machine where the apiserver is running.
func (st *State) EnableHA(
	numControllers int, cons constraints.Value, series string, placement []string,
) (ControllersChanges, error) {

	if numControllers < 0 || (numControllers != 0 && numControllers%2 != 1) {
		return ControllersChanges{}, errors.New("number of controllers must be odd and non-negative")
	}
	if numControllers > replicaset.MaxPeers {
		return ControllersChanges{}, errors.Errorf("controller count is too large (allowed %d)", replicaset.MaxPeers)
	}
	var change ControllersChanges
	buildTxn := func(attempt int) ([]txn.Op, error) {
		currentInfo, err := st.ControllerInfo()
		if err != nil {
			return nil, errors.Trace(err)
		}
		desiredControllerCount := numControllers
		votingCount, err := st.getVotingMachineCount(currentInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if desiredControllerCount == 0 {
			// Make sure we go to add odd number of desired voters. Even if HA was currently at 2 desired voters
			desiredControllerCount = votingCount + (votingCount+1)%2
			if desiredControllerCount <= 1 {
				desiredControllerCount = 3
			}
		}
		if votingCount > desiredControllerCount {
			return nil, errors.New("cannot reduce controller count")
		}

		intent, err := st.enableHAIntentions(currentInfo, placement)
		if err != nil {
			return nil, err
		}
		voteCount := 0
		for _, m := range intent.maintain {
			if m.WantsVote() {
				voteCount++
			}
		}
		if voteCount == desiredControllerCount {
			return nil, jujutxn.ErrNoOperations
		}
		// Promote as many machines as we can to fulfil the shortfall.
		if n := desiredControllerCount - voteCount; n < len(intent.promote) {
			intent.promote = intent.promote[:n]
		}
		voteCount += len(intent.promote)

		if n := desiredControllerCount - voteCount; n < len(intent.convert) {
			intent.convert = intent.convert[:n]
		}
		voteCount += len(intent.convert)

		intent.newCount = desiredControllerCount - voteCount

		logger.Infof("%d new machines; promoting %v; converting %v", intent.newCount, intent.promote, intent.convert)

		var ops []txn.Op
		ops, change, err = st.enableHAIntentionOps(intent, currentInfo, cons, series)
		return ops, err
	}
	if err := st.db().Run(buildTxn); err != nil {
		err = errors.Annotate(err, "failed to create new controller machines")
		return ControllersChanges{}, err
	}
	return change, nil
}

// Change in controllers after the ensure availability txn has committed.
type ControllersChanges struct {
	Added      []string
	Removed    []string
	Maintained []string
	Promoted   []string
	Demoted    []string
	Converted  []string
}

// enableHAIntentionOps returns operations to fulfil the desired intent.
func (st *State) enableHAIntentionOps(
	intent *enableHAIntent,
	currentInfo *ControllerInfo,
	cons constraints.Value,
	series string,
) ([]txn.Op, ControllersChanges, error) {
	var ops []txn.Op
	var change ControllersChanges
	for _, m := range intent.promote {
		ops = append(ops, promoteControllerOps(m)...)
		change.Promoted = append(change.Promoted, m.doc.Id)
	}
	for _, m := range intent.convert {
		ops = append(ops, convertControllerOps(m)...)
		change.Converted = append(change.Converted, m.doc.Id)
	}
	// Use any placement directives that have been provided
	// when adding new machines, until the directives have
	// been all used up. Ignore constraints for provided machines.
	// Set up a helper function to do the work required.
	placementCount := 0
	getPlacementConstraints := func() (string, constraints.Value) {
		if placementCount >= len(intent.placement) {
			return "", cons
		}
		result := intent.placement[placementCount]
		placementCount++
		return result, constraints.Value{}
	}
	mdocs := make([]*machineDoc, intent.newCount)
	for i := range mdocs {
		placement, cons := getPlacementConstraints()
		template := MachineTemplate{
			Series: series,
			Jobs: []MachineJob{
				JobHostUnits,
				JobManageModel,
			},
			Constraints: cons,
			Placement:   placement,
		}
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, ControllersChanges{}, err
		}
		mdocs[i] = mdoc
		ops = append(ops, addOps...)
		change.Added = append(change.Added, mdoc.Id)

	}
	for _, m := range intent.maintain {
		tag, err := names.ParseTag(m.Tag().String())
		if err != nil {
			return nil, ControllersChanges{}, errors.Annotate(err, "could not parse machine tag")
		}
		if tag.Kind() != names.MachineTagKind {
			return nil, ControllersChanges{}, errors.Errorf("expected machine tag kind, got %s", tag.Kind())
		}
		change.Maintained = append(change.Maintained, tag.Id())
	}
	ssOps, err := st.maintainControllersOps(mdocs, currentInfo)
	if err != nil {
		return nil, ControllersChanges{}, errors.Annotate(err, "cannot prepare machine add operations")
	}
	ops = append(ops, ssOps...)
	return ops, change, nil
}

type enableHAIntent struct {
	newCount  int
	placement []string

	promote, maintain, convert []*Machine
}

// enableHAIntentions returns what we would like
// to do to maintain the availability of the existing servers
// mentioned in the given info, including:
//   gathering available, non-voting machines that may be promoted;
func (st *State) enableHAIntentions(info *ControllerInfo, placement []string) (*enableHAIntent, error) {
	var intent enableHAIntent
	for _, s := range placement {
		// TODO(natefinch): unscoped placements shouldn't ever get here (though
		// they do currently).  We should fix up the CLI to always add a scope
		// to placements and then we can remove the need to deal with unscoped
		// placements.
		p, err := instance.ParsePlacement(s)
		if err == instance.ErrPlacementScopeMissing {
			intent.placement = append(intent.placement, s)
			continue
		}
		if err == nil && p.Scope == instance.MachineScope {
			if names.IsContainerMachine(p.Directive) {
				return nil, errors.New("container placement directives not supported")
			}

			m, err := st.Machine(p.Directive)
			if err != nil {
				return nil, errors.Annotatef(err, "can't find machine for placement directive %q", s)
			}
			if m.IsManager() {
				return nil, errors.Errorf("machine for placement directive %q is already a controller", s)
			}
			intent.convert = append(intent.convert, m)
			intent.placement = append(intent.placement, s)
			continue
		}
		return nil, errors.Errorf("unsupported placement directive %q", s)
	}

	for _, mid := range info.MachineIds {
		m, err := st.Machine(mid)
		if err != nil {
			return nil, err
		}
		logger.Infof("machine %q, wants vote %v, has vote %v", m, m.WantsVote(), m.HasVote())
		if m.WantsVote() {
			intent.maintain = append(intent.maintain, m)
		} else {
			intent.promote = append(intent.promote, m)
		}
	}
	logger.Infof("initial intentions: promote %v; maintain %v; convert: %v",
		intent.promote, intent.maintain, intent.convert)
	return &intent, nil
}

func convertControllerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:  machinesC,
		Id: m.doc.DocID,
		Update: bson.D{
			{"$addToSet", bson.D{{"jobs", JobManageModel}}},
			{"$set", bson.D{{"novote", false}}},
		},
		Assert: bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}}},
	}, {
		C:  controllersC,
		Id: modelGlobalKey,
		Update: bson.D{
			{"$addToSet", bson.D{
				{"machineids", m.doc.Id},
			}},
		},
	}}
}

func promoteControllerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: bson.D{{"novote", true}},
		Update: bson.D{{"$set", bson.D{{"novote", false}}}},
	}}
}

func removeControllerOps(m *Machine, controllerInfo *ControllerInfo) []txn.Op {
	return []txn.Op{{
		C:  machinesC,
		Id: m.doc.DocID,
		Assert: bson.D{
			{"novote", true},
			{"hasvote", false},
		},
		Update: bson.D{
			{"$pull", bson.D{{"jobs", JobManageModel}}},
		},
	}, {
		C:      controllersC,
		Id:     modelGlobalKey,
		Assert: bson.D{{"machineids", controllerInfo.MachineIds}},
		Update: bson.D{{"$pull", bson.D{{"machineids", m.doc.Id}}}},
	}}
}

// RemoveControllerMachine will remove Machine from being part of the set of Controllers.
// It must not have or want to vote, and it must not be the last controller.
func (st *State) RemoveControllerMachine(m *Machine) error {
	logger.Infof("removing controller machine %q", m.doc.Id)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			// Something changed, make sure we're still up to date
			m.Refresh()
		}
		if m.WantsVote() {
			return nil, errors.Errorf("machine %s cannot be removed as a controller as it still wants to vote", m.Id())
		}
		if m.HasVote() {
			return nil, errors.Errorf("machine %s cannot be removed as a controller as it still has a vote", m.Id())
		}
		controllerInfo, err := st.ControllerInfo()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(controllerInfo.MachineIds) <= 1 {
			return nil, errors.Errorf("machine %s cannot be removed as it is the last controller", m.Id())
		}
		return removeControllerOps(m, controllerInfo), nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
