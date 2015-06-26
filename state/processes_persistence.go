// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
)

// TODO(ericsnow) Implement persistence using a TXN abstraction (used
// in the business logic) with ops factories available from the
// persistence layer.

type procsPersistenceBase interface {
	One(collName, id string, doc interface{}) error
	All(collName string, ids []string, docs interface{}) error
	Run(transactions jujutxn.TransactionSource) error
}

type statePersistence struct {
	st *State
}

func (sp statePersistence) One(collName, id string, doc interface{}) error {
	coll, closeColl := sp.st.getCollection(collName)
	defer closeColl()

	err := coll.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf(id)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (sp statePersistence) All(collName string, ids []string, docs interface{}) error {
	coll, closeColl := sp.st.getCollection(collName)
	defer closeColl()

	//q := bson.M{"_id": bson.M{"$in": ids}}
	q := bson.M{"$in": ids}
	if err := coll.FindId(q).All(docs); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (sp statePersistence) Run(transactions jujutxn.TransactionSource) error {
	if err := sp.st.run(transactions); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type procsPersistence struct {
	st    procsPersistenceBase
	charm names.CharmTag
	unit  names.UnitTag
}

func newProcsPersistence(st procsPersistenceBase, charm *names.CharmTag, unit *names.UnitTag) *procsPersistence {
	pp := &procsPersistence{
		st: st,
	}
	if charm != nil {
		pp.charm = *charm
	}
	if unit != nil {
		pp.unit = *unit
	}
	return pp
}

func (pp procsPersistence) one(id string, doc interface{}) error {
	return errors.Trace(pp.st.One(workloadProcessesC, id, doc))
}

func (pp procsPersistence) all(ids []string, docs interface{}) error {
	return errors.Trace(pp.st.All(workloadProcessesC, ids, docs))
}

func (pp procsPersistence) indexDefinitionDocs(ids []string) (map[interface{}]ProcessDefinitionDoc, error) {
	var docs []ProcessDefinitionDoc
	if err := pp.all(ids, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	indexed := make(map[interface{}]ProcessDefinitionDoc)
	for _, doc := range docs {
		indexed[doc.DocID] = doc
	}
	return indexed, nil
}

// EnsureDefinitions checks persistence to see if records for the
// definitions are already there. If not then they are added. If so then
// they are checked to be sure they match those provided. The lists of
// names for those that already exist and that don't match are returned.
func (pp procsPersistence) EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error) {
	var found []string
	var mismatched []string

	var ids []string
	var ops []txn.Op
	for _, definition := range definitions {
		ids = append(ids, pp.definitionID(definition.Name))
		ops = append(ops, pp.newInsertDefinitionOp(definition))
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// TODO(ericsnow) Track found and mismatched.

			// The last attempt aborted so clear out any ops that failed
			// the DocMissing assertion and try again.
			indexed, err := pp.indexDefinitionDocs(ids)
			if err != nil {
				return nil, errors.Trace(err)
			}

			var okOps []txn.Op
			for _, op := range ops {
				if _, ok := indexed[op.Id]; !ok {
					okOps = append(okOps, op)
				} else {
					// TODO(ericsnow) compare ops to corresponding
					// definitions; fail if not the same.
				}
				// Otherwise the op is dropped.
			}
			if len(okOps) == 0 {
				return nil, jujutxn.ErrNoOperations
			}
			ops = okOps
		}
		return ops, nil
	}
	if err := pp.st.Run(buildTxn); err != nil {
		return nil, nil, errors.Trace(err)
	}

	return found, mismatched, nil
}

// Insert adds records for the process to persistence. If the process
// is already there then false gets returned (true if inserted).
// Existing records are not checked for consistency.
func (pp procsPersistence) Insert(info process.Info) (bool, error) {
	var ops []txn.Op
	// TODO(ericsnow) Add unitPersistence.newEnsureAliveOp(pp.unit)?
	// TODO(ericsnow) Add pp.newEnsureDefinitionOp(info.Process)?
	ops = append(ops, pp.newInsertProcessOps(info)...)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// TODO(ericsnow) Return false if found.
		return ops, nil
	}
	if err := pp.st.Run(buildTxn); err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// SetStatus updates the raw status for the identified process in
// persistence. The return value corresponds to whether or not the
// record was found in persistence. Any other problem results in
// an error. The process is not checked for inconsistent records.
func (pp procsPersistence) SetStatus(id string, status process.Status) (bool, error) {
	var found bool
	var ops []txn.Op
	// TODO(ericsnow) Add unitPersistence.newEnsureAliveOp(pp.unit)?
	ops = append(ops, pp.newSetRawStatusOps(id, status)...)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			_, err := pp.proc(id)
			if err == mgo.ErrNotFound {
				found = false
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
			// We ignore the request since the proc is dying.
			return nil, jujutxn.ErrNoOperations
		}
		found = true
		return ops, nil
	}
	if err := pp.st.Run(buildTxn); err != nil {
		return false, errors.Trace(err)
	}
	return found, nil
}

// List builds the list of processes found in persistence which match
// the provided IDs. The lists of IDs with missing records is also
// returned. Inconsistent records result in errors.NotValid.
func (pp procsPersistence) List(ids ...string) ([]process.Info, []string, error) {
	var missing []string

	// TODO(ericsnow) Track missing records.
	// TODO(ericsnow) Fail for inconsistent records.
	// TODO(ericsnow) Ensure that the unit is Alive?
	// TODO(ericsnow) possible race here
	procDocs, err := pp.procs(ids)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	launchDocs, err := pp.launches(ids)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	definitionDocs, err := pp.definitions(ids)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// TODO(ericsnow) charm-defined proc definitions must be accommodated.
	results := make([]process.Info, len(ids))
	for i := range results {
		doc := processInfoDoc{
			definition: definitionDocs[i],
			launch:     launchDocs[i],
			proc:       procDocs[i],
		}
		info := doc.info()
		info.CharmID = pp.charm.Id()
		info.UnitID = pp.unit.Id()
		results[i] = doc.info()
	}
	return results, missing, nil
}

// TODO(ericsnow) Add procs to state/cleanup.go.

// TODO(ericsnow) How to ensure they are completely removed from state?

// Remove removes all records associated with the identified process
// from persistence. Also returned is whether or not the process was
// found. If the records for the process are not consistent then
// errors.NotValid is returned.
func (pp procsPersistence) Remove(id string) (bool, error) {
	var found bool
	var ops []txn.Op
	// TODO(ericsnow) Add unitPersistence.newEnsureAliveOp(pp.unit)?
	ops = append(ops, pp.newRemoveProcessOps(id)...)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// TODO(ericsnow) Fail if records not consistent.
		// TODO(ericsnow) Set found back to false if missing.
		found = true
		return ops, nil
	}
	if err := pp.st.Run(buildTxn); err != nil {
		return false, errors.Trace(err)
	}
	return found, nil
}

