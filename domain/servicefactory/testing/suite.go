// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain/modelmanager/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	databasetesting "github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/servicefactory"
)

// ServiceFactorySuite is a test suite that can be composed into tests that
// require a Juju ServiceFactory and database access. It holds the notion of a
// controller model uuid and that of a default model uuid. Both of these models
// will be instantiated into the database upon test setup.
type ServiceFactorySuite struct {
	schematesting.ControllerModelSuite

	// ControllerModelUUID is the unique id for the controller model. If not set
	// will be set during test set up.
	ControllerModelUUID coremodel.UUID

	// DefaultModelUUID is the unique id for the default model. If not set
	// will be set during test set up.
	DefaultModelUUID coremodel.UUID
}

type stubDBDeleter struct {
	DB *sql.DB
}

func (s stubDBDeleter) DeleteDB(namespace string) error {
	return nil
}

// ControllerServiceFactory conveniently constructs a service factory for the
// controller model.
func (s *ServiceFactorySuite) ControllerServiceFactory(c *gc.C) servicefactory.ServiceFactory {
	return s.ServiceFactoryGetter(c)(string(s.ControllerModelUUID))
}

// DefaultModelServiceFactory conveniently constructs a service factory for the
// default model.
func (s *ServiceFactorySuite) DefaultModelServiceFactory(c *gc.C) servicefactory.ServiceFactory {
	return s.ServiceFactoryGetter(c)(string(s.ControllerModelUUID))
}

// SeedModelDatabases makes sure that model's for both the controller and default
// model have been created in the database.
func (s *ServiceFactorySuite) SeedModelDatabases(c *gc.C) {
	err := bootstrap.RegisterModel(s.ControllerModelUUID)(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.RegisterModel(s.DefaultModelUUID)(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

// ServiceFactoryGetter provides an implementation of the ServiceFactoryGetter
// interface to use in tests.
func (s *ServiceFactorySuite) ServiceFactoryGetter(c *gc.C) ServiceFactoryGetterFunc {
	return func(modelUUID string) servicefactory.ServiceFactory {
		return domainservicefactory.NewServiceFactory(
			databasetesting.ConstFactory(s.TxnRunner()),
			databasetesting.ConstFactory(s.ModelTxnRunner(c, modelUUID)),
			stubDBDeleter{DB: s.DB()},
			NewCheckLogger(c),
		)
	}
}

// SetUpTest creates the controller and default model unique identifiers if they
// have not already been set. Also seeds the initial database with the models.
func (s *ServiceFactorySuite) SetUpTest(c *gc.C) {
	s.ControllerModelSuite.SetUpTest(c)
	if s.ControllerModelUUID == "" {
		s.ControllerModelUUID = modeltesting.GenModelUUID(c)
	}
	if s.DefaultModelUUID == "" {
		s.DefaultModelUUID = modeltesting.GenModelUUID(c)
	}
	s.SeedModelDatabases(c)
}

// ServiceFactoryGetterFunc is a convenience type for translating a getter
// function into the ServiceFactoryGetter interface.
type ServiceFactoryGetterFunc func(string) servicefactory.ServiceFactory

// FactoryForModel implements the ServiceFactoryGetter interface.
func (s ServiceFactoryGetterFunc) FactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return s(modelUUID)
}
