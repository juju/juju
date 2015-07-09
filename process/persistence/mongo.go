// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

// TODO(ericsnow) Move the methods under their own type (processcollection?).

func (pp Persistence) extractProc(id string, procDocs map[string]processDoc) (*process.Info, bool) {
	procDoc, ok := procDocs[id]
	if !ok {
		return nil, false
	}
	info := procDoc.info()
	return &info, true
}

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

func (pp Persistence) processID(id string) string {
	return fmt.Sprintf("proc#%s#%s", pp.unit.Id(), id)
}

func (pp Persistence) newInsertProcessOps(info process.Info) []txn.Op {
	var ops []txn.Op

	doc := pp.newProcessDoc(info)
	ops = append(ops, txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	})

	return ops
}

func (pp Persistence) newSetRawStatusOps(id string, status process.PluginStatus) []txn.Op {
	id = pp.processID(id)
	return []txn.Op{{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"pluginstatus", status.Label}}}},
	}}
}

func (pp Persistence) newRemoveProcessOps(id string) []txn.Op {
	id = pp.processID(id)
	return []txn.Op{{
		C:      workloadProcessesC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

// processDoc is the top-level document for processes.
type processDoc struct {
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

	PluginID       string `bson:"pluginid"`
	OriginalStatus string `bson:"origstatus"`

	PluginStatus string `bson:"pluginstatus"`
}

func (d processDoc) info() process.Info {
	info := process.Info{
		Process: d.definition(),
		Details: d.details(),
	}
	info.Details.Status.Label = d.PluginStatus
	return info
}

func (d processDoc) definition() charm.Process {
	definition := charm.Process{
		Name:        d.Name,
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

func (d processDoc) details() process.Details {
	return process.Details{
		ID: d.PluginID,
		Status: process.PluginStatus{
			Label: d.OriginalStatus,
		},
	}
}

func (pp Persistence) newProcessDoc(info process.Info) *processDoc {
	definition := info.Process

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

	id := pp.processID(info.ID())
	return &processDoc{
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

		PluginID:       info.Details.ID,
		OriginalStatus: info.Details.Status.Label,

		PluginStatus: info.Details.Status.Label,
	}
}

func (pp Persistence) allProcs() (map[string]processDoc, error) {
	var docs []processDoc
	query := bson.D{{"unitid", pp.unit.Id()}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]processDoc)
	for _, doc := range docs {
		id := doc.info().ID()
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
