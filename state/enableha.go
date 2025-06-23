// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/mongo"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/tools"
)

var errControllerNotAllowed = errors.New("controller jobs specified but not allowed")

func (st *State) getVotingControllerCount() (int, error) {
	controllerNodesColl, closer := st.db().GetCollection(controllerNodesC)
	defer closer()

	return controllerNodesColl.Find(bson.M{"wants-vote": true}).Count()
}

// maintainControllersOps returns a set of operations that will maintain
// the controller information when controllers with the given ids
// are added. If bootstrapOnly is true, there can be only one id = 0;
// (this is a special case to allow adding the bootstrap node).
func (st *State) maintainControllersOps(newIds []string, bootstrapOnly bool) ([]txn.Op, error) {
	if len(newIds) == 0 {
		return nil, nil
	}
	currentControllerIds, err := st.ControllerIds()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get controller info")
	}
	if bootstrapOnly {
		// Allow bootstrap machine only.
		if len(newIds) != 1 || newIds[0] != "0" {
			return nil, errControllerNotAllowed
		}
		if len(currentControllerIds) > 0 {
			return nil, errors.New("controllers already exist")
		}
	}
	ops := []txn.Op{{
		C:  controllersC,
		Id: modelGlobalKey,
		Assert: bson.D{
			{"controller-ids", bson.D{{"$size", len(currentControllerIds)}}},
		},
		Update: bson.D{
			{"$addToSet",
				bson.D{
					{"controller-ids", bson.D{{"$each", newIds}}},
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
	numControllers int, cons constraints.Value, base Base, placement []string,
) (ControllersChanges, []string, error) {

	if numControllers < 0 || (numControllers != 0 && numControllers%2 != 1) {
		return ControllersChanges{}, nil, errors.New("number of controllers must be odd and non-negative")
	}
	if numControllers > controller.MaxPeers {
		return ControllersChanges{}, nil, errors.Errorf("controller count is too large (allowed %d)", controller.MaxPeers)
	}

	// TODO(wallyworld) - only need until we transition away from enable-ha
	controllerApp, err := st.Application(bootstrap.ControllerApplicationName)
	if err != nil && !errors.IsNotFound(err) {
		return ControllersChanges{}, nil, errors.Annotate(err, "getting controller application")
	}

	enableHAOp := &enableHAOperation{
		controllerApp:  controllerApp,
		st:             st,
		numControllers: numControllers,
		cons:           cons,
		base:           base,
		placement:      placement,
	}

	if err := st.ApplyOperation(enableHAOp); err != nil {
		err = errors.Annotatef(err, "failed to enable HA with %d controllers", numControllers)
		return ControllersChanges{}, nil, err
	}
	return enableHAOp.change, enableHAOp.addedUnits, nil
}

// ControllersChanges records change in controllers after the ensure availability txn has committed.
type ControllersChanges struct {
	Added      []string
	Removed    []string
	Maintained []string
	Converted  []string
}

type enableHAOperation struct {
	// controllerApp may be nil if the controller application is not present.
	controllerApp  *Application
	st             *State
	numControllers int
	cons           constraints.Value
	base           Base
	placement      []string

	change     ControllersChanges
	addedUnits []string
}

func (e *enableHAOperation) Build(attempt int) ([]txn.Op, error) {
	desiredControllerCount := e.numControllers
	votingCount, err := e.st.getVotingControllerCount()
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
		return nil, errors.New("cannot remove controllers with enable-ha, use remove-machine and chose the controller(s) to remove")
	}

	controllerIds, err := e.st.ControllerIds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	intent, err := e.st.enableHAIntentions(controllerIds, e.placement)
	if err != nil {
		return nil, err
	}
	voteCount := len(intent.maintain)
	if voteCount == desiredControllerCount {
		return nil, jujutxn.ErrNoOperations
	}

	if n := desiredControllerCount - voteCount; n < len(intent.convert) {
		intent.convert = intent.convert[:n]
	}
	voteCount += len(intent.convert)

	intent.newCount = desiredControllerCount - voteCount

	logger.Infof(context.TODO(), "%d new machines; converting %v", intent.newCount, intent.convert)

	var ops []txn.Op
	ops, e.change, e.addedUnits, err = e.st.enableHAIntentionOps(attempt, e.controllerApp, intent, e.cons, e.base)
	return ops, err
}

func (e *enableHAOperation) Done(err error) error {
	return err
}

// enableHAIntentionOps returns operations to fulfil the desired intent.
func (st *State) enableHAIntentionOps(
	attempt int,
	controllerApp *Application,
	intent *enableHAIntent,
	cons constraints.Value,
	base Base,
) ([]txn.Op, ControllersChanges, []string, error) {
	var (
		ops        []txn.Op
		change     ControllersChanges
		addedUnits []string
	)

	for _, m := range intent.convert {
		ops = append(ops, convertControllerOps(m)...)
		change.Converted = append(change.Converted, m.Id())
		// Add a controller charm unit to the promoted machine.
		if controllerApp != nil {
			unitName, unitOps, err := st.addControllerUnitOps(attempt, controllerApp, AddUnitParams{machineID: m.Id()})
			if err != nil {
				return nil, ControllersChanges{}, nil, errors.Annotate(err, "composing controller unit operations")
			}
			ops = append(ops, unitOps...)
			addedUnits = append(addedUnits, unitName)
			addToMachineOp := txn.Op{
				C:      machinesC,
				Id:     m.doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$addToSet", bson.D{{"principals", unitName}}}, {"$set", bson.D{{"clean", false}}}},
			}
			ops = append(ops, addToMachineOp)
		}
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

	var controllerIds []string
	for i := 0; i < intent.newCount; i++ {
		placement, cons := getPlacementConstraints()
		template := MachineTemplate{
			Base:        base,
			Constraints: cons,
			Placement:   placement,
		}
		// Set up the new controller to have a controller charm unit.
		// The unit itself is created below.
		var controllerUnitName string
		if controllerApp != nil {
			var err error
			controllerUnitName, err = controllerApp.newUnitName()
			if err != nil {
				return nil, ControllersChanges{}, nil, errors.Trace(err)
			}
			template.Dirty = true
			template.principals = []string{controllerUnitName}
		}
		mdoc, addOps, err := st.addMachineOps(template)
		if err != nil {
			return nil, ControllersChanges{}, nil, errors.Trace(err)
		}
		controllerIds = append(controllerIds, mdoc.Id)
		ops = append(ops, addOps...)
		change.Added = append(change.Added, mdoc.Id)
		if controllerApp != nil {
			_, unitOps, err := st.addControllerUnitOps(attempt, controllerApp, AddUnitParams{
				UnitName:  &controllerUnitName,
				machineID: mdoc.Id,
			})
			if err != nil {
				return nil, ControllersChanges{}, nil, errors.Trace(err)
			}
			addedUnits = append(addedUnits, controllerUnitName)
			ops = append(ops, unitOps...)
		}
	}

	for _, m := range intent.maintain {
		change.Maintained = append(change.Maintained, m.Id())
	}
	ssOps, err := st.maintainControllersOps(controllerIds, false)
	if err != nil {
		return nil, ControllersChanges{}, nil, errors.Annotate(err, "cannot prepare machine add operations")
	}
	ops = append(ops, ssOps...)
	return ops, change, addedUnits, nil
}

func (st *State) addControllerUnitOps(attempt int, controllerApp *Application, p AddUnitParams) (string, []txn.Op, error) {
	unitName, unitOps, err := controllerApp.addUnitOps("", p, nil)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	// TODO(JUJU-7912) - Implement below, open the controller SSH unit port
	var portOps []txn.Op
	//config, err := st.ControllerConfig()
	//if err != nil {
	//	return "", nil, errors.Trace(err)
	//}
	//machinePorts, err := getOpenedMachinePortRanges(st, p.machineID)
	//if err != nil {
	//	return "", nil, errors.Annotatef(err, "cannot retrieve ports for unit %q", unitName)
	//}
	//// The controller unit doesn't exist yet.
	//machinePorts.unitExists = false
	//
	//pcp := machinePorts.ForUnit(unitName)
	//pcp.Open("", network.PortRange{
	//	FromPort: config.SSHServerPort(),
	//	ToPort:   config.SSHServerPort(),
	//	Protocol: "tcp",
	//})
	//portOps, err := pcp.Changes().Build(attempt)
	//if err != nil && !errors.Is(err, jujutxn.ErrNoOperations) {
	//	return "", nil, errors.Trace(err)
	//}

	return unitName, append(unitOps, portOps...), nil
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
//
//	gathering available, non-voting machines that may be promoted;
func (st *State) enableHAIntentions(controllerIds []string, placement []string) (*enableHAIntent, error) {
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
			intent.convert = append(intent.convert, m)
			continue
		}
		return nil, errors.Errorf("unsupported placement directive %q", s)
	}

	for _, id := range controllerIds {
		node, err := st.ControllerNode(id)
		if err != nil {
			return nil, err
		}
		intent.maintain = append(intent.maintain, node)
	}
	logger.Infof(context.TODO(), "initial intentions: maintain %v; convert: %v",
		intent.maintain, intent.convert)
	return &intent, nil
}

func convertControllerOps(m *Machine) []txn.Op {
	return []txn.Op{{
		C:  controllersC,
		Id: modelGlobalKey,
		Update: bson.D{
			{"$addToSet", bson.D{
				{"controller-ids", m.doc.Id},
			}},
		},
	},
		addControllerNodeOp(m.st, m.doc.Id),
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

func (st *State) removeControllerReferenceOps(cid string, controllerIds []string) []txn.Op {
	return []txn.Op{{
		C:      controllersC,
		Id:     modelGlobalKey,
		Assert: bson.D{{"controller-ids", controllerIds}},
		Update: bson.D{{"$pull", bson.D{{"controller-ids", cid}}}},
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
	Tag() names.Tag
	Refresh() error
	Watch() NotifyWatcher
	SetMongoPassword(password string) error
}

// ControllerIds returns the ids of the controller nodes.
func (st *State) ControllerIds() ([]string, error) {
	controllerInfo, err := st.ControllerInfo()
	if err != nil {
		return nil, errors.Annotatef(err, "reading controller info")
	}
	return controllerInfo.ControllerIds, nil
}

// ControllerNode returns the controller node with the given id.
func (st *State) ControllerNode(id string) (ControllerNode, error) {
	cdoc, err := st.getControllerNodeDoc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &controllerNode{*cdoc, st}, nil
}

// AddControllerNode creates a new controller node.
func (st *State) AddControllerNode() (*controllerNode, error) {
	seq, err := sequence(st, "controller")
	if err != nil {
		return nil, err
	}
	id := strconv.Itoa(seq)
	doc := controllerNodeDoc{
		DocID:     st.docID(id),
		WantsVote: true,
	}

	currentInfo, err := st.ControllerInfo()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	ops := []txn.Op{addControllerNodeOp(st, id)}
	ssOps, err := st.maintainControllersOps([]string{id}, currentInfo == nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, ssOps...)
	if err := st.db().RunTransaction(ops); err != nil {
		return nil, errors.Annotate(err, "cannot add controller node")
	}
	return &controllerNode{doc: doc, st: st}, nil
}

// HAPrimaryMachine returns machine tag for a controller machine
// that has a mongo instance that is primary in replicaset.
func (st *State) HAPrimaryMachine() (names.MachineTag, error) {
	nodeID := -1
	// Current status of replicaset contains node state.
	// Here we determine node id of the primary node.
	replicaStatus, err := replicaset.CurrentStatus(st.MongoSession())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	for _, m := range replicaStatus.Members {
		if m.State == replicaset.PrimaryState {
			nodeID = m.Id
		}
	}
	if nodeID == -1 {
		return names.MachineTag{}, errors.NotFoundf("HA primary machine")
	}

	// Current members collection of replicaset contains additional
	// information for the nodes, including machine IDs.
	ms, err := replicaset.CurrentMembers(st.MongoSession())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	for _, m := range ms {
		if m.Id == nodeID {
			if machineID, ok := m.Tags["juju-machine-id"]; ok {
				return names.NewMachineTag(machineID), nil
			}
		}
	}
	return names.MachineTag{}, errors.NotFoundf("HA primary machine")
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
	DocID        string       `bson:"_id"`
	HasVote      bool         `bson:"has-vote"`
	WantsVote    bool         `bson:"wants-vote"`
	PasswordHash string       `bson:"password-hash"`
	AgentVersion *tools.Tools `bson:"agent-version,omitempty"`
}

// Id returns the controller id.
func (c *controllerNode) Id() string {
	return c.st.localID(c.doc.DocID)
}

// Life is always alive for controller nodes.
// This API is used when a controller agent attempts
// to connect to a controller, currently only for CAAS.
// IAAS models still connect to machines as controllers.
// TODO(controlleragent) - model life on controller nodes
func (c *controllerNode) Life() Life {
	return Alive
}

// IsManager is always true for controller nodes.
func (c *controllerNode) IsManager() bool {
	return true
}

// Tag returns the controller tag.
func (c *controllerNode) Tag() names.Tag {
	return names.NewControllerAgentTag(c.Id())
}

func (c *controllerNode) SetMongoPassword(password string) error {
	return mongo.SetAdminMongoPassword(c.st.session, c.Tag().String(), password)
}

// SetPassword implements Authenticator.
func (c *controllerNode) SetPassword(password string) error {
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := internalpassword.AgentPasswordHash(password)
	ops := []txn.Op{{
		C:      controllerNodesC,
		Id:     c.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"password-hash", passwordHash}}}},
	}}
	err := c.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of controller node %q: %v", c.Id(), err)
	}
	c.doc.PasswordHash = passwordHash
	return nil
}

// PasswordValid implements Authenticator.
func (c *controllerNode) PasswordValid(password string) bool {
	agentHash := internalpassword.AgentPasswordHash(password)
	return agentHash == c.doc.PasswordHash
}

func (c *controllerNode) AgentTools() (*tools.Tools, error) {
	if c.doc.AgentVersion == nil {
		return nil, errors.NotFoundf("agent binaries for controller %v", c)
	}
	agentVersion := *c.doc.AgentVersion
	return &agentVersion, nil
}

func (c *controllerNode) SetAgentVersion(v semversion.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "setting agent version for controller %s", c.Id())
	if err := checkVersionValidity(v); err != nil {
		return err
	}
	binaryVersion := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      controllerNodesC,
		Id:     c.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"agent-version", binaryVersion}}}},
	}}
	// A "raw" transaction is needed here because this function gets
	// called before database migrations have run so we don't
	// necessarily want the model UUID added to the id.
	if err := c.st.runRawTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	c.doc.AgentVersion = binaryVersion
	return nil
}

// Refresh reloads the controller state.
func (c *controllerNode) Refresh() error {
	id := c.st.localID(c.doc.DocID)
	cdoc, err := c.st.getControllerNodeDoc(id)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
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

func setControllerWantsVoteOp(st *State, id string, wantsVote bool) txn.Op {
	return txn.Op{
		C:      controllerNodesC,
		Id:     st.docID(id),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"wants-vote", wantsVote}}}},
	}
}

type controllerReference interface {
	Id() string
	Refresh() error
}

// RemoveControllerReference will unregister Controller from being part of the set of Controllers.
// It must not have or want to vote, and it must not be the last controller.
func (st *State) RemoveControllerReference(c controllerReference) error {
	logger.Infof(context.TODO(), "removing controller machine %q", c.Id())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			// Something changed, make sure we're still up to date
			if err := c.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		controllerIds, err := st.ControllerIds()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(controllerIds) <= 1 {
			return nil, errors.Errorf("controller %s cannot be removed as it is the last controller", c.Id())
		}
		return st.removeControllerReferenceOps(c.Id(), controllerIds), nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func addControllerNodeOp(mb modelBackend, id string) txn.Op {
	doc := &controllerNodeDoc{
		DocID:     mb.docID(id),
		HasVote:   true,
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
