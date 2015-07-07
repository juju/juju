// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Move this to a subpackage and split it up?

package persistence

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
)

const (
	workloadProcessesC = "workloadprocesses"
)

// Collections is the list of names of the mongo collections where state
// is stored for workload processes.
var Collections = []string{
	workloadProcessesC,
}

// TODO(ericsnow) Move the methods under their own type.

func (pp Persistence) indexDefinitionDocs(ids []string) (map[interface{}]definitionDoc, error) {
	var docs []definitionDoc
	query := bson.D{{"$in", ids}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	indexed := make(map[interface{}]definitionDoc)
	for _, doc := range docs {
		indexed[doc.DocID] = doc
	}
	return indexed, nil
}

func (pp Persistence) extractProc(id string, definitionDocs map[string]definitionDoc, launchDocs map[string]launchDoc, procDocs map[string]processDoc) (*process.Info, int) {
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

func dropEnvUUID(id string) string {
	fullID := id
	parts := strings.SplitN(fullID, ":", 2)
	if len(parts) == 2 {
		if names.IsValidEnvironment(parts[0]) {
			fullID = parts[1]
		}
	}
	return fullID
}

// TODO(ericsnow) Factor most of the below into a processesCollection type.

func (pp Persistence) one(id string, doc interface{}) error {
	return errors.Trace(pp.st.One(workloadProcessesC, id, doc))
}

func (pp Persistence) all(query bson.D, docs interface{}) error {
	return errors.Trace(pp.st.All(workloadProcessesC, query, docs))
}

func (pp Persistence) allID(query bson.D, docs interface{}) error {
	if query != nil {
		query = bson.D{{"_id", query}}
	}
	return errors.Trace(pp.all(query, docs))
}

func (pp Persistence) definitionID(id string) string {
	name, _ := process.ParseID(id)
	return fmt.Sprintf("procd#%s#%s", pp.charm.Id(), name)
}

func (pp Persistence) processID(id string) string {
	return fmt.Sprintf("proc#%s#%s", pp.unit.Id(), id)
}

func (pp Persistence) launchID(id string) string {
	return pp.processID(id) + "#launch"
}

func (pp Persistence) newInsertDefinitionOp(definition charm.Process) txn.Op {
	doc := pp.newdefinitionDoc(definition)
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
	doc := pp.newprocessDoc(info)
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
		Assert: txn.DocExists,
		Remove: true,
	}}
}

type processInfoDoc struct {
	definition definitionDoc
	launch     launchDoc
	proc       processDoc
}

func (d processInfoDoc) info() process.Info {
	info := d.proc.info()

	info.Process = d.definition.definition()

	rawStatus := info.Details.Status
	info.Details = d.launch.details()
	info.Details.Status = rawStatus

	return info
}

// definitionDoc is the document for process definitions.
type definitionDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	CharmID  string `bson:"charmid"`
	ProcName string `bson:"procname"`
	DocKind  string `bson:"dockind"`

	UnitID string `bson:"unitid"`

	Description string            `bson:"description"`
	Type        string            `bson:"type"`
	TypeOptions map[string]string `bson:"typeoptions"`
	Command     string            `bson:"command"`
	Image       string            `bson:"image"`
	Ports       []string          `bson:"ports"`
	Volumes     []string          `bson:"volumes"`
	EnvVars     map[string]string `bson:"envvars"`
}

func (d definitionDoc) definition() charm.Process {
	definition := charm.Process{
		Name:        d.ProcName,
		Description: d.Description,
		Type:        d.Type,
		Command:     d.Command,
		Image:       d.Image,
	}

	if len(d.TypeOptions) > 0 {
		definition.TypeOptions = d.TypeOptions
	}

	if len(d.EnvVars) > 0 {
		definition.EnvVars = d.EnvVars
	}

	if len(d.Ports) > 0 {
		ports := make([]charm.ProcessPort, len(d.Ports))
		for i, raw := range d.Ports {
			p := ports[i]
			fmt.Sscanf(raw, "%d:%d:%s", &p.External, &p.Internal, &p.Endpoint)
		}
		definition.Ports = ports
	}

	if len(d.Volumes) > 0 {
		volumes := make([]charm.ProcessVolume, len(d.Volumes))
		for i, raw := range d.Volumes {
			v := volumes[i]
			fmt.Sscanf(raw, "%s:%s:%s:%s", &v.ExternalMount, &v.InternalMount, &v.Mode, &v.Name)
		}
		definition.Volumes = volumes
	}

	return definition
}

