// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/apiserver/stateauthenticator"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	controllerconfigbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/environs"
	environsconfig "github.com/juju/juju/environs/config"
	databasetesting "github.com/juju/juju/internal/database/testing"
	internallease "github.com/juju/juju/internal/lease"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/mongotest"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
	"github.com/juju/juju/internal/password"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/pubsub/centralhub"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/lease"
	wmultiwatcher "github.com/juju/juju/internal/worker/multiwatcher"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

const AdminSecret = "dummy-secret"

var (
	// AdminUser is the default admin user for a controller.
	AdminUser = names.NewUserTag("admin")

	// DefaultCloudRegion is the default cloud region for a controller model.
	DefaultCloudRegion = "dummy-region"

	// DefaultCloud is the default cloud for a controller model.
	DefaultCloud = cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: DefaultCloudRegion}},
	}

	// DefaultCredentialTag is the default credential for all models.
	DefaultCredentialTag = names.NewCloudCredentialTag("dummy/admin/default")

	// DefaultCredentialId is the default credential id for all models.
	DefaultCredentialId = corecredential.KeyFromTag(DefaultCredentialTag)
)

// ApiServerSuite is a text fixture which spins up an apiserver on top of a controller model.
type ApiServerSuite struct {
	servicefactorytesting.ServiceFactorySuite

	// MgoSuite is needed until we finally can
	// represent the model fully in dqlite.
	mgotesting.MgoSuite

	apiInfo    api.Info
	controller *state.Controller

	// apiConns are opened api.Connections to close on teardown
	apiConns []api.Connection

	baseURL    *url.URL
	httpServer *httptest.Server
	mux        *apiserverhttp.Mux

	// ControllerConfigAttrs can be set up before SetUpTest
	// is invoked. Any attributes set here will be added to
	// the suite's controller configuration.
	ControllerConfigAttrs map[string]interface{}

	// ControllerModelConfigAttrs can be set up before SetUpTest
	// is invoked. Any attributes set here will be added to
	// the suite's controller model configuration.
	ControllerModelConfigAttrs map[string]interface{}

	// These are exposed for the tests to use.
	Server            *apiserver.Server
	LeaseManager      *lease.Manager
	ObjectStoreGetter objectstore.ObjectStoreGetter
	Clock             testclock.AdvanceableClock

	// These attributes are set before SetUpTest to indicate we want to
	// set up the api server with real components instead of stubs.

	WithLeaseManager        bool
	WithMultiWatcher        bool
	WithControllerModelType state.ModelType
	WithEmbeddedCLICommand  func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusArgs string) int

	// These can be set prior to login being called.

	WithUpgrading      bool
	WithAuditLogConfig *auditlog.Config
	WithIntrospection  func(func(string, http.Handler))

	// AdminUserUUID is the root user for the controller.
	AdminUserUUID coreuser.UUID

	// InstancePrechecker is used to validate instance creation.
	// DEPRECATED: This will be removed in the future.
	InstancePrechecker func(*gc.C, *state.State) environs.InstancePrechecker

	// ConfigSchemaSourceGetter is used to provide the config schema for the model.
	// DEPRECATED: This will be removed in the future.
	ConfigSchemaSourceGetter func(*gc.C) environsconfig.ConfigSchemaSourceGetter
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}

func leaseManager(controllerUUID string, db database.DBGetter, clock clock.Clock) (*lease.Manager, error) {
	logger := loggo.GetLogger("juju.worker.lease.test")
	return lease.NewManager(lease.ManagerConfig{
		SecretaryFinder:      internallease.NewSecretaryFinder(controllerUUID),
		Store:                lease.NewStore(db, logger),
		Logger:               logger,
		Clock:                clock,
		MaxSleep:             time.Minute,
		EntityUUID:           controllerUUID,
		PrometheusRegisterer: noopRegisterer{},
		Tracer:               trace.NoopTracer{},
	})
}

func multiWatcher(c *gc.C, statePool *state.StatePool, clock clock.Clock) *wmultiwatcher.Worker {
	allWatcherBacking, err := state.NewAllWatcherBacking(statePool)
	c.Assert(err, jc.ErrorIsNil)
	multiWatcherWorker, err := wmultiwatcher.NewWorker(wmultiwatcher.Config{
		Clock:                clock,
		Logger:               loggo.GetLogger("dummy.multiwatcher"),
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	})
	c.Assert(err, jc.ErrorIsNil)
	return multiWatcherWorker
}

