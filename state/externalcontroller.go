// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
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
		Addrs:         rc.doc.Addrs,
		CACert:        rc.doc.CACert,
	}
}

// ExternalControllers instances provide access to external controllers in state.
type ExternalControllers interface {
	Save(_ crossmodel.ControllerInfo, modelUUIDs ...string) (ExternalController, error)
	ControllerForModel(modelUUID string) (ExternalController, error)
}

type externalControllers struct {
	st *State
}

// NewExternalControllers creates an external controllers instance backed by a state.
func NewExternalControllers(st *State) *externalControllers {
	return &externalControllers{st: st}
}

// Add creates a new external controller record.
func (ec *externalControllers) Save(controller crossmodel.ControllerInfo, modelUUIDs ...string) (ExternalController, error) {
	if err := controller.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	doc := externalControllerDoc{
		Id:     controller.ControllerTag.Id(),
		Addrs:  controller.Addrs,
		CACert: controller.CACert,
	}
	buildTxn := func(int) ([]txn.Op, error) {
		model, err := ec.st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if model.Life() != Alive {
			return nil, errors.New("model is not alive")
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
	if err := ec.st.run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create external controllers")
	}

	return &externalController{
		doc: doc,
	}, nil
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