// TODO(ericsnow) Factor most of the below into a processesCollection type.

func (pp procsPersistence) definitionID(id string) string {
	name, _ := process.ParseID(id)
	// The URL will always parse successfully.
	charmURL, _ := charm.ParseURL(pp.charm.Id())
	return fmt.Sprintf("%s#%s", charmGlobalKey(charmURL), name)
}

func (pp procsPersistence) processID(id string) string {
	return fmt.Sprintf("%s#%s", unitGlobalKey(pp.unit.Id()), id)
}

func (pp procsPersistence) launchID(id string) string {
	return pp.processID(id) + "#launch"
}

func (pp procsPersistence) newInsertDefinitionOp(definition charm.Process) txn.Op {
	doc := pp.newProcessDefinitionDoc(definition)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp procsPersistence) newInsertProcessOps(info process.Info) []txn.Op {
	var ops []txn.Op
	ops = append(ops, pp.newInsertLaunchOp(info))
	ops = append(ops, pp.newInsertProcOp(info))
	return ops
}

func (pp procsPersistence) newInsertLaunchOp(info process.Info) txn.Op {
	doc := pp.newLaunchDoc(info)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp procsPersistence) newInsertProcOp(info process.Info) txn.Op {
	doc := pp.newProcessDoc(info)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp procsPersistence) newSetRawStatusOps(id string, status process.RawStatus) []txn.Op {
	id = pp.processID(id)
	return []txn.Op{{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
	}, {
		C:      workloadProcessesC,
		Id:     id,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"pluginstatus", status.Value}}}},
	}}
}

func (pp procsPersistence) newRemoveProcessOps(id string) []txn.Op {
	var ops []txn.Op
	ops = append(ops, pp.newRemoveLaunchOp(id))
	ops = append(ops, pp.newRemoveProcOp(id))
	return ops
}

func (pp procsPersistence) newRemoveLaunchOp(id string) txn.Op {
	id = pp.launchID(id)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}
}

func (pp procsPersistence) newRemoveProcOp(id string) txn.Op {
	id = pp.processID(id)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}
}

type processInfoDoc struct {
	definition ProcessDefinitionDoc
	launch     ProcessLaunchDoc
	proc       ProcessDoc
}

func (d processInfoDoc) info() process.Info {
	info := d.proc.info()

	info.Process = d.definition.definition()

	rawStatus := info.Details.Status
	info.Details = d.launch.details()
	info.Details.Status = rawStatus

	return info
}

type ProcessDefinitionDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	UnitID string `bson:"unitid"`

	Name        string            `bson:"name"`
	Description string            `bson:"description"`
	Type        string            `bson:"type"`
	TypeOptions map[string]string `bson:"typeoptions"`
	Command     string            `bson:"command"`
	Image       string            `bson:"image"`
	Ports       []string          `bson:"ports"`
	Volumes     []string          `bson:"volumes"`
	EnvVars     map[string]string `bson:"envvars"`
}