func (s *ApiServerSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.ControllerSuite.SetUpSuite(c)
}

func mongoInfo() mongo.MongoInfo {
	if mgotesting.MgoServer.Addr() == "" {
		panic("ApiServer tests must be run with MgoTestPackage")
	}
	mongoPort := strconv.Itoa(mgotesting.MgoServer.Port())
	addrs := []string{net.JoinHostPort("localhost", mongoPort)}
	return mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      addrs,
			CACert:     coretesting.CACert,
			DisableTLS: !mgotesting.MgoServer.SSLEnabled(),
		},
	}
}

func (s *ApiServerSuite) setupHttpServer(c *gc.C) {
	s.mux = apiserverhttp.NewMux()

	certPool, err := api.CreateCertPool(coretesting.CACert)
	c.Assert(err, jc.ErrorIsNil)
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}

	// Note that we can't listen on localhost here because
	// TestAPIServerCanListenOnBothIPv4AndIPv6 assumes
	// that we listen on IPv6 too, and listening on localhost does not do that.
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	s.httpServer = &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: s.mux},
		TLS:      tlsConfig,
	}
	s.httpServer.TLS = tlsConfig
	s.httpServer.StartTLS()

	baseURL, err := url.Parse(s.httpServer.URL)
	c.Assert(err, jc.ErrorIsNil)
	s.baseURL = baseURL
}

