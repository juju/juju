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
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
)

// TODO(ericsnow) Implement persistence using a TXN abstraction (used
// in the business logic) with ops factories available from the
// persistence layer.

type processesPersistenceBase interface {
	getCollection(name string) (stateCollection, func())
	run(transactions jujutxn.TransactionSource) error
}

type processesPersistence struct {
	st    processesPersistenceBase
	charm names.CharmTag
	unit  names.UnitTag
}

func (pp processesPersistence) ensureDefinitions(definitions ...charm.Process) error {
	// Add definition if not already added (or ensure matches).
	var ops []txn.Op
	for _, definition := range definitions {
		ops = append(ops, pp.newInsertDefinitionOp(definition))
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// The last attempt aborted so clear out any ops that failed
			// the DocMissing assertion and try again.
			coll, closeColl := pp.st.getCollection(workloadProcessesC)
			defer closeColl()
			var okOps []txn.Op
			for _, op := range ops {
				var doc processDefinitionDoc
				err := coll.FindId(op.Id).One(&doc)
				if err == mgo.ErrNotFound {
					okOps = append(okOps, op)
				} else if err != nil {
					return nil, errors.Trace(err)
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
	if err := pp.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (pp processesPersistence) insert(info process.Info) error {
	// Ensure defined.

	// Add launch info.
	// Add process info.

	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) setStatus(id string, status process.Status) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) list(ids ...string) ([]process.Info, error) {
	// TODO(ericsnow) finish!
	return nil, errors.Errorf("not finished")
}

func (pp processesPersistence) remove(id string) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) definitionID(name string) string {
	// The URL will always parse successfully.
	charmURL, _ := charm.ParseURL(pp.charm.Id())
	return fmt.Sprintf("%s#%s", charmGlobalKey(charmURL), name)
}

func (pp processesPersistence) processID(id string) string {
	return fmt.Sprintf("%s#%s", unitGlobalKey(pp.unit.Id()), id)
}

func (pp processesPersistence) newInsertDefinitionOp(definition charm.Process) txn.Op {
	doc := pp.newProcessDefinitionDoc(definition)
	return txn.Op{
		C:      workloadProcessesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

type processDefinitionDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"` // XXX needed?

	UnitID string `bson:"unitid"`

	Name        string `bson:"name"`
	Description string `bson:"description"`
	Type        string `bson:"type"`
	TypeOptions XXX    `bson:"typeoptions"`
	Command     string `bson:"command"`
	Image       string `bson:"image"`
	Ports       XXX    `bson:"ports"`
	Volumes     XXX    `bson:"volumes"`
	EnvVars     XXX    `bson:"envvars"`
}

func (pp processesPersistence) newProcessDefinitionDoc(definition charm.Process) *processDefinitionDoc {
	id := pp.definitionID(definition.Name)
	return &processDefinitionDoc{
		DocID:  id,
		UnitID: pp.unit.Id(),

		Name:        definition.Name,
		Description: definition.Description,
		Type:        definition.Type,
		TypeOptions: definition.TypeOptions,
		Command:     definition.Command,
		Image:       definition.Image,
		Ports:       definition.Ports,
		Volumes:     definition.Volumes,
		EnvVars:     definition.EnvVars,
	}
}
