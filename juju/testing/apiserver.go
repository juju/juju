// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/prometheus/client_golang/prometheus"

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
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/controllernode"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	servicefactorytesting "github.com/juju/juju/domain/services/testing"
	"github.com/juju/juju/internal/cmd"
	databasetesting "github.com/juju/juju/internal/database/testing"
	internallease "github.com/juju/juju/internal/lease"
	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

const AdminSecret = "dummy-secret"

var (
	// AdminUser is the default admin user for a controller.
	AdminUser = names.NewUserTag("admin")
	AdminName = coreuser.NameFromTag(AdminUser)

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
	servicefactorytesting.DomainServicesSuite

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
	WithControllerModelType state.ModelType
	WithEmbeddedCLICommand  func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusArgs string) int

	// These can be set prior to login being called.

	WithUpgrading      bool
	WithAuditLogConfig *auditlog.Config
	WithIntrospection  func(func(string, http.Handler))

	// AdminUserUUID is the root user for the controller.
	AdminUserUUID coreuser.UUID

	// ControllerUUID is the unique identifier for the controller.
	ControllerUUID string

	objectStoresMutex sync.Mutex
	objectStores      []objectstore.ObjectStore
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

func leaseManager(c *tc.C, controllerUUID string, db database.DBGetter, clock clock.Clock) (*lease.Manager, error) {
	logger := loggertesting.WrapCheckLog(c)
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

func (s *ApiServerSuite) SetUpSuite(c *tc.C) {
	s.DomainServicesSuite.SetUpSuite(c)
	s.ControllerSuite.SetUpSuite(c)
}

func (s *ApiServerSuite) setupHttpServer(c *tc.C) {
	s.mux = apiserverhttp.NewMux()

	certPool, err := api.CreateCertPool(coretesting.CACert)
	c.Assert(err, tc.ErrorIsNil)
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}

	// Note that we can't listen on localhost here because
	// TestAPIServerCanListenOnBothIPv4AndIPv6 assumes
	// that we listen on IPv6 too, and listening on localhost does not do that.
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, tc.ErrorIsNil)
	s.httpServer = &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: s.mux},
		TLS:      tlsConfig,
	}
	s.httpServer.TLS = tlsConfig
	s.httpServer.StartTLS()

	baseURL, err := url.Parse(s.httpServer.URL)
	c.Assert(err, tc.ErrorIsNil)
	s.baseURL = baseURL
}

