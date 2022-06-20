// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
)

//// ServiceLocators describes the state functionality for service locators.
//type ServiceLocators interface {
//	// AllServiceLocators returns the list of all service locators.
//	//AllServiceLocators() ([]*ServiceLocator, error)
//}

// ServiceLocators returns the service locators functionality for the current state.
func (st *State) ServiceLocators() *serviceLocatorPersistence {
	return st.serviceLocators()
}

// serviceLocators returns the service locators functionality for the current state.
func (st *State) serviceLocators() *serviceLocatorPersistence {
	return &serviceLocatorPersistence{
		st: st,
	}
}

var slLogger = logger.Child("service-locator")

// serviceLocatorPersistence provides the persistence
// functionality for service locators.
type serviceLocatorPersistence struct {
	st *State
}

// ServiceLocator represents the state of a juju network service locator.
type ServiceLocator struct {
	st  *State
	doc serviceLocatorDoc
}

type serviceLocatorDoc struct {
	DocId  string                 `bson:"_id"`
	Id     string                 `bson:"service-locator-id"`
	Name   string                 `bson:"name"`
	Type   string                 `bson:"type"`
	Params map[string]interface{} `bson:"params"`
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

// AddServiceLocatorParams contains the parameters for adding an service locator
// to the model.
type AddServiceLocatorParams struct {
	// ServiceLocatorUUID is the UUID of the service locator.
	ServiceLocatorUUID string

	// Name is the name of the service locator.
	Name string

	// Type is the type of the service locator.
	Type string
}

func validateServiceLocatorParams(args AddServiceLocatorParams) (err error) {
	// No Sanity checks.
	return nil
}

// AddServiceLocator creates a new service locator record, which records details about a
// network service provided to related units.
func (sp *serviceLocatorPersistence) AddServiceLocator(args AddServiceLocatorParams) (_ *ServiceLocator, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service locator for %q", args.ServiceLocatorUUID)

	if err := validateServiceLocatorParams(args); err != nil {
		return nil, errors.Trace(err)
	}

	model, err := sp.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	// Create the application addition operations.
	serviceLocatorDoc := serviceLocatorDoc{
		Id:    args.ServiceLocatorUUID,
		Name:  args.Name,
		Type:  args.Type,
		DocId: args.ServiceLocatorUUID,
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(sp.st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.AlreadyExistsf("service locator Name: %s ID: %s", args.Name, args.ServiceLocatorUUID)
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      serviceLocatorsC,
				Id:     serviceLocatorDoc.DocId,
				Assert: txn.DocMissing,
				Insert: &serviceLocatorDoc,
			},
		}
		return ops, nil
	}
	if err = sp.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return &ServiceLocator{doc: serviceLocatorDoc}, nil
}

// RemoveServiceLocator removes a service locator record
func RemoveServiceLocator(slId string) []txn.Op {
	op := txn.Op{
		C:      serviceLocatorsC,
		Id:     slId,
		Remove: true,
	}
	return []txn.Op{op}
}

// AllServiceLocators returns all service locators in the model.
func (sp *serviceLocatorPersistence) AllServiceLocators() ([]*ServiceLocator, error) {
	locators, err := sp.serviceLocators(nil)
	return locators, errors.Annotate(err, "getting service locators")
}

// ServiceLocator returns the service locator.
func (sp *serviceLocatorPersistence) ServiceLocator(ServiceLocatorUUID string) ([]*ServiceLocator, error) {
	locators, err := sp.serviceLocators(bson.D{{"service-locator-uuid", ServiceLocatorUUID}})
	return locators, errors.Annotatef(err, "getting service locators for %v", ServiceLocatorUUID)
}

// serviceLocators returns the service locators for the input condition
func (sp *serviceLocatorPersistence) serviceLocators(condition bson.D) ([]*ServiceLocator, error) {
	serviceLocatorCollection, closer := sp.st.db().GetCollection(serviceLocatorsC)
	defer closer()

	var locatorDocs []serviceLocatorDoc
	if err := serviceLocatorCollection.Find(condition).All(&locatorDocs); err != nil {
		return nil, errors.Trace(err)
	}

	locators := make([]*ServiceLocator, len(locatorDocs))
	for i, v := range locatorDocs {
		locators[i] = newServiceLocator(sp.st, &v)
	}
	return locators, nil
}