func (s *ApiServerSuite) setupControllerModel(c *gc.C, controllerCfg controller.Config) {
	session, err := mongo.DialWithInfo(mongoInfo(), mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	apiPort := s.httpServer.Listener.Addr().(*net.TCPAddr).Port
	controllerCfg[controller.APIPort] = apiPort

	modelAttrs := coretesting.Attrs{
		"name": "controller",
		"type": DefaultCloud.Type,
	}
	for k, v := range s.ControllerModelConfigAttrs {
		modelAttrs[k] = v
	}
	controllerModelCfg := coretesting.CustomModelConfig(c, modelAttrs)
	s.ServiceFactorySuite.ControllerModelUUID = coremodel.UUID(controllerModelCfg.UUID())
	s.ServiceFactorySuite.SetUpTest(c)

	modelType := state.ModelTypeIAAS
	if s.WithControllerModelType == state.ModelTypeCAAS {
		modelType = s.WithControllerModelType
	}

	// modelUUID param is not used so can pass in anything.
	serviceFactory := s.ControllerServiceFactory(c)

	ctrl, err := state.Initialize(state.InitializeParams{
		Clock: clock.WallClock,
		// TODO (stickupkid): Remove controller config from the state
		// InitializeParams once we have removed the controller config
		// from the state.
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:            modelType,
			Owner:           AdminUser,
			Config:          controllerModelCfg,
			CloudName:       DefaultCloud.Name,
			CloudRegion:     DefaultCloudRegion,
			CloudCredential: DefaultCredentialTag,
		},
		CloudName:     DefaultCloud.Name,
		MongoSession:  session,
		AdminPassword: AdminSecret,
		NewPolicy: stateenvirons.GetNewPolicyFunc(serviceFactory.Cloud(), serviceFactory.Credential(), func(modelUUID string, registry storage.ProviderRegistry) state.StoragePoolGetter {
			return s.ServiceFactoryGetter(c).FactoryForModel(modelUUID).Storage(registry)
		}),
	}, environs.ProviderConfigSchemaSource(serviceFactory.Cloud()))
	c.Assert(err, jc.ErrorIsNil)
	s.controller = ctrl

	// Set the api host ports in state.
	sHsPs := []network.SpaceHostPorts{{
		network.SpaceHostPort{
			SpaceAddress: network.SpaceAddress{MachineAddress: network.MachineAddress{
				Value: "localhost",
				Type:  network.AddressType("hostname"),
			}},
			NetPort: network.NetPort(apiPort),
		},
	}}
	st, err := ctrl.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetAPIHostPorts(controllerCfg, sHsPs, sHsPs)
	c.Assert(err, jc.ErrorIsNil)

	// Allow "dummy" cloud.
	err = InsertDummyCloudType(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	// Seed the test database with the controller cloud and credential etc.
	s.AdminUserUUID = s.ServiceFactorySuite.AdminUserUUID
	SeedDatabase(c, s.TxnRunner(), controllerCfg)
}

func (s *ApiServerSuite) setupApiServer(c *gc.C, controllerCfg controller.Config) {
	cfg := DefaultServerConfig(c, s.Clock)
	cfg.Mux = s.mux
	cfg.DBGetter = stubDBGetter{db: stubWatchableDB{TxnRunner: s.TxnRunner()}}
	cfg.ServiceFactoryGetter = s.ServiceFactoryGetter(c)
	cfg.StatePool = s.controller.StatePool()
	cfg.PublicDNSName = controllerCfg.AutocertDNSName()

	cfg.UpgradeComplete = func() bool {
		return !s.WithUpgrading
	}
	cfg.GetAuditConfig = func() auditlog.Config {
		if s.WithAuditLogConfig != nil {
			return *s.WithAuditLogConfig
		}
		return auditlog.Config{Enabled: false}
	}
	if s.WithMultiWatcher {
		cfg.MultiwatcherFactory = multiWatcher(c, cfg.StatePool, s.Clock)
	}
	if s.WithIntrospection != nil {
		cfg.RegisterIntrospectionHandlers = s.WithIntrospection
	}
	if s.WithEmbeddedCLICommand != nil {
		cfg.ExecEmbeddedCommand = s.WithEmbeddedCLICommand
	}
	if s.WithLeaseManager {
		leaseManager, err := leaseManager(coretesting.ControllerTag.Id(), databasetesting.SingularDBGetter(s.TxnRunner()), s.Clock)
		c.Assert(err, jc.ErrorIsNil)
		cfg.LeaseManager = leaseManager
		s.LeaseManager = leaseManager
	}

	cfg.ObjectStoreGetter = &stubObjectStoreGetter{
		rootDir:              c.MkDir(),
		claimer:              objectstoretesting.MemoryClaimer(),
		serviceFactoryGetter: cfg.ServiceFactoryGetter,
	}
	s.ObjectStoreGetter = cfg.ObjectStoreGetter

	// Set up auth handler.
	factory := s.ControllerServiceFactory(c)

	systemState, err := cfg.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	agentAuthFactory := authentication.NewAgentAuthenticatorFactory(systemState, nil)

	authenticator, err := stateauthenticator.NewAuthenticator(cfg.StatePool, systemState, factory.ControllerConfig(), factory.Access(), agentAuthFactory, cfg.Clock)
	c.Assert(err, jc.ErrorIsNil)
	cfg.LocalMacaroonAuthenticator = authenticator
	err = authenticator.AddHandlers(s.mux)
	c.Assert(err, jc.ErrorIsNil)

	s.Server, err = apiserver.NewServer(context.Background(), cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.apiInfo = api.Info{
		Addrs:  []string{fmt.Sprintf("localhost:%d", s.httpServer.Listener.Addr().(*net.TCPAddr).Port)},
		CACert: coretesting.CACert,
	}
}

func (s *ApiServerSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)

	s.InstancePrechecker = func(c *gc.C, s *state.State) environs.InstancePrechecker {
		return state.NoopInstancePrechecker{}
	}
	s.ConfigSchemaSourceGetter = func(c *gc.C) environsconfig.ConfigSchemaSourceGetter {
		return state.NoopConfigSchemaSource
	}

	if s.Clock == nil {
		s.Clock = testclock.NewClock(time.Now())
	}
	s.setupHttpServer(c)

	controllerCfg := coretesting.FakeControllerConfig()
	for key, value := range s.ControllerConfigAttrs {
		controllerCfg[key] = value
	}
	s.setupControllerModel(c, controllerCfg)
	s.setupApiServer(c, controllerCfg)
}

func (s *ApiServerSuite) TearDownTest(c *gc.C) {
	s.WithMultiWatcher = false
	s.WithLeaseManager = false
	s.WithAuditLogConfig = nil
	s.WithUpgrading = false
	s.WithIntrospection = nil
	s.WithEmbeddedCLICommand = nil
	s.WithControllerModelType = ""

	s.tearDownConn(c)
	if s.Server != nil {
		err := s.Server.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	s.ServiceFactorySuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

// InsertDummyCloudType is a db bootstrap option which inserts the dummy cloud type.
func InsertDummyCloudType(ctx context.Context, controller, model database.TxnRunner) error {
	return cloudstate.AllowCloudType(ctx, controller, 666, "dummy")
}

// URL returns a URL for this server with the given path and
// query parameters. The URL scheme will be "https".
func (s *ApiServerSuite) URL(path string, queryParams url.Values) *url.URL {
	url := *s.baseURL
	url.Path = path
	url.RawQuery = queryParams.Encode()
	return &url
}

// ObjectStore returns the object store for the given model uuid.
func (s *ApiServerSuite) ObjectStore(c *gc.C, uuid string) objectstore.ObjectStore {
	store, err := s.ObjectStoreGetter.GetObjectStore(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	return store
}

// openAPIAs opens the API and ensures that the api.Connection returned will be
// closed during the test teardown by using a cleanup function.
func (s *ApiServerSuite) openAPIAs(c *gc.C, tag names.Tag, password, nonce string, modelUUID string) api.Connection {
	apiInfo := s.apiInfo
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	if modelUUID != "" {
		apiInfo.ModelTag = names.NewModelTag(modelUUID)
	}
	conn, err := api.Open(&apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	s.apiConns = append(s.apiConns, conn)
	return conn
}

// ControllerModelApiInfo returns the api address and ca cert needed to
// connect to an api server's controller model endpoint. User and password are empty.
func (s *ApiServerSuite) ControllerModelApiInfo() *api.Info {
	return s.ModelApiInfo(s.ServiceFactorySuite.ControllerModelUUID.String())
}

// ModelApiInfo returns the api address and ca cert needed to
// connect to an api server's model endpoint. User and password are empty.
func (s *ApiServerSuite) ModelApiInfo(modelUUID string) *api.Info {
	info := s.apiInfo
	info.ControllerUUID = coretesting.ControllerTag.Id()
	info.ModelTag = names.NewModelTag(modelUUID)
	return &info
}

// OpenControllerAPIAs opens a controller api connection.
func (s *ApiServerSuite) OpenControllerAPIAs(c *gc.C, tag names.Tag, password string) api.Connection {
	return s.openAPIAs(c, tag, password, "", "")
}

// OpenControllerAPI opens a controller api connection for the admin user.
func (s *ApiServerSuite) OpenControllerAPI(c *gc.C) api.Connection {
	return s.OpenControllerAPIAs(c, AdminUser, AdminSecret)
}

// OpenModelAPIAs opens a model api connection.
func (s *ApiServerSuite) OpenModelAPIAs(c *gc.C, modelUUID string, tag names.Tag, password, nonce string) api.Connection {
	return s.openAPIAs(c, tag, password, nonce, modelUUID)
}

// OpenControllerModelAPI opens the controller model api connection for the admin user.
func (s *ApiServerSuite) OpenControllerModelAPI(c *gc.C) api.Connection {
	return s.openAPIAs(c, AdminUser, AdminSecret, "", s.ServiceFactorySuite.ControllerModelUUID.String())
}

// OpenModelAPI opens a model api connection for the admin user.
func (s *ApiServerSuite) OpenModelAPI(c *gc.C, modelUUID string) api.Connection {
	return s.openAPIAs(c, AdminUser, AdminSecret, "", modelUUID)
}

// OpenAPIAsNewMachine creates a new machine entry that lives in system state,
// and then uses that to open the API. The returned api.Connection should not be
// closed by the caller as a cleanup function has been registered to do that.
// The machine will run the supplied jobs; if none are given, JobHostUnits is assumed.
func (s *ApiServerSuite) OpenAPIAsNewMachine(c *gc.C, jobs ...state.MachineJob) (api.Connection, *state.Machine) {
	if len(jobs) == 0 {
		jobs = []state.MachineJob{state.JobHostUnits}
	}

	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(state.NoopInstancePrechecker{}, state.UbuntuBase("12.10"), jobs...)
	c.Assert(err, jc.ErrorIsNil)
	password, err := password.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return s.openAPIAs(c, machine.Tag(), password, "fake_nonce", st.ModelUUID()), machine
}

// StatePool returns the server's state pool.
func (s *ApiServerSuite) StatePool() *state.StatePool {
	return s.controller.StatePool()
}

// NewFactory returns a factory for the given model.
func (s *ApiServerSuite) NewFactory(c *gc.C, modelUUID string) (*factory.Factory, func() bool) {
	var (
		st       *state.State
		model    *state.Model
		releaser func() bool
		err      error
	)
	if modelUUID == s.ServiceFactorySuite.ControllerModelUUID.String() {
		st, err = s.controller.SystemState()
		c.Assert(err, jc.ErrorIsNil)
		model = s.ControllerModel(c)
		releaser = func() bool { return true }
	} else {
		pooledSt, err := s.controller.GetState(names.NewModelTag(modelUUID))
		c.Assert(err, jc.ErrorIsNil)
		releaser = pooledSt.Release
		st = pooledSt.State
		model, err = st.Model()
		c.Assert(err, jc.ErrorIsNil)
	}

	modelServiceFactory := s.ServiceFactoryGetter(c).FactoryForModel(modelUUID)
	var registry storage.ProviderRegistry
	if model.Type() == state.ModelTypeIAAS {
		registry = storage.ChainedProviderRegistry{
			dummy.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	} else {
		registry = storage.ChainedProviderRegistry{
			storage.StaticProviderRegistry{
				Providers: map[storage.ProviderType]storage.Provider{
					"kubernetes": &dummy.StorageProvider{},
				},
			},
			provider.CommonStorageProviders(),
		}
	}
	return factory.NewFactory(st, s.controller.StatePool(), coretesting.FakeControllerConfig()).
		WithApplicationService(modelServiceFactory.Application(registry)), releaser
}

// ControllerModelUUID returns the controller model uuid.
func (s *ApiServerSuite) ControllerModelUUID() string {
	return s.ServiceFactorySuite.ControllerModelUUID.String()
}

// ControllerModel returns the controller model.
func (s *ApiServerSuite) ControllerModel(c *gc.C) *state.Model {
	st, err := s.controller.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	return m
}

// Model returns the specified model.
func (s *ApiServerSuite) Model(c *gc.C, uuid string) (*state.Model, func() bool) {
	m, helper, err := s.controller.StatePool().GetModel(uuid)
	c.Assert(err, jc.ErrorIsNil)
	return m, helper.Release
}

func (s *ApiServerSuite) tearDownConn(c *gc.C) {
	testServer := mgotesting.MgoServer.Addr()
	serverDead := testServer == "" || s.Server == nil

	// Close any api connections we know about first.
	for _, st := range s.apiConns {
		err := st.Close()
		if !serverDead {
			c.Check(err, jc.ErrorIsNil)
		}
	}
	s.apiConns = nil
	if s.controller != nil {
		err := s.controller.Close()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *ApiServerSuite) SeedCAASCloud(c *gc.C) {
	cred := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": "dummy",
			"password": "secret",
		},
	}

	cloudUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	credUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return cloudstate.CreateCloud(ctx, tx, cloudUUID.String(), cloud.Cloud{
			Name:      "caascloud",
			Type:      "kubernetes",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		})
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return credentialstate.CreateCredential(ctx, tx, credUUID.String(), corecredential.Key{
			Cloud: "caascloud",
			Owner: "admin",
			Name:  "dummy-credential",
		}, cred)
	})
	c.Assert(err, jc.ErrorIsNil)
}

// SeedDatabase the database with a supplied controller config, and dummy
// cloud and dummy credentials.
func SeedDatabase(c *gc.C, controller database.TxnRunner, controllerConfig controller.Config) {
	ctx := context.Background()
	err := controllerconfigbootstrap.InsertInitialControllerConfig(controllerConfig)(ctx, controller, noopTxnRunner{})
	c.Assert(err, jc.ErrorIsNil)
}

// DefaultServerConfig returns a minimal server config.
func DefaultServerConfig(c *gc.C, testclock clock.Clock) apiserver.ServerConfig {
	if testclock == nil {
		testclock = clock.WallClock
	}
	fakeOrigin := names.NewMachineTag("0")
	hub := centralhub.New(fakeOrigin, centralhub.PubsubNoOpMetrics{})
	return apiserver.ServerConfig{
		Clock:                      testclock,
		Tag:                        names.NewMachineTag("0"),
		LogDir:                     c.MkDir(),
		DataDir:                    c.MkDir(),
		Hub:                        hub,
		MultiwatcherFactory:        &fakeMultiwatcherFactory{},
		Presence:                   &fakePresence{},
		LeaseManager:               apitesting.StubLeaseManager{},
		NewObserver:                func() observer.Observer { return &fakeobserver.Instance{} },
		MetricsCollector:           apiserver.NewMetricsCollector(),
		UpgradeComplete:            func() bool { return true },
		LogSink:                    noopLogSink{},
		CharmhubHTTPClient:         &http.Client{},
		DBGetter:                   stubDBGetter{},
		ServiceFactoryGetter:       nil,
		TracerGetter:               &stubTracerGetter{},
		ObjectStoreGetter:          &stubObjectStoreGetter{},
		StatePool:                  &state.StatePool{},
		Mux:                        &apiserverhttp.Mux{},
		LocalMacaroonAuthenticator: &mockAuthenticator{},
		GetAuditConfig:             func() auditlog.Config { return auditlog.Config{} },
	}
}

type stubDBGetter struct {
	db changestream.WatchableDB
}

func (s stubDBGetter) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	if namespace != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, namespace)
	}
	return s.db, nil
}

