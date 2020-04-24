// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
)

// ExternalController represents the state of a controller hosting
// other models.
type ExternalController interface {
	// Id returns the external controller UUID, also used as
	// the mongo id.
	Id() string

	// ControllerInfo returns the details required to connect to the
	// external controller.
	ControllerInfo() crossmodel.ControllerInfo
}

// externalController is an implementation of ExternalController.
type externalController struct {
	doc externalControllerDoc
}

type externalControllerDoc struct {
	// Id holds external controller document key.
	// It is the controller UUID.
	Id string `bson:"_id"`

	// Alias holds an alias (human friendly) name for the controller.
	Alias string `bson:"alias"`

	// Addrs holds the host:port values for the external
	// controller's API server.
	Addrs []string `bson:"addresses"`

	// CACert holds the certificate to validate the external
	// controller's target API server's TLS certificate.
	CACert string `bson:"cacert"`

	// Models holds model UUIDs hosted on this controller.
	Models []string `bson:"models"`
}

// newExternalControllerDoc returns a new external controller document
// reference representing the input controller info.
func newExternalControllerDoc(controller crossmodel.ControllerInfo) *externalControllerDoc {
	return &externalControllerDoc{
		Id:     controller.ControllerTag.Id(),
		Alias:  controller.Alias,
		Addrs:  controller.Addrs,
		CACert: controller.CACert,
	}
}

// Id implements ExternalController.
func (rc *externalController) Id() string {
	return rc.doc.Id
}

// ControllerInfo implements ExternalController.
func (rc *externalController) ControllerInfo() crossmodel.ControllerInfo {
	return crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(rc.doc.Id),
		Alias:         rc.doc.Alias,
		Addrs:         rc.doc.Addrs,
		CACert:        rc.doc.CACert,
	}
}

// ExternalControllers instances provide access to external controllers in state.
type ExternalControllers interface {
	Save(_ crossmodel.ControllerInfo, modelUUIDs ...string) (ExternalController, error)
	SaveAndMoveModels(_ crossmodel.ControllerInfo, modelUUIDs ...string) error
	Controller(controllerUUID string) (ExternalController, error)
	ControllerForModel(modelUUID string) (ExternalController, error)
	Remove(controllerUUID string) error
	Watch() StringsWatcher
	WatchController(controllerUUID string) NotifyWatcher
}

type externalControllers struct {
	st *State
}

// NewExternalControllers creates an external controllers instance backed by a state.
func NewExternalControllers(st *State) *externalControllers {
	return &externalControllers{st: st}
}

// Save creates or updates an external controller record.
func (ec *externalControllers) Save(
	controller crossmodel.ControllerInfo, modelUUIDs ...string,
) (ExternalController, error) {
	if err := controller.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	doc := newExternalControllerDoc(controller)
	buildTxn := func(int) ([]txn.Op, error) {
		ops, err := ec.upsertExternalControllerOps(doc, modelUUIDs)
		return ops, errors.Trace(err)
	}
	if err := ec.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to save external controller")
	}

	return &externalController{
		doc: *doc,
	}, nil
}

// SaveAndMoveModels is the same as `Save`, but if any of the input model UUIDs
// are in other external external controllers, those records will be updated
// to disassociate them.
func (ec *externalControllers) SaveAndMoveModels(controller crossmodel.ControllerInfo, modelUUIDs ...string) error {
	if err := controller.Validate(); err != nil {
		return errors.Trace(err)
	}
	doc := newExternalControllerDoc(controller)
	buildTxn := func(int) ([]txn.Op, error) {
		// Find any controllers that already have one of
		// the input model UUIDs associated with it.
		controllers, err := ec.st.externalControllerDocsForModels(modelUUIDs...)
		if err != nil {
			return nil, errors.Trace(err)
		}

		var ops []txn.Op
		changingModels := set.NewStrings(modelUUIDs...)
		for _, controller := range controllers {
			// If the controller to be saved already has one
			// of the input model UUIDs, do not change it.
			if controller.Id == doc.Id {
				continue
			}

			// If the models for the controller need changing,
			// generate a transaction operation.
			currentModels := set.NewStrings(controller.Models...)
			modelDiff := currentModels.Difference(changingModels)
			if modelDiff.Size() != currentModels.Size() {
				// TODO (manadart 2019-12-13): There is a case for deleting
				// records with no more associated model UUIDs.
				// We are being conservative here and keeping them.
				ops = append(ops, txn.Op{
					C:      externalControllersC,
					Id:     controller.Id,
					Assert: txn.DocExists,
					Update: bson.D{{"$set", bson.D{{"models", modelDiff.SortedValues()}}}},
				})
			}
		}

		newControllerOps, err := ec.upsertExternalControllerOps(doc, modelUUIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return append(ops, newControllerOps...), nil
	}
	return errors.Annotate(ec.st.db().Run(buildTxn), "saving location of external models")
}

// upsertExternalControllerOps returns the transaction operations for saving
// the input controller document as the location of the input models.
func (ec *externalControllers) upsertExternalControllerOps(
	doc *externalControllerDoc, modelUUIDs []string,
) ([]txn.Op, error) {
	model, err := ec.st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "failed to load model")
	}
	if err := checkModelActive(ec.st); err != nil {
		return nil, errors.Trace(err)
	}
	existing, err := ec.controller(doc.Id)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	upsertOp := upsertExternalControllerOp(doc, existing, modelUUIDs)
	ops := []txn.Op{
		upsertOp,
		model.assertActiveOp(),
	}
	return ops, nil
}

