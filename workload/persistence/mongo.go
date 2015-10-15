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

	"github.com/juju/juju/workload"
)

const (
	workloadsC = "workloads"
)

// Collections is the list of names of the mongo collections where state
// is stored for workloads.
// TODO(ericsnow) Not needed anymore...modify for a new registration scheme?
var Collections = []string{
	workloadsC,
}

// TODO(ericsnow) Move the methods under their own type (workloadcollection?).

func (pp Persistence) extractWorkload(id string, workloadDocs map[string]workloadDoc) (*workload.Info, bool) {
	workloadDoc, ok := workloadDocs[id]
	if !ok {
		return nil, false
	}
	info := workloadDoc.info()
	return &info, true
}

func (pp Persistence) one(id string, doc interface{}) error {
	return errors.Trace(pp.st.One(workloadsC, id, doc))
}

func (pp Persistence) all(query bson.D, docs interface{}) error {
	return errors.Trace(pp.st.All(workloadsC, query, docs))
}

func (pp Persistence) allID(query bson.D, docs interface{}) error {
	if query != nil {
		query = bson.D{{"_id", query}}
	}
	return errors.Trace(pp.all(query, docs))
}

func (pp Persistence) workloadID(id string) string {
	return fmt.Sprintf("workload#%s#%s", pp.unit.Id(), id)
}

func (pp Persistence) newInsertWorkloadOps(info workload.Info) []txn.Op {
	var ops []txn.Op

	doc := pp.newWorkloadDoc(info)
	ops = append(ops, txn.Op{
		C:      workloadsC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	})

	return ops
}

func (pp Persistence) newSetRawStatusOps(status, id string) []txn.Op {
	id = pp.workloadID(id)
	updates := bson.D{
		{"state", status},
		{"status", status},
		{"pluginstatus", status},
	}
	return []txn.Op{{
		C:      workloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", updates}},
	}}
}

func (pp Persistence) newRemoveWorkloadOps(id string) []txn.Op {
	id = pp.workloadID(id)
	return []txn.Op{{
		C:      workloadsC,
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

// workloadDoc is the top-level document for workloads.
type workloadDoc struct {
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

	State   string `bson:"state"`
	Blocker string `bson:"blocker"`
	Status  string `bson:"status"`

	PluginID       string `bson:"pluginid"`
	OriginalStatus string `bson:"origstatus"`

	PluginStatus string `bson:"pluginstatus"`
}

func (d workloadDoc) info() workload.Info {
	info := workload.Info{
		Workload: d.definition(),
		Status:   d.status(),
		Details:  d.details(),
	}
	info.Details.Status.State = d.PluginStatus
	return info
}

func (d workloadDoc) definition() charm.Workload {
	definition := charm.Workload{
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
		ports := make([]charm.WorkloadPort, len(d.Ports))
		for i, raw := range d.Ports {
			p := &ports[i]
			fmt.Sscanf(raw, "%d:%d:%s", &p.External, &p.Internal, &p.Endpoint)
		}
		definition.Ports = ports
	}

	if len(d.Volumes) > 0 {
		volumes := make([]charm.WorkloadVolume, len(d.Volumes))
		for i, raw := range d.Volumes {
			parts := strings.Split(raw, ":")
			// len(parts) will always be 4.
			volumes[i] = charm.WorkloadVolume{
				ExternalMount: parts[0],
				InternalMount: parts[1],
				Mode:          parts[2],
				Name:          parts[3],
			}
		}
		definition.Volumes = volumes
	}

	return definition
}

func (d workloadDoc) status() workload.Status {
	return workload.Status{
		State:   d.State,
		Blocker: d.Blocker,
		Message: d.Status,
	}
}

func (d workloadDoc) details() workload.Details {
	return workload.Details{
		ID: d.PluginID,
		Status: workload.PluginStatus{
			State: d.OriginalStatus,
		},
	}
}

func (pp Persistence) newWorkloadDoc(info workload.Info) *workloadDoc {
	definition := info.Workload

	var ports []string
	for _, p := range definition.Ports {
		ports = append(ports, fmt.Sprintf("%d:%d:%s", p.External, p.Internal, p.Endpoint))
	}

	var volumes []string
	for _, v := range definition.Volumes {
		volumes = append(volumes, fmt.Sprintf("%s:%s:%s:%s", v.ExternalMount, v.InternalMount, v.Mode, v.Name))
	}

	id := pp.workloadID(info.ID())
	return &workloadDoc{
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

		State:   info.Status.State,
		Blocker: info.Status.Blocker,
		Status:  info.Status.Message,

		PluginID:       info.Details.ID,
		OriginalStatus: info.Details.Status.State,

		PluginStatus: info.Details.Status.State,
	}
}

func (pp Persistence) allWorkloads() (map[string]workloadDoc, error) {
	var docs []workloadDoc
	query := bson.D{{"unitid", pp.unit.Id()}}
	if err := pp.all(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]workloadDoc)
	for _, doc := range docs {
		id := doc.info().ID()
		results[id] = doc
	}
	return results, nil
}

func (pp Persistence) workloads(ids []string) (map[string]workloadDoc, error) {
	fullIDs := make([]string, len(ids))
	idMap := make(map[string]string, len(ids))
	for i, id := range ids {
		fullID := pp.workloadID(id)
		fullIDs[i] = fullID
		idMap[fullID] = id
	}

	var docs []workloadDoc
	query := bson.D{{"$in", fullIDs}}
	if err := pp.allID(query, &docs); err != nil {
		return nil, errors.Trace(err)
	}

	results := make(map[string]workloadDoc)
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