type stubTracerGetter struct{}

func (s *stubTracerGetter) GetTracer(ctx context.Context, namespace trace.TracerNamespace) (trace.Tracer, error) {
	return trace.NoopTracer{}, nil
}

type stubObjectStoreGetter struct {
	rootDir              string
	claimer              internalobjectstore.Claimer
	serviceFactoryGetter servicefactory.ServiceFactoryGetter
}

func (s *stubObjectStoreGetter) GetObjectStore(ctx context.Context, namespace string) (objectstore.ObjectStore, error) {
	serviceFactory := s.serviceFactoryGetter.FactoryForModel(namespace)

	return internalobjectstore.ObjectStoreFactory(ctx,
		internalobjectstore.DefaultBackendType(),
		namespace,
		internalobjectstore.WithRootDir(s.rootDir),
		internalobjectstore.WithMetadataService(&stubMetadataService{serviceFactory: serviceFactory}),
		internalobjectstore.WithClaimer(s.claimer),
		internalobjectstore.WithLogger(loggo.GetLogger("juju.objectstore")),
	)
}

type stubMetadataService struct {
	serviceFactory servicefactory.ServiceFactory
}

func (s *stubMetadataService) ObjectStore() objectstore.ObjectStoreMetadata {
	return s.serviceFactory.ObjectStore()
}

