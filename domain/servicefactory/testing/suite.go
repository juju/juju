// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/status"
	coreuser "github.com/juju/juju/core/user"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/internal/auth"
	databasetesting "github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/servicefactory"
	jujutesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

// ServiceFactorySuite is a test suite that can be composed into tests that
// require a Juju ServiceFactory and database access. It holds the notion of a
// controller model uuid and that of a default model uuid. Both of these models
// will be instantiated into the database upon test setup.
type ServiceFactorySuite struct {
	schematesting.ControllerModelSuite

	// AdminUserUUID is the uuid of the admin user made during the setup of this
	// test suite.
	AdminUserUUID coreuser.UUID

	// CloudName is the name of the cloud made during the setup of this suite.
	CloudName string

	CredentialKey credential.Key

	// ControllerModelUUID is the unique id for the controller model. If not set
	// will be set during test set up.
	ControllerModelUUID coremodel.UUID

	// DefaultModelUUID is the unique id for the default model. If not set
	// will be set during test set up.
	DefaultModelUUID coremodel.UUID

	// ProviderTracker is the provider tracker to use in the service factory.
	ProviderTracker providertracker.ProviderFactory

	// StatusHistoryFactory is the status history factory to use in the service
	// factory.
	StatusHistoryFactory status.StatusHistoryFactory
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

func (s *ServiceFactorySuite) SeedAdminUser(c *gc.C) {
	password := auth.NewPassword("dummy-secret")
	uuid, fn := userbootstrap.AddUserWithPassword(
		coreuser.AdminUserName,
		password,
		permission.ControllerForAccess(permission.SuperuserAccess),
	)
	s.AdminUserUUID = uuid
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceFactorySuite) SeedCloudAndCredential(c *gc.C) {
	ctx := context.Background()

	err := cloudstate.AllowCloudType(ctx, s.ControllerTxnRunner(), 99, "dummy")
	c.Assert(err, jc.ErrorIsNil)

	s.CloudName = "dummy"
	err = cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.CloudName,
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Regions: []cloud.Region{
			{
				Name: "dummy-region",
			},
		},
	})(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	s.CredentialKey = credential.Key{
		Cloud: s.CloudName,
		Name:  "default",
		Owner: coreuser.AdminUserName,
	}
	err = credentialbootstrap.InsertCredential(
		s.CredentialKey,
		cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
			"username": "dummy",
			"password": "secret",
		}),
	)(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

// SeedModelDatabases makes sure that model's for both the controller and default
// model have been created in the database.
func (s *ServiceFactorySuite) SeedModelDatabases(c *gc.C) {
	ctx := context.Background()

	controllerUUID := coremodel.UUID(jujutesting.ControllerTag.Id())

	controllerArgs := modeldomain.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        s.CloudName,
		Credential:   s.CredentialKey,
		Name:         coremodel.ControllerModelName,
		Owner:        s.AdminUserUUID,
		UUID:         s.ControllerModelUUID,
	}

	uuid, fn := modelbootstrap.CreateModel(controllerArgs)
	err := fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	s.ControllerModelUUID = uuid

	err = modelbootstrap.CreateReadOnlyModel(
		controllerArgs, controllerUUID, coreuser.AdminUserName,
	)(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, uuid.String()))
	c.Assert(err, jc.ErrorIsNil)

	modelArgs := modeldomain.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        s.CloudName,
		Credential:   s.CredentialKey,
		Name:         "test",
		Owner:        s.AdminUserUUID,
		UUID:         s.DefaultModelUUID,
	}

	uuid, fn = modelbootstrap.CreateModel(modelArgs)
	err = fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	s.DefaultModelUUID = uuid

	err = modelbootstrap.CreateReadOnlyModel(
		modelArgs, controllerUUID, coreuser.AdminUserName,
	)(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, uuid.String()))
	c.Assert(err, jc.ErrorIsNil)
}

// ServiceFactoryGetter provides an implementation of the ServiceFactoryGetter
// interface to use in tests.
func (s *ServiceFactorySuite) ServiceFactoryGetter(c *gc.C) ServiceFactoryGetterFunc {
	return func(modelUUID string) servicefactory.ServiceFactory {
		return domainservicefactory.NewServiceFactory(
			databasetesting.ConstFactory(s.TxnRunner()),
			coremodel.UUID(modelUUID),
			databasetesting.ConstFactory(s.ModelTxnRunner(c, modelUUID)),
			stubDBDeleter{DB: s.DB()},
			s.ProviderTracker,
			s.StatusHistoryFactory,
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
	s.SeedAdminUser(c)
	s.SeedCloudAndCredential(c)
	s.SeedModelDatabases(c)
}

// ServiceFactoryGetterFunc is a convenience type for translating a getter
// function into the ServiceFactoryGetter interface.
type ServiceFactoryGetterFunc func(string) servicefactory.ServiceFactory

// FactoryForModel implements the ServiceFactoryGetter interface.
func (s ServiceFactoryGetterFunc) FactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return s(modelUUID)
}