func (s *ApiServerSuite) setupControllerModel(c *tc.C, controllerCfg controller.Config) {
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
	s.DomainServicesSuite.ControllerConfig = controllerCfg
	s.DomainServicesSuite.ControllerModelUUID = coremodel.UUID(controllerModelCfg.UUID())
	s.DomainServicesSuite.SetUpTest(c)

	modelType := state.ModelTypeIAAS
	if s.WithControllerModelType == state.ModelTypeCAAS {
		modelType = s.WithControllerModelType
	}

	// modelUUID param is not used so can pass in anything.
	domainServices := s.ControllerDomainServices(c)

	storageServiceGetter := func(modelUUID coremodel.UUID) (state.StoragePoolGetter, error) {
		svc, err := s.DomainServicesGetter(c, s.NoopObjectStore(c), s.NoopLeaseManager(c)).ServicesForModel(c.Context(), modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc.Storage(), nil
	}
	ctrl, err := state.Initialize(state.InitializeParams{
		Clock: clock.WallClock,
		// Pass the minimal controller config needed for bootstrap, the rest
		// should be added through the controller config service.
		ControllerConfig: controller.Config{
			controller.ControllerUUIDKey: controllerCfg.ControllerUUID(),
		},
		ControllerModelArgs: state.ModelArgs{
			Name:            controllerModelCfg.Name(),
			UUID:            coremodel.UUID(controllerModelCfg.UUID()),
			Type:            modelType,
			Owner:           AdminUser,
			CloudName:       DefaultCloud.Name,
			CloudRegion:     DefaultCloudRegion,
			CloudCredential: DefaultCredentialTag,
		},
		CloudName:        DefaultCloud.Name,
		NewPolicy:        stateenvirons.GetNewPolicyFunc(storageServiceGetter),
		SSHServerHostKey: coretesting.SSHServerHostKey,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.controller = ctrl

	// Set the api host ports in state.
	apiAddrArgs := controllernode.SetAPIAddressArgs{
		MgmtSpace: nil,
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": {
				network.SpaceHostPort{
					SpaceAddress: network.SpaceAddress{MachineAddress: network.MachineAddress{
						Value: "localhost",
						Type:  network.AddressType("hostname"),
					}},
					NetPort: network.NetPort(apiPort),
				},
			},
		},
	}
	controllerNodeService := domainServices.ControllerNode()
	err = controllerNodeService.SetAPIAddresses(c.Context(), apiAddrArgs)
	c.Assert(err, tc.ErrorIsNil)

	// Allow "dummy" cloud.
	err = InsertDummyCloudType(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	// Seed the test database with the controller cloud and credential etc.
	s.AdminUserUUID = s.DomainServicesSuite.AdminUserUUID
	SeedDatabase(c, s.TxnRunner(), domainServices, controllerCfg)
}

func (s *ApiServerSuite) setupApiServer(c *tc.C, controllerCfg controller.Config) {
	cfg := DefaultServerConfig(c, s.Clock)
	cfg.Mux = s.mux
	cfg.DBGetter = stubDBGetter{db: stubWatchableDB{TxnRunner: s.TxnRunner()}}
	cfg.DBDeleter = stubDBDeleter{}
	cfg.DomainServicesGetter = s.DomainServicesGetter(c, s.NoopObjectStore(c), s.NoopLeaseManager(c))
	cfg.ControllerConfigService = s.ControllerDomainServices(c).ControllerConfig()
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
	if s.WithIntrospection != nil {
		cfg.RegisterIntrospectionHandlers = s.WithIntrospection
	}
	if s.WithEmbeddedCLICommand != nil {
		cfg.ExecEmbeddedCommand = s.WithEmbeddedCLICommand
	}
	if s.WithLeaseManager {
		leaseManager, err := leaseManager(c, coretesting.ControllerTag.Id(), databasetesting.SingularDBGetter(s.TxnRunner()), s.Clock)
		c.Assert(err, tc.ErrorIsNil)
		cfg.LeaseManager = leaseManager
		s.LeaseManager = leaseManager
	}

	cfg.ObjectStoreGetter = &stubObjectStoreGetter{
		suite:                     s,
		rootDir:                   c.MkDir(),
		claimer:                   objectstoretesting.MemoryClaimer(),
		objectStoreServicesGetter: s.ObjectStoreServicesGetter(c),
	}
	s.ObjectStoreGetter = cfg.ObjectStoreGetter

	// Set up auth handler.
	factory := s.ControllerDomainServices(c)

	agentAuthGetter := authentication.NewAgentAuthenticatorGetter(factory.AgentPassword(), nil)

	authenticator, err := stateauthenticator.NewAuthenticator(
		c.Context(),
		cfg.ControllerModelUUID,
		factory.ControllerConfig(),
		agentPasswordServiceGetter{
			DomainServicesGetter: s.ModelDomainServicesGetter(c),
		},
		factory.Access(),
		factory.Macaroon(),
		agentAuthGetter,
		cfg.Clock,
	)
	c.Assert(err, tc.ErrorIsNil)
	cfg.LocalMacaroonAuthenticator = authenticator
	err = authenticator.AddHandlers(s.mux)
	c.Assert(err, tc.ErrorIsNil)

	s.Server, err = apiserver.NewServer(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.apiInfo = api.Info{
		Addrs:  []string{fmt.Sprintf("localhost:%d", s.httpServer.Listener.Addr().(*net.TCPAddr).Port)},
		CACert: coretesting.CACert,
	}
}

type agentPasswordServiceGetter struct {
	services.DomainServicesGetter
}

// GetAgentPasswordServiceForModel returns a AgentPasswordService for the given
// model.
func (s agentPasswordServiceGetter) GetAgentPasswordServiceForModel(ctx context.Context, modelUUID coremodel.UUID) (authentication.AgentPasswordService, error) {
	svc, err := s.DomainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return svc.AgentPassword(), nil
}

func (s *ApiServerSuite) SetUpTest(c *tc.C) {

	if s.Clock == nil {
		s.Clock = testclock.NewClock(time.Now())
	}
	s.setupHttpServer(c)

	controllerCfg := coretesting.FakeControllerConfig()
	for key, value := range s.ControllerConfigAttrs {
		controllerCfg[key] = value
	}
	s.ControllerUUID = controllerCfg.ControllerUUID()
	s.setupControllerModel(c, controllerCfg)
	s.setupApiServer(c, controllerCfg)
}

func (s *ApiServerSuite) TearDownTest(c *tc.C) {
	if s.LeaseManager != nil {
		s.LeaseManager.Kill()
	}

	s.WithLeaseManager = false
	s.WithAuditLogConfig = nil
	s.WithUpgrading = false
	s.WithIntrospection = nil
	s.WithEmbeddedCLICommand = nil
	s.WithControllerModelType = ""

	s.tearDownConn(c)
	if s.Server != nil {
		err := s.Server.Stop()
		if err != nil {
			c.Assert(err, tc.ErrorIs, apiserver.ErrAPIServerDying)
		}
	}
	if s.httpServer != nil {
		s.httpServer.Close()
	}

	s.objectStoresMutex.Lock()
	for _, store := range s.objectStores {
		w, ok := store.(worker.Worker)
		if !ok {
			c.Fatalf("object store %T does not implement worker.Worker", store)
		}
		w.Kill()
	}
	s.objectStores = nil
	s.objectStoresMutex.Unlock()

	s.DomainServicesSuite.TearDownTest(c)
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
func (s *ApiServerSuite) ObjectStore(c *tc.C, uuid string) objectstore.ObjectStore {
	store, err := s.ObjectStoreGetter.GetObjectStore(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	return store
}

// openAPIAs opens the API and ensures that the api.Connection returned will be
// closed during the test teardown by using a cleanup function.
func (s *ApiServerSuite) openAPIAs(c *tc.C, tag names.Tag, password, nonce string, modelUUID string) api.Connection {
	apiInfo := s.apiInfo
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	if modelUUID != "" {
		apiInfo.ModelTag = names.NewModelTag(modelUUID)
	}
	conn, err := api.Open(c.Context(), &apiInfo, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)
	s.apiConns = append(s.apiConns, conn)
	return conn
}

// ControllerModelApiInfo returns the api address and ca cert needed to
// connect to an api server's controller model endpoint. User and password are empty.
func (s *ApiServerSuite) ControllerModelApiInfo() *api.Info {
	return s.ModelApiInfo(s.DomainServicesSuite.ControllerModelUUID.String())
}

// ModelApiInfo returns the api address and ca cert needed to
// connect to an api server's model endpoint. User and password are empty.
func (s *ApiServerSuite) ModelApiInfo(modelUUID string) *api.Info {
	info := s.apiInfo
	info.ControllerUUID = s.ControllerUUID
	info.ModelTag = names.NewModelTag(modelUUID)
	return &info
}

// OpenControllerAPIAs opens a controller api connection.
func (s *ApiServerSuite) OpenControllerAPIAs(c *tc.C, tag names.Tag, password string) api.Connection {
	return s.openAPIAs(c, tag, password, "", "")
}

// OpenControllerAPI opens a controller api connection for the admin user.
func (s *ApiServerSuite) OpenControllerAPI(c *tc.C) api.Connection {
	return s.OpenControllerAPIAs(c, AdminUser, AdminSecret)
}

// OpenModelAPIAs opens a model api connection.
func (s *ApiServerSuite) OpenModelAPIAs(c *tc.C, modelUUID string, tag names.Tag, password, nonce string) api.Connection {
	return s.openAPIAs(c, tag, password, nonce, modelUUID)
}

// OpenControllerModelAPI opens the controller model api connection for the admin user.
func (s *ApiServerSuite) OpenControllerModelAPI(c *tc.C) api.Connection {
	return s.openAPIAs(c, AdminUser, AdminSecret, "", s.DomainServicesSuite.ControllerModelUUID.String())
}

// OpenModelAPI opens a model api connection for the admin user.
func (s *ApiServerSuite) OpenModelAPI(c *tc.C, modelUUID string) api.Connection {
	return s.openAPIAs(c, AdminUser, AdminSecret, "", modelUUID)
}

// StatePool returns the server's state pool.
func (s *ApiServerSuite) StatePool() *state.StatePool {
	return s.controller.StatePool()
}

// NewFactory returns a factory for the given model.
func (s *ApiServerSuite) NewFactory(c *tc.C, modelUUID string) (*factory.Factory, func() bool) {
	var (
		st       *state.State
		releaser func() bool
		err      error
	)
	if modelUUID == s.DomainServicesSuite.ControllerModelUUID.String() {
		st, err = s.controller.SystemState()
		c.Assert(err, tc.ErrorIsNil)
		releaser = func() bool { return true }
	} else {
		pooledSt, err := s.controller.GetState(names.NewModelTag(modelUUID))
		c.Assert(err, tc.ErrorIsNil)
		releaser = pooledSt.Release
		st = pooledSt.State
	}

	modelDomainServices, err := s.DomainServicesGetter(c, s.NoopObjectStore(c), servicefactorytesting.TestingLeaseManager{}).ServicesForModel(c.Context(), coremodel.UUID(modelUUID))
	c.Assert(err, tc.ErrorIsNil)

	applicationService := modelDomainServices.Application()
	return factory.NewFactory(st, s.controller.StatePool(), coretesting.FakeControllerConfig()).
		WithApplicationService(applicationService), releaser
}

// ControllerModelUUID returns the controller model uuid.
func (s *ApiServerSuite) ControllerModelUUID() string {
	return s.DomainServicesSuite.ControllerModelUUID.String()
}

// ControllerModel returns the controller model.
func (s *ApiServerSuite) ControllerModel(c *tc.C) *state.Model {
	st, err := s.controller.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	return m
}

// Model returns the specified model.
func (s *ApiServerSuite) Model(c *tc.C, uuid string) (*state.Model, func() bool) {
	m, helper, err := s.controller.StatePool().GetModel(uuid)
	c.Assert(err, tc.ErrorIsNil)
	return m, helper.Release
}

func (s *ApiServerSuite) tearDownConn(c *tc.C) {
	// Close any api connections we know about first.
	for _, st := range s.apiConns {
		st.Close()
	}
	s.apiConns = nil
	if s.controller != nil {
		err := s.controller.Close()
		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *ApiServerSuite) SeedCAASCloud(c *tc.C) {
	cred := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": "dummy",
			"password": "secret",
		},
	}

	cloudUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	credUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return cloudstate.CreateCloud(ctx, tx, AdminName, cloudUUID.String(), cloud.Cloud{
			Name:      "caascloud",
			Type:      "kubernetes",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		})
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return credentialstate.CreateCredential(ctx, tx, credUUID.String(), corecredential.Key{
			Cloud: "caascloud",
			Owner: AdminName,
			Name:  "dummy-credential",
		}, cred)
	})
	c.Assert(err, tc.ErrorIsNil)
}

// SeedDatabase the database with a supplied controller config, and dummy
// cloud and dummy credentials.
func SeedDatabase(c *tc.C, controller database.TxnRunner, domainServices services.DomainServices, controllerConfig controller.Config) {
	bakeryConfigService := domainServices.Macaroon()
	err := bakeryConfigService.InitialiseBakeryConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// DefaultServerConfig returns a minimal server config.
func DefaultServerConfig(c *tc.C, testclock clock.Clock) apiserver.ServerConfig {
	if testclock == nil {
		testclock = clock.WallClock
	}
	return apiserver.ServerConfig{
		Clock:                      testclock,
		Tag:                        names.NewMachineTag("0"),
		LogDir:                     c.MkDir(),
		DataDir:                    c.MkDir(),
		LeaseManager:               apitesting.StubLeaseManager{},
		NewObserver:                func() observer.Observer { return &fakeobserver.Instance{} },
		MetricsCollector:           apiserver.NewMetricsCollector(),
		UpgradeComplete:            func() bool { return true },
		LogSink:                    noopLogSink{},
		CharmhubHTTPClient:         &http.Client{},
		DBGetter:                   stubDBGetter{},
		DomainServicesGetter:       nil,
		TracerGetter:               &stubTracerGetter{},
		ObjectStoreGetter:          &stubObjectStoreGetter{},
		StatePool:                  &state.StatePool{},
		Mux:                        &apiserverhttp.Mux{},
		LocalMacaroonAuthenticator: &mockAuthenticator{},
		GetAuditConfig:             func() auditlog.Config { return auditlog.Config{} },
		ControllerUUID:             coretesting.ControllerTag.Id(),
		ControllerModelUUID:        coremodel.UUID(coretesting.ModelTag.Id()),
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

type stubDBDeleter struct{}

func (s stubDBDeleter) DeleteDB(namespace string) error {
	return nil
}

type stubTracerGetter struct{}

func (s *stubTracerGetter) GetTracer(ctx context.Context, namespace trace.TracerNamespace) (trace.Tracer, error) {
	return trace.NoopTracer{}, nil
}

type stubObjectStoreGetter struct {
	suite                     *ApiServerSuite
	rootDir                   string
	claimer                   internalobjectstore.Claimer
	objectStoreServicesGetter services.ObjectStoreServicesGetter
}

func (s *stubObjectStoreGetter) GetObjectStore(ctx context.Context, namespace string) (objectstore.ObjectStore, error) {
	services := s.objectStoreServicesGetter.ServicesForModel(coremodel.UUID(namespace))

	store, err := internalobjectstore.ObjectStoreFactory(ctx,
		internalobjectstore.DefaultBackendType(),
		namespace,
		internalobjectstore.WithRootDir(s.rootDir),
		internalobjectstore.WithMetadataService(&stubMetadataService{services: services}),
		internalobjectstore.WithClaimer(s.claimer),
		internalobjectstore.WithLogger(internallogger.GetLogger("juju.objectstore")),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	s.suite.objectStoresMutex.Lock()
	defer s.suite.objectStoresMutex.Unlock()
	s.suite.objectStores = append(s.suite.objectStores, store)

	return store, nil
}

type stubMetadataService struct {
	services services.ObjectStoreServices
}

func (s *stubMetadataService) ObjectStore() objectstore.ObjectStoreMetadata {
	return s.services.ObjectStore()
}

type stubWatchableDB struct {
	database.TxnRunner
}

func (stubWatchableDB) Subscribe(...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return nil, nil
}

// These mocks are used in place of real components when creating server config.

type noopLogWriter struct{}

func (noopLogWriter) Log([]corelogger.LogRecord) error { return nil }

func (noopLogWriter) Close() error { return nil }

type noopLogSink struct{}

func (s noopLogSink) GetLogWriter(ctx context.Context, modelUUID coremodel.UUID) (corelogger.LogWriter, error) {
	return &noopLogWriter{}, nil
}

func (s noopLogSink) RemoveLogWriter(modelUUID coremodel.UUID) error {
	return nil
}

func (s noopLogSink) Close() error {
	return nil
}

func (noopLogSink) Log([]corelogger.LogRecord) error { return nil }

type mockAuthenticator struct {
	macaroon.LocalMacaroonAuthenticator
}
