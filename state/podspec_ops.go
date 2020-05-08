// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
)

type setPodSpecOperation struct {
	m       *CAASModel
	appTag  names.ApplicationTag
	spec    *string
	rawSpec *string

	tokenAwareTxnBuilder func(int) ([]txn.Op, error)
}

// newSetPodSpecOperation returns a ModelOperation for updating the PodSpec or
// for a particular application. A nil token can be specified to bypass the
// leadership check.
func newSetPodSpecOperation(model *CAASModel, token leadership.Token, appTag names.ApplicationTag, spec *string) *setPodSpecOperation {
	op := &setPodSpecOperation{
		m:      model,
		appTag: appTag,
		spec:   spec,
	}

	if token != nil {
		op.tokenAwareTxnBuilder = buildTxnWithLeadership(op.buildTxn, token)
	}
	return op
}

// newSetRawK8sSpecOperation returns a ModelOperation for updating the raw k8s spec
// for a particular application. A nil token can be specified to bypass the
// leadership check.
func newSetRawK8sSpecOperation(model *CAASModel, token leadership.Token, appTag names.ApplicationTag, rawSpec *string) *setPodSpecOperation {
	op := &setPodSpecOperation{
		m:       model,
		appTag:  appTag,
		rawSpec: rawSpec,
	}

	if token != nil {
		op.tokenAwareTxnBuilder = buildTxnWithLeadership(op.buildTxn, token)
	}
	return op
}

// Build implements ModelOperation.
func (op *setPodSpecOperation) Build(attempt int) ([]txn.Op, error) {
	if op.tokenAwareTxnBuilder != nil {
		return op.tokenAwareTxnBuilder(attempt)
	}
	return op.buildTxn(attempt)
}

func (op *setPodSpecOperation) buildTxn(_ int) ([]txn.Op, error) {
	if op.spec != nil && op.rawSpec != nil {
		return nil, errors.NewForbidden(nil, "either spec or raw k8s spec can be set for each application, but not both")
	}

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
	// The app's charm may not be there yet (as is the case when migrating).
	// This check is for checking the k8s-spec-set/k8s-raw-set call.
	ch, _, err := app.Charm()
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	} else if err == nil {
		if ch.Meta().Deployment != nil && ch.Meta().Deployment.DeploymentMode == charm.ModeOperator {
			return nil, errors.New("cannot set k8s spec on an operator charm")
		}
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
		asserts := bson.D{{Name: "upgrade-counter", Value: existing.UpgradeCounter}}
		if existing.UpgradeCounter == 0 {
			asserts = bson.D{
				bson.DocElem{
					Name: "$or", Value: []bson.D{
						{{Name: "upgrade-counter", Value: 0}},
						{{
							Name: "upgrade-counter",
							Value: bson.D{
								{Name: "$exists", Value: false},
							},
						}},
					},
				},
			}
		}
		updates := bson.D{{Name: "$inc", Value: bson.D{{"upgrade-counter", 1}}}}
		// Either "spec" or "raw-spec" can be set for each application.
		if op.spec != nil {
			updates = append(updates, bson.DocElem{Name: "$set", Value: bson.D{{"spec", *op.spec}}})
			asserts = append(asserts, getEmptyStringFieldAssert("raw-spec"))
		}
		if op.rawSpec != nil {
			updates = append(updates, bson.DocElem{Name: "$set", Value: bson.D{{"raw-spec", *op.rawSpec}}})
			asserts = append(asserts, getEmptyStringFieldAssert("spec"))
		}
		sop.Assert = asserts
		sop.Update = updates
	} else if errors.IsNotFound(err) {
		sop.Assert = txn.DocMissing
		newDoc := containerSpecDoc{}
		if op.spec != nil {
			newDoc.Spec = *op.spec
		}
		if op.rawSpec != nil {
			newDoc.RawSpec = *op.rawSpec
		}
		sop.Insert = newDoc
	} else {
		return nil, errors.Annotate(err, "setting pod spec")
	}
	return append(prereqOps, sop), nil
}

// Done implements ModelOperation.
func (op *setPodSpecOperation) Done(err error) error { return err }

func getEmptyStringFieldAssert(fieldName string) bson.DocElem {
	return bson.DocElem{
		Name: "$or", Value: []bson.D{
			{{Name: fieldName, Value: ""}},
			{{
				Name: fieldName,
				Value: bson.D{
					{Name: "$exists", Value: false},
				},
			}},
		},
	}
}
