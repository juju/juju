// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/providertracker"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	controllerconfigbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	modelconfigbootstrap "github.com/juju/juju/domain/modelconfig/bootstrap"
	modeldefaultsbootstrap "github.com/juju/juju/domain/modeldefaults/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	backendbootstrap "github.com/juju/juju/domain/secretbackend/bootstrap"
	domainservices "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/auth"
	databasetesting "github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/services"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/storage/provider/dummy"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

// DomainServicesSuite is a test suite that can be composed into tests that
// require a Juju DomainServices and database access. It holds the notion of a
// controller model uuid and that of a default model uuid. Both of these models
// will be instantiated into the database upon test setup.
type DomainServicesSuite struct {
	schematesting.ControllerModelSuite

	// AdminUserUUID is the uuid of the admin user made during the setup of this
	// test suite.
	AdminUserUUID coreuser.UUID

	// CloudName is the name of the cloud made during the setup of this suite.
	CloudName string

	// CloudType represents the type of the cloud made during the setup of this
	// suite.
	CloudType string

	CredentialKey credential.Key

	// ControllerModelUUID is the unique id for the controller model. If not set
	// will be set during test set up.
	ControllerModelUUID model.UUID

	// ControllerConfig is the controller configuration, including its UUID. If
	// not set will be set to the default testing value during test set up.
	ControllerConfig controller.Config

	// DefaultModelUUID is the unique id for the default model. If not set
	// will be set during test set up.
	DefaultModelUUID model.UUID

	// ProviderFactory is the provider tracker factory to use in the domain
	// services.
	ProviderFactory providertracker.ProviderFactory
}

type stubDBDeleter struct{}

func (s stubDBDeleter) DeleteDB(namespace string) error {
	return nil
}

// ControllerDomainServices conveniently constructs a domain services for the
// controller model.
func (s *DomainServicesSuite) ControllerDomainServices(c *gc.C) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(s.ControllerModelUUID)
}

// DefaultModelDomainServices conveniently constructs a domain services for the
// default model.
func (s *DomainServicesSuite) DefaultModelDomainServices(c *gc.C) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(s.ControllerModelUUID)
}

// ModelDomainServices conveniently constructs a domain services for the
// default model.
func (s *DomainServicesSuite) ModelDomainServices(c *gc.C, modelUUID model.UUID) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(modelUUID)
}

// ModelDomainServicesGetter conveniently constructs a domain services getter
// for the default model.
func (s *DomainServicesSuite) ModelDomainServicesGetter(c *gc.C) services.DomainServicesGetter {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})
}

func (s *DomainServicesSuite) SeedControllerConfig(c *gc.C) {
	fn := controllerconfigbootstrap.InsertInitialControllerConfig(
		s.ControllerConfig,
		s.ControllerModelUUID,
	)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DomainServicesSuite) SeedAdminUser(c *gc.C) {
	password := auth.NewPassword("dummy-secret")
	uuid, fn := userbootstrap.AddUserWithPassword(
		coreuser.AdminUserName,
		password,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        jujutesting.ControllerTag.Id(),
			},
		},
	)
	s.AdminUserUUID = uuid
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DomainServicesSuite) SeedCloudAndCredential(c *gc.C) {
	ctx := context.Background()

	err := cloudstate.AllowCloudType(ctx, s.ControllerTxnRunner(), 99, "dummy")
	c.Assert(err, jc.ErrorIsNil)

	s.CloudName = "dummy"
	s.CloudType = "dummy"
	err = cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.CloudName,
		Type:      s.CloudType,
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
func (s *DomainServicesSuite) SeedModelDatabases(c *gc.C) {
	ctx := context.Background()

	controllerUUID, err := uuid.UUIDFromString(jujutesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	controllerArgs := modeldomain.GlobalModelCreationArgs{
		Cloud:       s.CloudName,
		CloudRegion: "dummy-region",
		Credential:  s.CredentialKey,
		Name:        model.ControllerModelName,
		Owner:       s.AdminUserUUID,
	}

	fn := modelbootstrap.CreateGlobalModelRecord(s.ControllerModelUUID, controllerArgs)
	c.Assert(backendbootstrap.CreateDefaultBackends(model.IAAS)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String())), jc.ErrorIsNil)
	err = fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(s.ControllerModelUUID, controllerUUID, jujuversion.Current)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String()))
	c.Assert(err, jc.ErrorIsNil)

	fn = modelconfigbootstrap.SetModelConfig(
		s.ControllerModelUUID,
		nil,
		modeldefaultsbootstrap.ModelDefaultsProvider(nil, nil, s.CloudType),
	)
	err = fn(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String()))
	c.Assert(err, jc.ErrorIsNil)

	modelArgs := modeldomain.GlobalModelCreationArgs{
		Cloud:      s.CloudName,
		Credential: s.CredentialKey,
		Name:       "test",
		Owner:      s.AdminUserUUID,
	}

	fn = modelbootstrap.CreateGlobalModelRecord(s.DefaultModelUUID, modelArgs)
	err = fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(s.DefaultModelUUID, controllerUUID, jujuversion.Current)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.DefaultModelUUID.String()))
	c.Assert(err, jc.ErrorIsNil)

	fn = modelconfigbootstrap.SetModelConfig(
		s.DefaultModelUUID,
		nil,
		modeldefaultsbootstrap.ModelDefaultsProvider(nil, nil, s.CloudType),
	)
	err = fn(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.DefaultModelUUID.String()))
	c.Assert(err, jc.ErrorIsNil)
}