// Remove removes an external controller record with the given controller UUID.
func (ec *externalControllers) Remove(controllerUUID string) error {
	ops := []txn.Op{{
		C:      externalControllersC,
		Id:     controllerUUID,
		Remove: true,
	}}
	err := ec.st.db().RunTransaction(ops)
	return errors.Annotate(err, "failed to remove external controller")
}

// Controller retrieves an ExternalController with a given controller UUID.
func (ec *externalControllers) Controller(controllerUUID string) (ExternalController, error) {
	doc, err := ec.controller(controllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &externalController{*doc}, nil
}

func (ec *externalControllers) controller(controllerUUID string) (*externalControllerDoc, error) {
	coll, closer := ec.st.db().GetCollection(externalControllersC)
	defer closer()

	var doc externalControllerDoc
	err := coll.FindId(controllerUUID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("external controller with UUID %v", controllerUUID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// ControllerForModel retrieves an ExternalController with a given model UUID.
func (ec *externalControllers) ControllerForModel(modelUUID string) (ExternalController, error) {
	return ec.st.ExternalControllerForModel(modelUUID)
}

// Watch returns a strings watcher that watches for addition and removal of
// external controller documents. The strings returned will be the controller
// UUIDs.
func (ec *externalControllers) Watch() StringsWatcher {
	return newExternalControllersWatcher(ec.st)
}

// WatchController returns a notify watcher that watches for changes to the
// external controller with the specified controller UUID.
func (ec *externalControllers) WatchController(controllerUUID string) NotifyWatcher {
	return newEntityWatcher(ec.st, externalControllersC, controllerUUID)
}

// ExternalControllerForModel retrieves an ExternalController with a given
// model UUID.
// This is very similar to externalControllers.ControllerForModel, except the
// return type is a lot less strict, one that we can access the ModelUUIDs from
// the controller.
func (st *State) ExternalControllerForModel(modelUUID string) (*externalController, error) {
	docs, err := st.externalControllerDocsForModels(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch len(docs) {
	case 0:
		return nil, errors.NotFoundf("external controller with model %v", modelUUID)
	case 1:
		return &externalController{doc: docs[0]}, nil
	}
	return nil, errors.Errorf("expected 1 controller with model %v, got %d", modelUUID, len(docs))
}

func (st *State) externalControllerDocsForModels(modelUUIDs ...string) ([]externalControllerDoc, error) {
	coll, closer := st.db().GetCollection(externalControllersC)
	defer closer()

	var docs []externalControllerDoc
	err := coll.Find(bson.M{"models": bson.M{"$in": modelUUIDs}}).All(&docs)
	return docs, errors.Trace(err)
}

func upsertExternalControllerOp(doc, existing *externalControllerDoc, modelUUIDs []string) txn.Op {
	if existing != nil {
		models := set.NewStrings(existing.Models...)
		models = models.Union(set.NewStrings(modelUUIDs...))
		return txn.Op{
			C:      externalControllersC,
			Id:     existing.Id,
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set",
					bson.D{
						{"addresses", doc.Addrs},
						{"alias", doc.Alias},
						{"cacert", doc.CACert},
						{"models", models.SortedValues()},
					},
				},
			},
		}
	}

	doc.Models = modelUUIDs
	return txn.Op{
		C:      externalControllersC,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: *doc,
	}
}
