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
	// TODO(ericsnow) Drop the unit part.
	return fmt.Sprintf("workload#%s#%s", pp.unit.Id(), id)
}

func (pp Persistence) extractWorkloadID(docID string) string {
	parts := strings.Split(docID, "#")
	return parts[len(parts)-1]
}

func (pp Persistence) newInsertWorkloadOps(id string, info workload.Info) []txn.Op {
	var ops []txn.Op

	doc := pp.newWorkloadDoc(id, info)
	ops = append(ops, txn.Op{
		C:      workloadsC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	})

	return ops
}

func (pp Persistence) newSetRawStatusOps(id, status string) []txn.Op {
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

	Name string `bson:"name"`
	Type string `bson:"type"`

	State   string `bson:"state"`
	Blocker string `bson:"blocker"`
	Status  string `bson:"status"`

	Tags []string `bson:"tags"`

	PluginID       string `bson:"pluginid"`
	OriginalStatus string `bson:"origstatus"`

	PluginStatus string `bson:"pluginstatus"`
}

func (d workloadDoc) info() workload.Info {
	tags := make([]string, len(d.Tags))
	copy(tags, d.Tags)
	info := workload.Info{
		PayloadClass: d.definition(),
		Status:       d.status(),
		Tags:         tags,
		Details:      d.details(),
	}
	info.Details.Status.State = d.PluginStatus
	return info
}

func (d workloadDoc) definition() charm.PayloadClass {
	definition := charm.PayloadClass{
		Name: d.Name,
		Type: d.Type,
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

func (d workloadDoc) match(name, rawID string) bool {
	if d.Name != name {
		return false
	}
	if d.PluginID != rawID {
		return false
	}
	return true
}

func (pp Persistence) newWorkloadDoc(id string, info workload.Info) *workloadDoc {
	id = pp.workloadID(id)

	definition := info.PayloadClass

	tags := make([]string, len(info.Tags))
	copy(tags, info.Tags)

	return &workloadDoc{
		DocID:  id,
		UnitID: pp.unit.Id(),

		Name: definition.Name,
		Type: definition.Type,

		State:   info.Status.State,
		Blocker: info.Status.Blocker,
		Status:  info.Status.Message,

		Tags: tags,

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
		id := pp.extractWorkloadID(doc.DocID)
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