// DomainServicesGetter provides an implementation of the DomainServicesGetter
// interface to use in tests. This includes the dummy storage registry.
func (s *DomainServicesSuite) DomainServicesGetter(c *gc.C, objectStore objectstore.ObjectStore, leaseManager lease.Checker) DomainServicesGetterFunc {
	return s.DomainServicesGetterWithStorageRegistry(c, objectStore, leaseManager, storage.ChainedProviderRegistry{
		// Using the dummy storage provider for testing purposes isn't
		// ideal. We should potentially use a mock storage provider
		// instead.
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	})
}

type domainServices struct {
	*domainservices.ControllerServices
	*domainservices.ModelServices
}

// DomainServicesGetterWithStorageRegistry provides an implementation of the
// DomainServicesGetterWithStorageRegistry interface to use in tests with the
// additional storage provider.
func (s *DomainServicesSuite) DomainServicesGetterWithStorageRegistry(c *gc.C, objectStore objectstore.ObjectStore, leaseManager lease.Checker, storageRegistry storage.ProviderRegistry) DomainServicesGetterFunc {
	return func(modelUUID model.UUID) services.DomainServices {
		clock := clock.WallClock
		logger := loggertesting.WrapCheckLog(c)
		providerFactory := s.ProviderFactory
		if providerFactory == nil {
			providerFactory = &stubProviderFactory{}
		}
		controllerServices := domainservices.NewControllerServices(
			databasetesting.ConstFactory(s.TxnRunner()),
			stubDBDeleter{},
			controllerObjectStoreGetter(func(ctx context.Context) (objectstore.ObjectStore, error) {
				return objectStore, nil
			}),
			clock,
			logger,
		)
		modelServices := domainservices.NewModelServices(
			modelUUID,
			databasetesting.ConstFactory(s.TxnRunner()),
			databasetesting.ConstFactory(s.ModelTxnRunner(c, modelUUID.String())),
			providerFactory,
			modelObjectStoreGetter(func(ctx context.Context) (objectstore.ObjectStore, error) {
				return objectStore, nil
			}),
			modelStorageRegistryGetter(func(ctx context.Context) (storage.ProviderRegistry, error) {
				return storageRegistry, nil
			}),
			sshimporter.NewImporter(&http.Client{}),
			modelApplicationLeaseManagerGetter(func() lease.Checker {
				return leaseManager
			}),
			clock,
			logger,
		)
		return &domainServices{
			ControllerServices: controllerServices,
			ModelServices:      modelServices,
		}
	}
}

// ObjectStoreServicesGetter provides an implementation of the
// ObjectStoreServicesGetter interface to use in tests.
func (s *DomainServicesSuite) ObjectStoreServicesGetter(c *gc.C) ObjectStoreServicesGetterFunc {
	return func(modelUUID model.UUID) services.ObjectStoreServices {
		return domainservices.NewObjectStoreServices(
			databasetesting.ConstFactory(s.TxnRunner()),
			databasetesting.ConstFactory(s.ModelTxnRunner(c, modelUUID.String())),
			loggertesting.WrapCheckLog(c),
		)
	}
}

// NoopObjectStore returns a no-op implementation of the ObjectStore interface.
// This is useful when the test does not require any object store functionality.
func (s *DomainServicesSuite) NoopObjectStore(c *gc.C) objectstore.ObjectStore {
	return TestingObjectStore{}
}

// NoopLeaseManager returns a no-op implementation of lease.Checker.
func (s *DomainServicesSuite) NoopLeaseManager(c *gc.C) lease.Checker {
	return TestingLeaseManager{}
}

// SetUpTest creates the controller and default model unique identifiers if they
// have not already been set. Also seeds the initial database with the models.
func (s *DomainServicesSuite) SetUpTest(c *gc.C) {
	s.ControllerModelSuite.SetUpTest(c)
	if s.ControllerModelUUID == "" {
		s.ControllerModelUUID = modeltesting.GenModelUUID(c)
	}
	if s.ControllerConfig == nil {
		s.ControllerConfig = jujutesting.FakeControllerConfig()
	}
	if s.DefaultModelUUID == "" {
		s.DefaultModelUUID = modeltesting.GenModelUUID(c)
	}
	s.SeedControllerConfig(c)
	s.SeedAdminUser(c)
	s.SeedCloudAndCredential(c)
	s.SeedModelDatabases(c)
}

