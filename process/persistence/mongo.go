// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Move this to a subpackage and split it up?

package persistence

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
)

const workloadProcessesC = "workloadprocesses"

func unitGlobalKey(name string) string {
	return "u#" + name + "#charm"
}

func charmGlobalKey(charmURL *charm.URL) string {
	return "c#" + charmURL.String()
}

//type Persistence struct {
//	st    PersistenceBase
//	charm names.CharmTag
//	unit  names.UnitTag
//}

func (pp Persistence) indexDefinitionDocs(ids []string) (map[interface{}]ProcessDefinitionDoc, error) {
	var docs []ProcessDefinitionDoc
	//query := bson.M{"_id": bson.M{"$in": ids}}
	query := bson.M{"$in": ids}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	indexed := make(map[interface{}]ProcessDefinitionDoc)
	for _, doc := range docs {
		indexed[doc.DocID] = doc
	}
	return indexed, nil
}

func (pp Persistence) extractProc(id string, definitionDocs map[string]ProcessDefinitionDoc, launchDocs map[string]ProcessLaunchDoc, procDocs map[string]ProcessDoc) (*process.Info, int) {
	missing := 0
	name, _ := process.ParseID(id)
	definitionDoc, ok := definitionDocs[name]
	if !ok {
		missing += 1
	}
	launchDoc, ok := launchDocs[id]
	if !ok {
		missing += 2
	}
	procDoc, ok := procDocs[id]
	if !ok {
		missing += 4
	}
	if missing > 0 {
		return nil, missing
	}

	doc := processInfoDoc{
		definition: definitionDoc,
		launch:     launchDoc,
		proc:       procDoc,
	}
	info := doc.info()
	return &info, 0
}

func (pp Persistence) checkRecords(id string) (bool, error) {
	missing := 0
	_, err := pp.definition(id)
	if errors.IsNotFound(err) {
		missing += 1
	} else if err != nil {
		return false, errors.Trace(err)
	}
	_, err = pp.launch(id)
	if errors.IsNotFound(err) {
		missing += 2
	} else if err != nil {
		return false, errors.Trace(err)
	}
	_, err = pp.proc(id)
	if errors.IsNotFound(err) {
		missing += 4
	} else if err != nil {
		return false, errors.Trace(err)
	}
	if missing > 0 {
		if missing < 7 {
			return false, errors.Errorf("found inconsistent records for process %q", id)
		}
		return false, nil
	}
	return true, nil
}

// TODO(ericsnow) Factor most of the below into a processesCollection type.

func (pp Persistence) one(id string, doc interface{}) error {
	return errors.Trace(pp.st.One(workloadProcessesC, id, doc))
}

func (pp Persistence) all(query, docs interface{}) error {
	return errors.Trace(pp.st.All(workloadProcessesC, query, docs))
}

func (pp Persistence) definitionID(id string) string {
	name, _ := process.ParseID(id)
	// The URL will always parse successfully.
	charmURL, _ := charm.ParseURL(pp.charm.Id())
	return fmt.Sprintf("%s#%s", charmGlobalKey(charmURL), name)
}

func (pp Persistence) processID(id string) string {
	return fmt.Sprintf("%s#%s", unitGlobalKey(pp.unit.Id()), id)
}

func (pp Persistence) launchID(id string) string {
	return pp.processID(id) + "#launch"
}

func (pp Persistence) newInsertDefinitionOp(definition charm.Process) txn.Op {
	doc := pp.newProcessDefinitionDoc(definition)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp Persistence) newInsertProcessOps(info process.Info) []txn.Op {
	var ops []txn.Op
	ops = append(ops, pp.newInsertLaunchOp(info))
	ops = append(ops, pp.newInsertProcOp(info))
	return ops
}

func (pp Persistence) newInsertLaunchOp(info process.Info) txn.Op {
	doc := pp.newLaunchDoc(info)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp Persistence) newInsertProcOp(info process.Info) txn.Op {
	doc := pp.newProcessDoc(info)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func (pp Persistence) newSetRawStatusOps(id string, status process.Status) []txn.Op {
	id = pp.processID(id)
	return []txn.Op{{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
	}, {
		C:      workloadProcessesC,
		Id:     id,
		Assert: IsAliveDoc,
		Update: bson.D{{"$set", bson.D{{"pluginstatus", status.Label}}}},
	}}
}

func (pp Persistence) newRemoveProcessOps(id string) []txn.Op {
	var ops []txn.Op
	ops = append(ops, pp.newRemoveLaunchOp(id))
	ops = append(ops, pp.newRemoveProcOps(id)...)
	return ops
}

func (pp Persistence) newRemoveLaunchOp(id string) txn.Op {
	id = pp.launchID(id)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}
}