type stubWatchableDB struct {
	database.TxnRunner
}

func (stubWatchableDB) Subscribe(...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return nil, nil
}

// These mocks are used in place of real components when creating server config.

type noopLogger struct{}

func (noopLogger) Log([]corelogger.LogRecord) error { return nil }

func (noopLogger) Close() error { return nil }

type noopLogSink struct{}

func (s noopLogSink) GetLogger(modelUUID, modelName, modelOwner string) (corelogger.LoggerCloser, error) {
	return &noopLogger{}, nil
}

func (s noopLogSink) RemoveLogger(modelUUID string) error {
	return nil
}

func (s noopLogSink) Close() error {
	return nil
}

func (noopLogSink) Log([]corelogger.LogRecord) error { return nil }

type fakeMultiwatcherFactory struct {
	multiwatcher.Factory
}

type mockAuthenticator struct {
	macaroon.LocalMacaroonAuthenticator
}

// fakePresence returns alive for all agent alive requests.
type fakePresence struct {
	agent map[string]presence.Status
}

func (*fakePresence) Disable()        {}
func (*fakePresence) Enable()         {}
func (*fakePresence) IsEnabled() bool { return true }
func (*fakePresence) Connect(server, model, agent string, id uint64, controllerAgent bool, userData string) {
}
func (*fakePresence) Disconnect(server string, id uint64)                            {}
func (*fakePresence) Activity(server string, id uint64)                              {}
func (*fakePresence) ServerDown(server string)                                       {}
func (*fakePresence) UpdateServer(server string, connections []presence.Value) error { return nil }
func (f *fakePresence) Connections() presence.Connections                            { return f }

func (f *fakePresence) ForModel(model string) presence.Connections   { return f }
func (f *fakePresence) ForServer(server string) presence.Connections { return f }
func (f *fakePresence) ForAgent(agent string) presence.Connections   { return f }
func (*fakePresence) Count() int                                     { return 0 }
func (*fakePresence) Models() []string                               { return nil }
func (*fakePresence) Servers() []string                              { return nil }
func (*fakePresence) Agents() []string                               { return nil }
func (*fakePresence) Values() []presence.Value                       { return nil }

func (f *fakePresence) AgentStatus(agent string) (presence.Status, error) {
	if status, found := f.agent[agent]; found {
		return status, nil
	}
	return presence.Alive, nil
}

type noopTxnRunner struct{}

func (noopTxnRunner) Txn(context.Context, func(context.Context, *sqlair.TX) error) error {
	return errors.NotImplemented
}

func (noopTxnRunner) StdTxn(context.Context, func(context.Context, *sql.Tx) error) error {
	return errors.NotImplemented
}
