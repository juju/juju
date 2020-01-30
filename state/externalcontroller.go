// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
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

// Add creates or updates an external controller record.
func (ec *externalControllers) Save(controller crossmodel.ControllerInfo, modelUUIDs ...string) (ExternalController, error) {
	if err := controller.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	doc := externalControllerDoc{
		Id:     controller.ControllerTag.Id(),
		Alias:  controller.Alias,
		Addrs:  controller.Addrs,
		CACert: controller.CACert,
	}
	buildTxn := func(int) ([]txn.Op, error) {
		model, err := ec.st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if err := checkModelActive(ec.st); err != nil {
			return nil, errors.Trace(err)
		}
		existing, err := ec.controller(controller.ControllerTag.Id())
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if err == nil {
			models := set.NewStrings(existing.Models...)
			models = models.Union(set.NewStrings(modelUUIDs...))
			ops = []txn.Op{{
				C:      externalControllersC,
				Id:     existing.Id,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set",
						bson.D{{"addresses", doc.Addrs},
							{"alias", doc.Alias},
							{"cacert", doc.CACert},
							{"models", models.Values()}},
					},
				},
			}, model.assertActiveOp()}
		} else {
			doc.Models = modelUUIDs
			ops = []txn.Op{{
				C:      externalControllersC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}, model.assertActiveOp()}
		}
		return ops, nil
	}
	if err := ec.st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create external controllers")
	}

	return &externalController{
		doc: doc,
	}, nil
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
	coll, closer := ec.st.db().GetCollection(externalControllersC)
	defer closer()

	var doc []externalControllerDoc
	err := coll.Find(bson.M{"models": bson.M{"$in": []string{modelUUID}}}).All(&doc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch len(doc) {
	case 0:
		return nil, errors.NotFoundf("external controller with model %v", modelUUID)
	case 1:
		return &externalController{
			doc: doc[0],
		}, nil
	}
	return nil, errors.Errorf("expected 1 controller with model %v, got %d", modelUUID, len(doc))
}

// AllExternalControllers retrieves all ExternalController for a given model.
func (ec *externalControllers) AllExternalControllers() ([]externalControllerDoc, error) {
	coll, closer := ec.st.db().GetCollection(externalControllersC)
	defer closer()

	var docs []externalControllerDoc
	err := coll.Find(bson.M{}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
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
