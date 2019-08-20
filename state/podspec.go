// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type containerSpecDoc struct {
	// Id holds container spec document key.
	// It is the global key of the application represented
	// by this container.
	Id string `bson:"_id"`

	Spec string `bson:"spec"`
}

// SetPodSpec sets the pod spec for the given application tag.
// An error will be returned if the specified application is not alive.
func (m *CAASModel) SetPodSpec(appTag names.ApplicationTag, spec string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var prereqOps []txn.Op
		app, err := m.State().Application(appTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if app.Life() != Alive {
			return nil, errors.Errorf("application %s not alive", app.String())
		}
		prereqOps = append(prereqOps, txn.Op{
			C:      applicationsC,
			Id:     app.doc.DocID,
			Assert: isAliveDoc,
		})

		op := txn.Op{
			C:  podSpecsC,
			Id: applicationGlobalKey(appTag.Id()),
		}
		existing, err := m.PodSpec(appTag)
		if err == nil {
			if existing == spec {
				return nil, jujutxn.ErrNoOperations
			}
			op.Assert = txn.DocExists
			op.Update = bson.D{{"$set", bson.D{{"spec", spec}}}}
		} else if errors.IsNotFound(err) {
			op.Assert = txn.DocMissing
			op.Insert = containerSpecDoc{Spec: spec}
		} else {
			return nil, err
		}
		return append(prereqOps, op), nil
	}
	return m.mb.db().Run(buildTxn)
}

// PodSpec returns the pod spec for the given application tag.
func (m *CAASModel) PodSpec(appTag names.ApplicationTag) (string, error) {
	coll, cleanup := m.mb.db().GetCollection(podSpecsC)
	defer cleanup()
	var doc containerSpecDoc
	if err := coll.FindId(applicationGlobalKey(appTag.Id())).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return "", errors.NotFoundf(
				"pod spec for %s",
				names.ReadableString(appTag),
			)
		}
		return "", errors.Trace(err)
	}
	return doc.Spec, nil
}

func removePodSpecOp(appTag names.ApplicationTag) txn.Op {
	return txn.Op{
		C:      podSpecsC,
		Id:     applicationGlobalKey(appTag.Id()),
		Remove: true,
	}
}
