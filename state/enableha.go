// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
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
	controllerNodesColl, closer := st.db().GetCollection(controllerNodesC)
	defer closer()

	return controllerNodesColl.Find(
		bson.M{
			"_id":        bson.M{"$in": info.MachineIds},
			"wants-vote": true,
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

		if n := desiredControllerCount - voteCount; n < len(intent.convert) {
			intent.convert = intent.convert[:n]
		}
		voteCount += len(intent.convert)

		intent.newCount = desiredControllerCount - voteCount

		logger.Infof("%d new machines; converting %v", intent.newCount, intent.convert)

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

	for _, m := range intent.convert {
		ops = append(ops, convertControllerOps(m)...)
		change.Converted = append(change.Converted, m.Id())
	}

	// Use any placement directives that have been provided when adding new
	// machines, until the directives have been all used up.
	// Ignore constraints for provided machines.
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
		change.Maintained = append(change.Maintained, m.Id())
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

	maintain []ControllerNode
	convert  []*Machine
}

// enableHAIntentions returns what we would like
// to do to maintain the availability of the existing servers
// mentioned in the given info, including:
//   gathering available, non-voting machines that may be promoted;
func (st *State) enableHAIntentions(info *ControllerInfo, placement []string) (*enableHAIntent, error) {
	var intent enableHAIntent
	for _, s := range placement {
		// TODO(natefinch): Unscoped placements can end up here, though they
		// should not. We should fix up the CLI to always add a scope,
		// then we can remove the need to deal with unscoped placements.

		// Append unscoped placements to the intentions.
		// These will be used if/when adding new controllers is required.
		// These placements will be interpreted as availability zones.
		p, err := instance.ParsePlacement(s)
		if err == instance.ErrPlacementScopeMissing {
			intent.placement = append(intent.placement, s)
			continue
		}

		// Placements for machines are "consumed" by appending such machines as
		// candidates for promotion to controllers.
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
			continue
		}
		return nil, errors.Errorf("unsupported placement directive %q", s)
	}

	for _, id := range info.MachineIds {
		node, err := st.ControllerNode(id)
		if err != nil {
			return nil, err
		}
		logger.Infof("controller %q, wants vote %v, has vote %v", id, node.WantsVote(), node.HasVote())
		if node.WantsVote() {
			intent.maintain = append(intent.maintain, node)
		}
	}
	logger.Infof("initial intentions: maintain %v; convert: %v",
		intent.maintain, intent.convert)
	return &intent, nil
}

func convertControllerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:  machinesC,
		Id: m.doc.DocID,
		Update: bson.D{
			{"$addToSet", bson.D{{"jobs", JobManageModel}}},
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
	},
		addControllerNodeOp(m.st, m.doc.Id, false),
	}
}

func (st *State) getControllerNodeDoc(id string) (*controllerNodeDoc, error) {
	controllerNodesColl, closer := st.db().GetCollection(controllerNodesC)
	defer closer()

	cdoc := &controllerNodeDoc{}
	docId := st.docID(id)
	err := controllerNodesColl.FindId(docId).One(cdoc)

	switch err {
	case nil:
		return cdoc, nil
	case mgo.ErrNotFound:
		return nil, errors.NotFoundf("controller node %s", id)
	default:
		return nil, errors.Annotatef(err, "cannot get controller node %s", id)
	}
}

func (st *State) removeControllerReferenceOps(cid string, controllerInfo *ControllerInfo) []txn.Op {
	return []txn.Op{{
		C:  machinesC,
		Id: st.docID(cid),
		Update: bson.D{
			{"$pull", bson.D{{"jobs", JobManageModel}}},
		},
	}, {
		C:      controllersC,
		Id:     modelGlobalKey,
		Assert: bson.D{{"machineids", controllerInfo.MachineIds}},
		Update: bson.D{{"$pull", bson.D{{"machineids", cid}}}},
	}, {
		C:  controllerNodesC,
		Id: st.docID(cid),
		Assert: bson.D{
			{"wants-vote", false},
			{"has-vote", false},
		},
	}}
}

// ControllerNode represents an instance of a HA controller.
type ControllerNode interface {
	Id() string
	Refresh() error
	WantsVote() bool
	HasVote() bool
	SetHasVote(hasVote bool) error
	Watch() NotifyWatcher
}