func (d ProcessDefinitionDoc) definition() charm.Process {
	ports := make([]charm.ProcessPort, len(d.Ports))
	for i, raw := range d.Ports {
		p := ports[i]
		fmt.Sscanf(raw, "%d:%d:%s", &p.External, &p.Internal, &p.Endpoint)
	}

	volumes := make([]charm.ProcessVolume, len(d.Volumes))
	for i, raw := range d.Volumes {
		v := volumes[i]
		fmt.Sscanf(raw, "%d:%d:%s", &v.ExternalMount, &v.InternalMount, &v.Mode, &v.Name)
	}

	return charm.Process{
		Name:        d.Name,
		Description: d.Description,
		Type:        d.Type,
		TypeOptions: d.TypeOptions,
		Command:     d.Command,
		Image:       d.Image,
		Ports:       ports,
		Volumes:     volumes,
		EnvVars:     d.EnvVars,
	}
}

func (pp procsPersistence) newProcessDefinitionDoc(definition charm.Process) *ProcessDefinitionDoc {
	id := pp.definitionID(definition.Name)

	var ports []string
	for _, p := range definition.Ports {
		// TODO(ericsnow) Ensure p.Endpoint is in state?
		ports = append(ports, fmt.Sprintf("%d:%d:%s", p.External, p.Internal, p.Endpoint))
	}

	var volumes []string
	for _, v := range definition.Volumes {
		// TODO(ericsnow) Ensure v.Name is in state?
		volumes = append(volumes, fmt.Sprintf("%s:%s:%s:%s", v.ExternalMount, v.InternalMount, v.Mode, v.Name))
	}

	return &ProcessDefinitionDoc{
		DocID:  id,
		UnitID: pp.unit.Id(),

		Name:        definition.Name,
		Description: definition.Description,
		Type:        definition.Type,
		TypeOptions: definition.TypeOptions,
		Command:     definition.Command,
		Image:       definition.Image,
		Ports:       ports,
		Volumes:     volumes,
		EnvVars:     definition.EnvVars,
	}
}

func (pp procsPersistence) definitions(ids []string) ([]ProcessDefinitionDoc, error) {
	for i, id := range ids {
		ids[i] = pp.definitionID(id)
	}

	var docs []ProcessDefinitionDoc
	if err := pp.all(ids, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

type ProcessLaunchDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	PluginID  string `bson:"pluginid"`
	RawStatus string `bson:"rawstatus"`
}

func (d ProcessLaunchDoc) details() process.Details {
	return process.Details{
		ID: d.PluginID,
		Status: process.RawStatus{
			Value: d.RawStatus,
		},
	}
}

func (pp procsPersistence) newLaunchDoc(info process.Info) *ProcessLaunchDoc {
	id := pp.launchID(info.ID())
	return &ProcessLaunchDoc{
		DocID: id,

		PluginID:  info.Details.ID,
		RawStatus: info.Details.Status.Value,
	}
}

func (pp procsPersistence) launches(ids []string) ([]ProcessLaunchDoc, error) {
	for i, id := range ids {
		ids[i] = pp.launchID(id)
	}

	var docs []ProcessLaunchDoc
	if err := pp.all(ids, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

type ProcessDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	Life         Life   `bson:"life"`
	Status       string `bson:"status"`
	PluginStatus string `bson:"pluginstatus"`
}

func (d ProcessDoc) info() process.Info {
	var status process.Status
	switch d.Status {
	case "pending":
		status = process.StatusPending
	case "active":
		status = process.StatusActive
	case "failed":
		status = process.StatusFailed
	case "stopped":
		status = process.StatusStopped
	}
	if d.Life != Alive {
		if status != process.StatusFailed && status != process.StatusStopped {
			// TODO(ericsnow) Is this the right place to do this?
			status = process.StatusStopped
		}
	}

	return process.Info{
		Status: status,
	}
}

func (pp procsPersistence) newProcessDoc(info process.Info) *ProcessDoc {
	id := pp.processID(info.ID())

	var status string
	switch info.Status {
	case process.StatusPending:
		status = "pending"
	case process.StatusActive:
		status = "active"
	case process.StatusFailed:
		status = "failed"
	case process.StatusStopped:
		status = "stopped"
	default:
		// TODO(ericsnow) disallow? don't worry (shouldn't happen)?
		status = "unknown"
	}

	return &ProcessDoc{
		DocID: id,

		Life:         Alive,
		Status:       status,
		PluginStatus: info.Details.Status.Value,
	}
}

func (pp procsPersistence) proc(id string) (*ProcessDoc, error) {
	id = pp.processID(id)

	var doc ProcessDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp procsPersistence) procs(ids []string) ([]ProcessDoc, error) {
	for i, id := range ids {
		ids[i] = pp.processID(id)
	}

	var docs []ProcessDoc
	if err := pp.all(ids, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}
