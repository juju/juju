// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
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
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
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
func (s *DomainServicesSuite) ControllerDomainServices(c *tc.C) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(s.ControllerModelUUID)
}

// DefaultModelDomainServices conveniently constructs a domain services for the
// default model.
func (s *DomainServicesSuite) DefaultModelDomainServices(c *tc.C) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(s.DefaultModelUUID)
}

// ModelDomainServices conveniently constructs a domain services for the
// default model.
func (s *DomainServicesSuite) ModelDomainServices(c *tc.C, modelUUID model.UUID) services.DomainServices {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})(modelUUID)
}

// ModelDomainServicesGetter conveniently constructs a domain services getter
// for the default model.
func (s *DomainServicesSuite) ModelDomainServicesGetter(c *tc.C) services.DomainServicesGetter {
	return s.DomainServicesGetter(c, TestingObjectStore{}, TestingLeaseManager{})
}

func (s *DomainServicesSuite) SeedControllerConfig(c *tc.C) {
	fn := controllerconfigbootstrap.InsertInitialControllerConfig(
		s.ControllerConfig,
		s.ControllerModelUUID,
	)
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DomainServicesSuite) SeedAdminUser(c *tc.C) {
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
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *DomainServicesSuite) SeedCloudAndCredential(c *tc.C) {
	ctx := c.Context()

	err := cloudstate.AllowCloudType(ctx, s.ControllerTxnRunner(), 99, "dummy")
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)
}

// SeedModelDatabases makes sure that model's for both the controller and default
// model have been created in the database.
func (s *DomainServicesSuite) SeedModelDatabases(c *tc.C) {
	ctx := c.Context()

	controllerUUID, err := uuid.UUIDFromString(jujutesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	controllerArgs := modeldomain.GlobalModelCreationArgs{
		Cloud:       s.CloudName,
		CloudRegion: "dummy-region",
		Credential:  s.CredentialKey,
		Name:        model.ControllerModelName,
		Qualifier:   "prod",
		AdminUsers:  []coreuser.UUID{s.AdminUserUUID},
	}

	fn := modelbootstrap.CreateGlobalModelRecord(s.ControllerModelUUID, controllerArgs)
	c.Assert(backendbootstrap.CreateDefaultBackends(model.IAAS)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String())), tc.ErrorIsNil)
	err = fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(s.ControllerModelUUID, controllerUUID, jujuversion.Current)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	fn = modelconfigbootstrap.SetModelConfig(
		s.ControllerModelUUID,
		nil,
		modeldefaultsbootstrap.ModelDefaultsProvider(nil, nil, s.CloudType),
	)
	err = fn(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.ControllerModelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	modelArgs := modeldomain.GlobalModelCreationArgs{
		Cloud:      s.CloudName,
		Credential: s.CredentialKey,
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.AdminUserUUID},
	}

	fn = modelbootstrap.CreateGlobalModelRecord(s.DefaultModelUUID, modelArgs)
	err = fn(ctx, s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(s.DefaultModelUUID, controllerUUID, jujuversion.Current)(
		ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.DefaultModelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	fn = modelconfigbootstrap.SetModelConfig(
		s.DefaultModelUUID,
		nil,
		modeldefaultsbootstrap.ModelDefaultsProvider(nil, nil, s.CloudType),
	)
	err = fn(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.DefaultModelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
}

// DomainServicesGetter provides an implementation of the DomainServicesGetter
// interface to use in tests. This includes the dummy storage registry.
func (s *DomainServicesSuite) DomainServicesGetter(c *tc.C, objectStore objectstore.ObjectStore, leaseManager lease.LeaseManager) DomainServicesGetterFunc {
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
func (s *DomainServicesSuite) DomainServicesGetterWithStorageRegistry(c *tc.C, objectStore objectstore.ObjectStore, leaseManager lease.LeaseManager, storageRegistry storage.ProviderRegistry) DomainServicesGetterFunc {
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
			modelObjectStoreGetter(func(ctx context.Context) (objectstore.ObjectStore, error) {
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
			modelApplicationLeaseManagerGetter(func() lease.LeaseManager {
				return leaseManager
			}),
			c.MkDir(),
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
func (s *DomainServicesSuite) ObjectStoreServicesGetter(c *tc.C) ObjectStoreServicesGetterFunc {
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
func (s *DomainServicesSuite) NoopObjectStore(c *tc.C) objectstore.ObjectStore {
	return TestingObjectStore{}
}

// NoopLeaseManager returns a no-op implementation of lease.LeaseManager.
func (s *DomainServicesSuite) NoopLeaseManager(c *tc.C) lease.LeaseManager {
	return TestingLeaseManager{}
}

// SetUpTest creates the controller and default model unique identifiers if they
// have not already been set. Also seeds the initial database with the models.
func (s *DomainServicesSuite) SetUpTest(c *tc.C) {
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

type modelStorageRegistryGetter func(context.Context) (storage.ProviderRegistry, error)

func (s modelStorageRegistryGetter) GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error) {
	return s(ctx)
}

type modelApplicationLeaseManagerGetter func() lease.LeaseManager

func (s modelApplicationLeaseManagerGetter) GetLeaseManager() (lease.LeaseManager, error) {
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

// Revoke releases the named lease for the named holder.
func (TestingLeaseManager) Revoke(leaseName, holderName string) error {
	return nil
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
	return stubProvider{}, nil
}

// EphemeralProviderFromConfig returns an ephemeral provider for a given
// configuration. The provider is not tracked, instead is created and then
// discarded.
func (stubProviderFactory) EphemeralProviderFromConfig(ctx context.Context, config providertracker.EphemeralProviderConfig) (providertracker.Provider, error) {
	return nil, errors.New("suite missing provider factory").Add(coreerrors.NotSupported)
}

type stubProvider struct{}

// AdoptResources implements providertracker.Provider.
func (s stubProvider) AdoptResources(ctx context.Context, controllerUUID string, fromVersion semversion.Number) error {
	return nil
}

func (stubProvider) PrecheckInstance(ctx context.Context, params environs.PrecheckInstanceParams) error {
	return nil
}

func (stubProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

// AdoptResources is a stub implementation to satisfy the providertracker.Provider interface.
func (stubProvider) InstanceTypes(context.Context, constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, nil
}

// Bootstrap implements providertracker.Provider.
func (s stubProvider) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return nil, nil
}

// Config implements providertracker.Provider.
func (s stubProvider) Config() *config.Config {
	return nil
}

// Destroy implements providertracker.Provider.
func (s stubProvider) Destroy(ctx context.Context) error {
	return nil
}

// DestroyController implements providertracker.Provider.
func (s stubProvider) DestroyController(ctx context.Context, controllerUUID string) error {
	return nil
}

// PrepareForBootstrap implements providertracker.Provider.
func (s stubProvider) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	return nil
}

// SetConfig implements providertracker.Provider.
func (s stubProvider) SetConfig(ctx context.Context, cfg *config.Config) error {
	return nil
}

// StorageProvider implements providertracker.Provider.
func (s stubProvider) StorageProvider(storage.ProviderType) (storage.Provider, error) {
	return nil, nil
}

// StorageProviderTypes implements providertracker.Provider.
func (s stubProvider) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}