// ControllerNode returns the controller node with the given id.
func (st *State) ControllerNode(id string) (ControllerNode, error) {
	cdoc, err := st.getControllerNodeDoc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &controllerNode{*cdoc, st}, nil
}

// ControllerNodes returns all the controller nodes.
func (st *State) ControllerNodes() ([]*controllerNode, error) {
	controllerNodesColl, closer := st.db().GetCollection(controllerNodesC)
	defer closer()

	var docs []controllerNodeDoc
	err := controllerNodesColl.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*controllerNode, len(docs))
	for i, doc := range docs {
		result[i] = &controllerNode{doc, st}
	}
	return result, nil
}

type controllerNode struct {
	doc controllerNodeDoc
	st  *State
}

type controllerNodeDoc struct {
	DocID     string `bson:"_id"`
	HasVote   bool   `bson:"has-vote"`
	WantsVote bool   `bson:"wants-vote"`
}

// Id returns the controller id.
func (c *controllerNode) Id() string {
	return c.st.localID(c.doc.DocID)
}

// Refresh reloads the controller state..
func (c *controllerNode) Refresh() error {
	id := c.st.localID(c.doc.DocID)
	cdoc, err := c.st.getControllerNodeDoc(id)
	if err != nil {
		if errors.IsNotFound(err) {
			return err
		}
		return errors.Annotatef(err, "cannot refresh controller node %v", c)
	}
	c.doc = *cdoc
	return nil
}

// Watch returns a watcher for observing changes to a node.
func (c *controllerNode) Watch() NotifyWatcher {
	return newEntityWatcher(c.st, controllerNodesC, c.doc.DocID)
}

// WantsVote reports whether the controller
// that wants to take part in peer voting.
func (c *controllerNode) WantsVote() bool {
	return c.doc.WantsVote
}

// HasVote reports whether that controller is currently a voting
// member of the replica set.
func (c *controllerNode) HasVote() bool {
	return c.doc.HasVote
}

// SetHasVote sets whether the controller is currently a voting
// member of the replica set. It should only be called
// from the worker that maintains the replica set.
func (c *controllerNode) SetHasVote(hasVote bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := c.Refresh(); err != nil {
				return nil, err
			}
		}

		var ops []txn.Op
		// Check the host entity life (machine on IAAS models).
		host, err := c.st.Machine(c.Id())
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			if host.Life() == Dead {
				return nil, ErrDead
			}
			ops = []txn.Op{{
				C:      machinesC,
				Id:     host.doc.DocID,
				Assert: notDeadDoc,
			}}
		}
		ops = append(ops, c.setHasVoteOps(hasVote)...)
		return ops, nil
	}
	if err := c.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *controllerNode) setHasVoteOps(hasVote bool) []txn.Op {
	return []txn.Op{{
		C:      controllerNodesC,
		Id:     c.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"has-vote", hasVote}}}},
	}}
}

func setControllerWantsVoteOp(st *State, id string, wantsVote bool) txn.Op {
	return txn.Op{
		C:      controllerNodesC,
		Id:     st.docID(id),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"wants-vote", wantsVote}}}},
	}
}

// RemoveControllerReference will unregister Controller from being part of the set of Controllers.
// It must not have or want to vote, and it must not be the last controller.
func (st *State) RemoveControllerReference(c ControllerNode) error {
	logger.Infof("removing controller machine %q", c.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			// Something changed, make sure we're still up to date
			if err := c.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if c.WantsVote() {
			return nil, errors.Errorf("controller %s cannot be removed as it still wants to vote", c.Id())
		}
		if c.HasVote() {
			return nil, errors.Errorf("controller %s cannot be removed as it still has a vote", c.Id())
		}
		controllerInfo, err := st.ControllerInfo()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(controllerInfo.MachineIds) <= 1 {
			return nil, errors.Errorf("controller %s cannot be removed as it is the last controller", c.Id())
		}
		return st.removeControllerReferenceOps(c.Id(), controllerInfo), nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func addControllerNodeOp(mb modelBackend, id string, hasVote bool) txn.Op {
	doc := &controllerNodeDoc{
		DocID:     mb.docID(id),
		HasVote:   hasVote,
		WantsVote: true,
	}
	return txn.Op{
		C:      controllerNodesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func removeControllerNodeOp(mb modelBackend, id string) txn.Op {
	return txn.Op{
		C:      controllerNodesC,
		Id:     mb.docID(id),
		Remove: true,
	}
}
