// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	mgo "github.com/juju/mgo"
	"github.com/juju/mgo/bson"
	"github.com/juju/mgo/txn"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
)

type containerSpecDoc struct {
	Tag       string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`
	Spec      string `bson:"spec"`
}

// SetContainerSpec sets the container spec for the given entity tag.
//
// The entity must be either an application or a unit. If an application
// is specified, then the container spec is the default container spec
// for all units in the application. A unit-specific container spec may
// be set to override this default.
//
// An error will be returned if the specified entity is not alive.
func (m *CAASModel) SetContainerSpec(entity names.Tag, spec string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var prereqOps []txn.Op
		switch entity.(type) {
		case names.ApplicationTag:
			app, err := m.State().Application(entity.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if app.Life() != Alive {
				return nil, errors.Errorf("%s not alive", names.ReadableString(entity))
			}
			prereqOps = append(prereqOps, txn.Op{
				C:      applicationsC,
				Id:     app.doc.DocID,
				Assert: isAliveDoc,
			})
		case names.UnitTag:
			u, err := m.State().Unit(entity.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if u.Life() != Alive {
				return nil, errors.Errorf("%s not alive", names.ReadableString(entity))
			}
			prereqOps = append(prereqOps, txn.Op{
				C:      unitsC,
				Id:     u.doc.DocID,
				Assert: isAliveDoc,
			})
		default:
			return nil, errors.NotSupportedf(
				"setting container spec for %s entity",
				entity.Kind(),
			)
		}

		op := txn.Op{
			C:  containerSpecsC,
			Id: entity.String(),
		}
		existing, err := m.containerSpec(entity)
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

// ContainerSpec returns the container spec for the given entity tag.
//
// The entity must be either an application or a unit. If a unit is
// specified and there is a unit-specific container spec, it will
// be returned; otherwise the application default container spec
// will be returned, if there is one.
func (m *CAASModel) ContainerSpec(entity names.Tag) (string, error) {
	switch entity := entity.(type) {
	case names.UnitTag:
		spec, unitErr := m.containerSpec(entity)
		if !errors.IsNotFound(unitErr) {
			return spec, unitErr
		}
		// No unit-specific container spec found, so check
		// for an application container spec.
		appName, err := names.UnitApplication(entity.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
		spec, err = m.containerSpec(names.NewApplicationTag(appName))
		if err != nil {
			return "", unitErr
		}
		return spec, nil
	case names.ApplicationTag:
		return m.containerSpec(entity)
	default:
		return "", errors.NotSupportedf(
			"getting container spec for %s",
			names.ReadableString(entity),
		)
	}
}

func (m *CAASModel) containerSpec(entity names.Tag) (string, error) {
	coll, cleanup := m.mb.db().GetCollection(containerSpecsC)
	defer cleanup()
	var doc containerSpecDoc
	if err := coll.FindId(entity.String()).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return "", errors.NotFoundf(
				"container spec for %s",
				names.ReadableString(entity),
			)
		}
		return "", errors.Trace(err)
	}
	return doc.Spec, nil
}

func removeContainerSpecOp(entity names.Tag) txn.Op {
	return txn.Op{
		C:      containerSpecsC,
		Id:     entity.String(),
		Remove: true,
	}
}