func (pp Persistence) newdefinitionDoc(definition charm.Process) *definitionDoc {
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

	return &definitionDoc{
		DocID: id,

		CharmID:  pp.charmID(),
		ProcName: definition.Name,
		DocKind:  "definition",

		UnitID: pp.unit.Id(),

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

func (pp Persistence) definition(id string) (*definitionDoc, error) {
	id = pp.definitionID(id)

	var doc definitionDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allDefinitions() (map[string]definitionDoc, error) {
	var docs []definitionDoc
	query := bson.D{{"dockind", "definition"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]definitionDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-1]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) definitions(ids []string) (map[string]definitionDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.definitionID(id)
		fullIDs[i] = fullID
		name, _ := process.ParseID(id)
		idMap[fullID] = name
	}

	var docs []definitionDoc
	query := bson.D{{"$in", fullIDs}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]definitionDoc)
	for _, doc := range docs {
		fullID := dropEnvUUID(doc.DocID)
		id := idMap[fullID]
		results[id] = doc
	}
	return results, nil
}

// launchDoc is the document for process launch details.
type launchDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	UnitID   string `bson:"unitid"`
	ProcName string `bson:"procname"`
	PluginID string `bson:"pluginid"`
	DocKind  string `bson:"dockind"`

	RawStatus string `bson:"rawstatus"`
}

func (d launchDoc) details() process.Details {
	return process.Details{
		ID: d.PluginID,
		Status: process.Status{
			Label: d.RawStatus,
		},
	}
}

func (pp Persistence) newLaunchDoc(info process.Info) *launchDoc {
	id := pp.launchID(info.ID())
	return &launchDoc{
		DocID: id,

		UnitID:   pp.unit.Id(),
		ProcName: info.Name,
		PluginID: info.Details.ID,
		DocKind:  "launch",

		RawStatus: info.Details.Status.Label,
	}
}

func (pp Persistence) launch(id string) (*launchDoc, error) {
	id = pp.launchID(id)

	var doc launchDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allLaunches() (map[string]launchDoc, error) {
	var docs []launchDoc
	query := bson.D{{"dockind", "launch"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]launchDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-2]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) launches(ids []string) (map[string]launchDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.launchID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []launchDoc
	query := bson.D{{"$in", fullIDs}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]launchDoc)
	for _, doc := range docs {
		fullID := dropEnvUUID(doc.DocID)
		id := idMap[fullID]
		results[id] = doc
	}
	return results, nil
}

// processDoc is the top-level document for processes.
type processDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	UnitID   string `bson:"unitid"`
	ProcName string `bson:"procname"`
	PluginID string `bson:"pluginid"`
	DocKind  string `bson:"dockind"`

	PluginStatus string `bson:"pluginstatus"`
}

func (d processDoc) info() process.Info {
	return process.Info{
		Details: process.Details{
			Status: process.Status{
				Label: d.PluginStatus,
			},
		},
	}
}

func (pp Persistence) newprocessDoc(info process.Info) *processDoc {
	id := pp.processID(info.ID())

	return &processDoc{
		DocID: id,

		UnitID:   pp.unit.Id(),
		ProcName: info.Name,
		PluginID: info.Details.ID,
		DocKind:  "process",

		PluginStatus: info.Details.Status.Label,
	}
}

func (pp Persistence) proc(id string) (*processDoc, error) {
	id = pp.processID(id)

	var doc processDoc
	if err := pp.one(id, &doc); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

func (pp Persistence) allProcs() (map[string]processDoc, error) {
	var docs []processDoc
	query := bson.D{{"dockind", "process"}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]processDoc)
	for _, doc := range docs {
		parts := strings.Split(doc.DocID, "#")
		id := parts[len(parts)-1]
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) procs(ids []string) (map[string]processDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.processID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []processDoc
	query := bson.D{{"$in", fullIDs}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]processDoc)
	for _, doc := range docs {
		fullID := dropEnvUUID(doc.DocID)
		id := idMap[fullID]
		results[id] = doc
	}
	return results, nil
}