func (pp Persistence) newRemoveProcOps(id string) []txn.Op {
	id = pp.processID(id)
	return []txn.Op{{
		C:      workloadProcessesC,
		Id:     id,
		Assert: IsAliveDoc,
	}, {
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}}
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

// ProcessDefinitionDoc is the document for process definitions.
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
		fmt.Sscanf(raw, "%s:%s:%s:%s", &v.ExternalMount, &v.InternalMount, &v.Mode, &v.Name)
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

func (pp Persistence) newProcessDefinitionDoc(definition charm.Process) *ProcessDefinitionDoc {
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

func (pp Persistence) definition(id string) (*ProcessDefinitionDoc, error) {
	id = pp.definitionID(id)

	var doc ProcessDefinitionDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allDefinitions() (map[string]ProcessDefinitionDoc, error) {
	var docs []ProcessDefinitionDoc
	prefix := pp.definitionID(".*")
	query := bson.M{"$in": []string{"/^" + prefix + "/"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessDefinitionDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-1]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) definitions(ids []string) (map[string]ProcessDefinitionDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.definitionID(id)
		fullIDs[i] = fullID
		name, _ := process.ParseID(id)
		idMap[fullID] = name
	}

	var docs []ProcessDefinitionDoc
	query := bson.M{"$in": fullIDs}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessDefinitionDoc)
	for _, doc := range docs {
		id := idMap[doc.DocID]
		results[id] = doc
	}
	return results, nil
}

// ProcessLaunchDoc is the document for process launch details.
type ProcessLaunchDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	PluginID  string `bson:"pluginid"`
	RawStatus string `bson:"rawstatus"`
}

func (d ProcessLaunchDoc) details() process.Details {
	return process.Details{
		ID: d.PluginID,
		Status: process.Status{
			Label: d.RawStatus,
		},
	}
}

func (pp Persistence) newLaunchDoc(info process.Info) *ProcessLaunchDoc {
	id := pp.launchID(info.ID())
	return &ProcessLaunchDoc{
		DocID: id,

		PluginID:  info.Details.ID,
		RawStatus: info.Details.Status.Label,
	}
}

func (pp Persistence) launch(id string) (*ProcessLaunchDoc, error) {
	id = pp.launchID(id)

	var doc ProcessLaunchDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allLaunches() (map[string]ProcessLaunchDoc, error) {
	var docs []ProcessLaunchDoc
	prefix := pp.launchID(".*")
	query := bson.M{"$in": []string{"/^" + prefix + "/"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessLaunchDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-2]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) launches(ids []string) (map[string]ProcessLaunchDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.launchID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []ProcessLaunchDoc
	query := bson.M{"$in": fullIDs}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessLaunchDoc)
	for _, doc := range docs {
		id := idMap[doc.DocID]
		results[id] = doc
	}
	return results, nil
}

// TODO(ericsnow) The life stuff here is just a temporary hack.

type Life int8 // a mirror of state.Life.

const (
	Alive Life = iota
	Dying
	Dead
)

var IsAliveDoc = bson.D{{"life", Alive}}

// ProcessDoc is the top-level document for processes.
type ProcessDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	Life         Life   `bson:"life"`
	PluginStatus string `bson:"pluginstatus"`
}

func (d ProcessDoc) info() process.Info {
	return process.Info{
		Details: process.Details{
			Status: process.Status{
				Label: d.PluginStatus,
			},
		},
	}
}

func (pp Persistence) newProcessDoc(info process.Info) *ProcessDoc {
	id := pp.processID(info.ID())

	return &ProcessDoc{
		DocID: id,

		Life:         Alive,
		PluginStatus: info.Details.Status.Label,
	}
}

func (pp Persistence) proc(id string) (*ProcessDoc, error) {
	id = pp.processID(id)

	var doc ProcessDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allProcs() (map[string]ProcessDoc, error) {
	var docs []ProcessDoc
	prefix := pp.processID("[^#]*")
	query := bson.M{"$in": []string{"/^" + prefix + "/"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-1]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) procs(ids []string) (map[string]ProcessDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.processID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []ProcessDoc
	query := bson.M{"$in": fullIDs}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]ProcessDoc)
	for _, doc := range docs {
		id := idMap[doc.DocID]
		results[id] = doc
	}
	return results, nil
}
