// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
)

// ServiceLocator represents the state of a juju network service locator.
type ServiceLocator struct {
	st  *State
	doc serviceLocatorDoc
}

type serviceLocatorDoc struct {
	DocId string `bson:"_id"`
	Id    string `bson:"service-locator-id"`
	Name  string `bson:"name"`
	Type  string `bson:"type"`
}

func newServiceLocator(st *State, doc *serviceLocatorDoc) *ServiceLocator {
	app := &ServiceLocator{
		st:  st,
		doc: *doc,
	}
	return app
}

// Name returns the name of the service locator.
func (sl *ServiceLocator) Name() string {
	return sl.doc.Name
}

// Type returns the type of the service locator.
func (sl *ServiceLocator) Type() string {
	return sl.doc.Type
}

// serviceLocators returns the service locators for the input condition
func (st *State) serviceLocators(condition bson.D) ([]*ServiceLocator, error) {
	serviceLocatorCollection, closer := st.db().GetCollection(serviceLocatorsC)
	defer closer()

	var connDocs []serviceLocatorDoc
	if err := serviceLocatorCollection.Find(condition).All(&connDocs); err != nil {
		return nil, errors.Trace(err)
	}

	conns := make([]*ServiceLocator, len(connDocs))
	for i, v := range connDocs {
		conns[i] = newServiceLocator(st, &v)
	}
	return conns, nil
}

// AllServiceLocators returns all service locators in the model.
func (st *State) AllServiceLocators() ([]*ServiceLocator, error) {
	conns, err := st.serviceLocators(nil)
	return conns, errors.Annotate(err, "getting service locators")
}
