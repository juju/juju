// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type setPodSpecOperation struct {
	m      *CAASModel
	appTag names.ApplicationTag
	spec   *string
}

// Build implements ModelOperation.
func (op *setPodSpecOperation) Build(attempt int) ([]txn.Op, error) {
	var prereqOps []txn.Op
	appTagID := op.appTag.Id()
	app, err := op.m.State().Application(appTagID)
	if err != nil {
		return nil, errors.Annotate(err, "setting pod spec")
	}
	if app.Life() != Alive {
		return nil, errors.Annotate(
			errors.Errorf("application %s not alive", app.String()),
			"setting pod spec",
		)
	}
	prereqOps = append(prereqOps, txn.Op{
		C:      applicationsC,
		Id:     app.doc.DocID,
		Assert: isAliveDoc,
	})

	sop := txn.Op{
		C:  podSpecsC,
		Id: applicationGlobalKey(appTagID),
	}
	existing, err := op.m.podInfo(op.appTag)
	if err == nil {
		updates := bson.D{{"$inc", bson.D{{"upgrade-counter", 1}}}}
		if op.spec != nil {
			updates = append(updates, bson.DocElem{"$set", bson.D{{"spec", *op.spec}}})
		}
		sop.Assert = bson.D{{"upgrade-counter", existing.UpgradeCounter}}
		sop.Update = updates
	} else if errors.IsNotFound(err) {
		sop.Assert = txn.DocMissing
		var specStr string
		if op.spec != nil {
			specStr = *op.spec
		}
		sop.Insert = containerSpecDoc{Spec: specStr}
	} else {
		return nil, errors.Annotate(err, "setting pod spec")
	}
	return append(prereqOps, sop), nil
}

// Done implements ModelOperation.
func (op *setPodSpecOperation) Done(err error) error { return err }