// DomainServicesGetterFunc is a convenience type for translating a getter
// function into the DomainServicesGetter interface.
type DomainServicesGetterFunc func(model.UUID) services.DomainServices

// ServicesForModel implements the DomainServicesGetter interface.
func (s DomainServicesGetterFunc) ServicesForModel(ctx context.Context, modelUUID model.UUID) (services.DomainServices, error) {
	return s(modelUUID), nil
}

// ObjectStoreServicesGetterFunc is a convenience type for translating a getter
// function into the ObjectStoreServicesGetter interface.
type ObjectStoreServicesGetterFunc func(model.UUID) services.ObjectStoreServices

// ServicesForModel implements the ObjectStoreServicesGetter interface.
func (s ObjectStoreServicesGetterFunc) ServicesForModel(modelUUID model.UUID) services.ObjectStoreServices {
	return s(modelUUID)
}

type modelObjectStoreGetter func(context.Context) (objectstore.ObjectStore, error)

func (s modelObjectStoreGetter) GetObjectStore(ctx context.Context) (objectstore.ObjectStore, error) {
	return s(ctx)
}

type controllerObjectStoreGetter func(context.Context) (objectstore.ObjectStore, error)

func (s controllerObjectStoreGetter) GetControllerObjectStore(ctx context.Context) (objectstore.ObjectStore, error) {
	return s(ctx)
}

type modelStorageRegistryGetter func(context.Context) (storage.ProviderRegistry, error)

func (s modelStorageRegistryGetter) GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error) {
	return s(ctx)
}

type modelApplicationLeaseManagerGetter func() lease.Checker

func (s modelApplicationLeaseManagerGetter) GetLeaseManager() (lease.Checker, error) {
	return s(), nil
}

// TestingObjectStore is a testing implementation of the ObjectStore interface.
type TestingObjectStore struct{}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (TestingObjectStore) Get(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.Errorf(name+" %w", coreerrors.NotFound)
}

// GetBySHA256 returns an io.ReadCloser for data at path, namespaced to the
// model.
func (TestingObjectStore) GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.Errorf(sha256+" %w", coreerrors.NotFound)
}

// GetBySHA256Prefix returns an io.ReadCloser for data at path, namespaced to the
// model.
func (TestingObjectStore) GetBySHA256Prefix(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	return nil, 0, errors.Errorf(sha256+" %w", coreerrors.NotFound)
}

// Put stores data from reader at path, namespaced to the model.
func (TestingObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) (objectstore.UUID, error) {
	return "", nil
}

// PutAndCheckHash stores data from reader at path, namespaced to the model.
// It also ensures the stored data has the correct hash.
func (TestingObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) (objectstore.UUID, error) {
	return "", nil
}

// Remove removes data at path, namespaced to the model.
func (TestingObjectStore) Remove(ctx context.Context, path string) error {
	return nil
}

// TestingLeaseManager is a testing implementation of the lease.Checker
// interface. It returns canned responses for the methods.
type TestingLeaseManager struct{}

// WaitUntilExpired returns nil when the named lease is no longer held. If
// it returns any error, no reasonable inferences may be made. The supplied
// context can be used to cancel the request; in this case, the method will
// return ErrWaitCancelled.
// The started channel when non-nil is closed when the wait begins.
func (TestingLeaseManager) WaitUntilExpired(ctx context.Context, leaseName string, started chan<- struct{}) error {
	close(started)

	return nil
}

// Token returns a Token that can be interrogated at any time to discover
// whether the supplied lease is currently held by the supplied holder.
func (TestingLeaseManager) Token(leaseName, holderName string) lease.Token {
	return TestingLeaseManagerToken{}
}

// TestingLeaseManagerToken is a testing implementation of the Token interface.
type TestingLeaseManagerToken struct{}

// Check will always return lease.ErrNotHeld.
func (TestingLeaseManagerToken) Check() error {
	return lease.ErrNotHeld
}

// stubProviderFactory is a testing implementation of the ProviderFactory
// interface, when none is provided to the suite.
type stubProviderFactory struct{}

// ProviderForModel returns the encapsulated provider for a given model
// namespace. It will continue to be updated in the background for as long
// as the Worker continues to run. If the worker is not a singular worker,
// then an error will be returned.
func (stubProviderFactory) ProviderForModel(ctx context.Context, namespace string) (providertracker.Provider, error) {
	return nil, errors.New("suite missing provider factory").Add(coreerrors.NotSupported)
}

// EphemeralProviderFromConfig returns an ephemeral provider for a given
// configuration. The provider is not tracked, instead is created and then
// discarded.
func (stubProviderFactory) EphemeralProviderFromConfig(ctx context.Context, config providertracker.EphemeralProviderConfig) (providertracker.Provider, error) {
	return nil, errors.New("suite missing provider factory").Add(coreerrors.NotSupported)
}
