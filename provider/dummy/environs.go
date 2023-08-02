// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	stdcontext "context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub/v2"
	"github.com/juju/retry"
	"github.com/juju/schema"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/auditlog"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	corelease "github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/database/txn"
	domainschema "github.com/juju/juju/domain/schema"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/worker/lease"
	"github.com/juju/juju/worker/multiwatcher"
)

var logger = loggo.GetLogger("juju.provider.dummy")

var transientErrorInjection chan error

const BootstrapInstanceId = "localhost"

var errNotPrepared = errors.New("model is not prepared")

// SampleCloudSpec returns an environscloudspec.CloudSpec that can be used to
// open a dummy Environ.
func SampleCloudSpec() environscloudspec.CloudSpec {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "dummy", "password": "secret"})
	return environscloudspec.CloudSpec{
		Type:             "dummy",
		Name:             "dummy",
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		Region:           "dummy-region",
		StorageEndpoint:  "dummy-storage-endpoint",
		Credential:       &cred,
	}
}

// SampleConfig returns an environment configuration with all required
// attributes set.
func SampleConfig() testing.Attrs {
	return testing.Attrs{
		"type":                      "dummy",
		"name":                      "only",
		"uuid":                      testing.ModelTag.Id(),
		"authorized-keys":           testing.FakeAuthKeys,
		"firewall-mode":             config.FwInstance,
		"secret-backend":            "auto",
		"ssl-hostname-verification": true,
		"development":               false,
		"default-space":             "",
		"secret":                    "pork",
		"controller":                true,
	}
}

// PatchTransientErrorInjectionChannel sets the transientInjectionError
// channel which can be used to inject errors into StartInstance for
// testing purposes
// The injected errors will use the string received on the channel
// and the instance's state will eventually go to error, while the
// received string will appear in the info field of the machine's status
func PatchTransientErrorInjectionChannel(c chan error) func() {
	return jujutesting.PatchValue(&transientErrorInjection, c)
}

// mongoInfo returns a mongo.MongoInfo which allows clients to connect to the
// shared dummy state, if it exists.
func mongoInfo() mongo.MongoInfo {
	if mgotesting.MgoServer.Addr() == "" {
		panic("dummy environ state tests must be run with MgoTestPackage")
	}
	mongoPort := strconv.Itoa(mgotesting.MgoServer.Port())
	addrs := []string{net.JoinHostPort("localhost", mongoPort)}
	return mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      addrs,
			CACert:     testing.CACert,
			DisableTLS: !mgotesting.MgoServer.SSLEnabled(),
		},
	}
}

// Operation represents an action on the dummy provider.
type Operation interface{}

type OpBootstrap struct {
	Context environs.BootstrapContext
	Env     string
	Args    environs.BootstrapParams
}

type OpFinalizeBootstrap struct {
	Context        environs.BootstrapContext
	Env            string
	InstanceConfig *instancecfg.InstanceConfig
}

type OpDestroy struct {
	Env         string
	Cloud       string
	CloudRegion string
	Error       error
}

type OpNetworkInterfaces struct {
	Env        string
	InstanceId instance.Id
	Info       network.InterfaceInfos
}

type OpSubnets struct {
	Env        string
	InstanceId instance.Id
	SubnetIds  []network.Id
	Info       []network.SubnetInfo
}

type OpStartInstance struct {
	Env               string
	MachineId         string
	MachineNonce      string
	PossibleTools     coretools.List
	Instance          instances.Instance
	Constraints       constraints.Value
	SubnetsToZones    map[network.Id][]string
	NetworkInfo       network.InterfaceInfos
	RootDisk          *storage.VolumeParams
	Volumes           []storage.Volume
	VolumeAttachments []storage.VolumeAttachment
	Jobs              []model.MachineJob
	APIInfo           *api.Info
	Secret            string
	AgentEnvironment  map[string]string
}

type OpStopInstances struct {
	Env string
	Ids []instance.Id
}

type OpOpenPorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Rules      firewall.IngressRules
}

type OpClosePorts struct {
	Env        string
	MachineId  string
	InstanceId instance.Id
	Rules      firewall.IngressRules
}

type OpPutFile struct {
	Env      string
	FileName string
}

// environProvider represents the dummy provider.  There is only ever one
// instance of this type (dummy)
type environProvider struct {
	mu                         sync.Mutex
	ops                        chan<- Operation
	newStatePolicy             state.NewPolicyFunc
	supportsRulesWithIPV6CIDRs bool
	supportsSpaces             bool
	supportsSpaceDiscovery     bool
	apiPort                    int
	controllerState            *environState
	state                      map[string]*environState
	db                         changestream.WatchableDB
}

// APIPort returns the random api port used by the given provider instance.
func APIPort(p environs.EnvironProvider) int {
	return p.(*environProvider).apiPort
}

// environState represents the state of an environment.
// It can be shared between several environ values,
// so that a given environment can be opened several times.
type environState struct {
	name           string
	ops            chan<- Operation
	newStatePolicy state.NewPolicyFunc
	mu             sync.Mutex
	maxId          int // maximum instance id allocated so far.
	maxAddr        int // maximum allocated address last byte
	insts          map[instance.Id]*dummyInstance
	globalRules    firewall.IngressRules
	modelRules     firewall.IngressRules
	bootstrapped   bool
	mux            *apiserverhttp.Mux
	httpServer     *httptest.Server
	apiServer      *apiserver.Server
	apiState       *state.State
	apiStatePool   *state.StatePool
	hub            *pubsub.StructuredHub
	presence       *fakePresence
	leaseManager   *lease.Manager
	creator        string

	multiWatcherWorker worker.Worker
}

// environ represents a client's connection to a given environment's
// state.
type environ struct {
	storage.ProviderRegistry
	name         string
	modelUUID    string
	cloud        environscloudspec.CloudSpec
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

var _ environs.Environ = (*environ)(nil)
var _ environs.Networking = (*environ)(nil)

// discardOperations discards all Operations written to it.
var discardOperations = make(chan Operation)

func init() {
	environs.RegisterProvider("dummy", &dummy)

	// Prime the first ops channel, so that naive clients can use
	// the testing environment by simply importing it.
	go func() {
		for range discardOperations {
		}
	}()
}

// dummy is the dummy environmentProvider singleton.
var dummy = environProvider{
	ops:                        discardOperations,
	state:                      make(map[string]*environState),
	newStatePolicy:             stateenvirons.GetNewPolicyFunc(),
	supportsSpaces:             true,
	supportsSpaceDiscovery:     false,
	supportsRulesWithIPV6CIDRs: true,
}

// Reset resets the entire dummy environment and forgets any registered
// operation listener. All opened environments after Reset will share
// the same underlying state.
func Reset(c *gc.C) {
	logger.Infof("reset model")
	dummy.mu.Lock()
	dummy.ops = discardOperations
	oldState := dummy.state
	dummy.controllerState = nil
	if dummy.db != nil {
		dummy.db.(*trackedDB).db.Close()
	}
	dummy.state = make(map[string]*environState)
	dummy.newStatePolicy = stateenvirons.GetNewPolicyFunc()
	dummy.supportsSpaces = true
	dummy.supportsSpaceDiscovery = false
	dummy.supportsRulesWithIPV6CIDRs = true
	dummy.mu.Unlock()

	// NOTE(axw) we must destroy the old states without holding
	// the provider lock, or we risk deadlocking. Destroying
	// state involves closing the embedded API server, which
	// may require waiting on RPC calls that interact with the
	// EnvironProvider (e.g. EnvironProvider.Open).
	for _, s := range oldState {
		if s.httpServer != nil {
			logger.Debugf("closing httpServer")
			s.httpServer.Close()
		}
		s.destroy()
	}
	if mongoAlive() {
		err := retry.Call(retry.CallArgs{
			Func: mgotesting.MgoServer.Reset,
			// Only interested in retrying the intermittent
			// 'unexpected message'.
			IsFatalError: func(err error) bool {
				return !strings.HasSuffix(err.Error(), "unexpected message")
			},
			Delay:    time.Millisecond,
			Clock:    clock.WallClock,
			Attempts: 5,
			NotifyFunc: func(lastError error, attempt int) {
				logger.Infof("retrying MgoServer.Reset() after attempt %d: %v", attempt, lastError)
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *environState) destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyLocked()
}

func (s *environState) destroyLocked() {
	if !s.bootstrapped {
		return
	}
	apiServer := s.apiServer
	apiStatePool := s.apiStatePool
	leaseManager := s.leaseManager
	multiWatcherWorker := s.multiWatcherWorker
	s.apiServer = nil
	s.apiStatePool = nil
	s.apiState = nil
	s.leaseManager = nil
	s.bootstrapped = false
	s.hub = nil
	s.multiWatcherWorker = nil

	// Release the lock while we close resources. In particular,
	// we must not hold the lock while the API server is being
	// closed, as it may need to interact with the Environ while
	// shutting down.
	s.mu.Unlock()
	defer s.mu.Lock()

	if apiServer != nil {
		logger.Debugf("stopping apiServer")
		if err := apiServer.Stop(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if multiWatcherWorker != nil {
		logger.Debugf("stopping multiWatcherWorker worker")
		if err := worker.Stop(multiWatcherWorker); err != nil {
			panic(err)
		}
	}

	if leaseManager != nil {
		if err := worker.Stop(leaseManager); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if apiStatePool != nil {
		logger.Debugf("closing apiStatePool")
		if err := apiStatePool.Close(); err != nil && mongoAlive() {
			panic(err)
		}
	}

	if mongoAlive() {
		logger.Debugf("resetting MgoServer")
		_ = mgotesting.MgoServer.Reset()
	}
}

// mongoAlive reports whether the mongo server is
// still alive (i.e. has not been deliberately destroyed).
// If it has been deliberately destroyed, we will
// expect some errors when closing things down.
func mongoAlive() bool {
	return mgotesting.MgoServer.Addr() != ""
}

// GetStateInAPIServer returns the state connection used by the API server
// This is so code in the test suite can trigger Syncs, etc that the API server
// will see, which will then trigger API watchers, etc.
func (e *environ) GetStateInAPIServer() *state.State {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.apiState
}

// GetStatePoolInAPIServer returns the StatePool used by the API
// server.  As for GetStatePoolInAPIServer, this is so code in the
// test suite can trigger Syncs etc.
func (e *environ) GetStatePoolInAPIServer() *state.StatePool {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.apiStatePool
}

// GetHubInAPIServer returns the central hub used by the API server.
func (e *environ) GetHubInAPIServer() *pubsub.StructuredHub {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.hub
}

// GetLeaseManagerInAPIServer returns the channel used to update the
// cache.Controller used by the API server
func (e *environ) GetLeaseManagerInAPIServer() corelease.Manager {
	st, err := e.state()
	if err != nil {
		panic(err)
	}
	return st.leaseManager
}

// newState creates the state for a new environment with the given name.
func newState(name string, ops chan<- Operation, newStatePolicy state.NewPolicyFunc) *environState {
	buf := make([]byte, 8192)
	buf = buf[:runtime.Stack(buf, false)]
	s := &environState{
		name:           name,
		ops:            ops,
		newStatePolicy: newStatePolicy,
		insts:          make(map[instance.Id]*dummyInstance),
		modelRules: firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("22"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("17777"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
		creator: string(buf),
	}
	return s
}

// listenAPI starts an HTTP server listening for API connections.
func (s *environState) listenAPI() int {
	certPool, err := api.CreateCertPool(testing.CACert)
	if err != nil {
		panic(err)
	}
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*testing.ServerTLSCert}
	s.mux = apiserverhttp.NewMux()
	s.httpServer = httptest.NewUnstartedServer(s.mux)
	s.httpServer.TLS = tlsConfig
	return s.httpServer.Listener.Addr().(*net.TCPAddr).Port
}

// SetSupportsSpaces allows to enable and disable SupportsSpaces for tests.
func SetSupportsSpaces(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaces
	dummy.supportsSpaces = supports
	return current
}

// SetSupportsRulesWithIPV6CIDRs allows to toggle support for IPV6 CIDRs in firewall rules.
func SetSupportsRulesWithIPV6CIDRs(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsRulesWithIPV6CIDRs
	dummy.supportsRulesWithIPV6CIDRs = supports
	return current
}

// SetSupportsSpaceDiscovery allows to enable and disable
// SupportsSpaceDiscovery for tests.
func SetSupportsSpaceDiscovery(supports bool) bool {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	current := dummy.supportsSpaceDiscovery
	dummy.supportsSpaceDiscovery = supports
	return current
}

// Listen directs subsequent operations on any dummy environment
// to channel c (if not nil).
func Listen(c chan<- Operation) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if c == nil {
		c = discardOperations
	}
	dummy.ops = c
	for _, st := range dummy.state {
		st.mu.Lock()
		st.ops = c
		st.mu.Unlock()
	}
}

var configSchema = environschema.Fields{
	"controller": {
		Description: "Whether the model should start a controller",
		Type:        environschema.Tbool,
	},
	"broken": {
		Description: "Whitespace-separated Environ methods that should return an error when called",
		Type:        environschema.Tstring,
	},
	"secret": {
		Description: "A secret",
		Type:        environschema.Tstring,
	},
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{
	"broken":     "",
	"secret":     "pork",
	"controller": false,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) controller() bool {
	return c.attrs["controller"].(bool)
}

func (c *environConfig) broken() string {
	return c.attrs["broken"].(string)
}

func (c *environConfig) secret() string {
	return c.attrs["secret"].(string)
}

func (p *environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (p *environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

var _ config.ConfigSchemaSource = (*environProvider)(nil)

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p *environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p *environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (*environProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.EmptyAuthType: {},
		cloud.UserPassAuthType: {
			{
				Name: "username", CredentialAttr: cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				Name: "password", CredentialAttr: cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			},
		},
	}
}

func (*environProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

func (*environProvider) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}

func (*environProvider) DetectRegions() ([]cloud.Region, error) {
	return []cloud.Region{{Name: "dummy"}}, nil
}

func (p *environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	// Apply the coerced unknown values back into the config.
	return cfg.Apply(validated)
}

func (e *environ) state() (*environState, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	state, ok := dummy.state[e.modelUUID]
	if !ok {
		return nil, errNotPrepared
	}
	return state, nil
}

// Version is part of the EnvironProvider interface.
func (*environProvider) Version() int {
	return 0
}

func (p *environProvider) Open(_ stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ecfg, err := p.newConfig(args.Config)
	if err != nil {
		return nil, err
	}
	env := &environ{
		ProviderRegistry: StorageProviders(),
		name:             ecfg.Name(),
		modelUUID:        args.Config.UUID(),
		cloud:            args.Cloud,
		ecfgUnlocked:     ecfg,
	}
	if err := env.checkBroken("Open"); err != nil {
		return nil, err
	}
	return env, nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p *environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p *environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p *environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if _, err := dummy.newConfig(args.Config); err != nil {
		return nil, err
	}
	return args.Config, nil
}

// Override for testing - the data directory with which the state api server is initialised.
var DataDir = ""
var LogDir = ""

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) checkBroken(method string) error {
	for _, m := range strings.Fields(e.ecfg().broken()) {
		if m == method {
			return fmt.Errorf("dummy.%s is broken", method)
		}
	}
	return nil
}

// PrecheckInstance is specified in the environs.InstancePrechecker interface.
func (*environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if args.Placement != "" && args.Placement != "valid" {
		return fmt.Errorf("%s placement is invalid", args.Placement)
	}
	return nil
}

// Create is part of the Environ interface.
func (e *environ) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	dummy.state[e.modelUUID] = newState(e.name, dummy.ops, dummy.newStatePolicy)
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	ecfg := e.ecfgUnlocked

	if ecfg.controller() && dummy.controllerState != nil {
		// Because of global variables, we can only have one dummy
		// controller per process. Panic if there is an attempt to
		// bootstrap while there is another active controller.
		old := dummy.controllerState
		panic(fmt.Errorf("cannot share a state between two dummy environs; old %q; new %q: %s", old.name, e.name, old.creator))
	}

	// The environment has not been prepared, so create it and record it.
	// We don't start listening for State or API connections until
	// Bootstrap has been called.
	envState := newState(e.name, dummy.ops, dummy.newStatePolicy)
	if ecfg.controller() {
		dummy.apiPort = envState.listenAPI()
		dummy.controllerState = envState
	}
	dummy.state[e.modelUUID] = envState

	// Create a new sqlite3 database for the environment.
	db, err := e.newCleanDB()
	if err != nil {
		return errors.Trace(err)
	}
	dummy.db = db

	return nil
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	availableTools, err := args.AvailableTools.Match(coretools.Filter{OSType: "ubuntu"})
	if err != nil {
		return nil, err
	}
	arch, err := availableTools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}

	defer delay()
	if err := e.checkBroken("Bootstrap"); err != nil {
		return nil, err
	}
	if _, ok := args.ControllerConfig.CACert(); !ok {
		return nil, errors.New("no CA certificate in controller configuration")
	}

	logger.Infof("would pick agent binaries from %s", availableTools)

	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if estate.bootstrapped {
		return nil, errors.New("model is already bootstrapped")
	}

	// Create an instance for the bootstrap node.
	logger.Infof("creating bootstrap instance")
	i := &dummyInstance{
		id:           BootstrapInstanceId,
		addresses:    network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(),
		machineId:    agent.BootstrapControllerId,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
		controller:   true,
	}
	estate.insts[i.id] = i
	estate.bootstrapped = true
	estate.ops <- OpBootstrap{Context: ctx, Env: e.name, Args: args}

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) (err error) {
		if e.ecfg().controller() {
			icfg.Bootstrap.BootstrapMachineInstanceId = BootstrapInstanceId
			if err := instancecfg.FinishInstanceConfig(icfg, e.Config()); err != nil {
				return err
			}

			adminUser := names.NewUserTag("admin@local")
			var cloudCredentialTag names.CloudCredentialTag
			if icfg.Bootstrap.ControllerCloudCredentialName != "" {
				id := fmt.Sprintf(
					"%s/%s/%s",
					icfg.Bootstrap.ControllerCloud.Name,
					adminUser.Id(),
					icfg.Bootstrap.ControllerCloudCredentialName,
				)
				if !names.IsValidCloudCredential(id) {
					return errors.NotValidf("cloud credential ID %q", id)
				}
				cloudCredentialTag = names.NewCloudCredentialTag(id)
			}

			cloudCredentials := make(map[names.CloudCredentialTag]cloud.Credential)
			if icfg.Bootstrap.ControllerCloudCredential != nil && icfg.Bootstrap.ControllerCloudCredentialName != "" {
				cloudCredentials[cloudCredentialTag] = *icfg.Bootstrap.ControllerCloudCredential
			}

			session, err := mongo.DialWithInfo(mongoInfo(), mongotest.DialOpts())
			if err != nil {
				return err
			}
			defer session.Close()

			// Since the admin user isn't setup until after here,
			// the password in the info structure is empty, so the admin
			// user is constructed with an empty password here.
			// It is set just below.
			controller, err := state.Initialize(state.InitializeParams{
				Clock:            clock.WallClock,
				ControllerConfig: icfg.ControllerConfig,
				ControllerModelArgs: state.ModelArgs{
					Type:                    state.ModelTypeIAAS,
					Owner:                   adminUser,
					Config:                  icfg.Bootstrap.ControllerModelConfig,
					Constraints:             icfg.Bootstrap.BootstrapMachineConstraints,
					CloudName:               icfg.Bootstrap.ControllerCloud.Name,
					CloudRegion:             icfg.Bootstrap.ControllerCloudRegion,
					CloudCredential:         cloudCredentialTag,
					StorageProviderRegistry: e,
				},
				Cloud:            icfg.Bootstrap.ControllerCloud,
				CloudCredentials: cloudCredentials,
				MongoSession:     session,
				NewPolicy:        estate.newStatePolicy,
				AdminPassword:    icfg.APIInfo.Password,
			})
			if err != nil {
				return err
			}
			st, err := controller.SystemState()
			if err != nil {
				return err
			}
			defer func() {
				if err != nil {
					controller.Close()
				}
			}()
			if err := st.SetModelConstraints(args.ModelConstraints); err != nil {
				return errors.Trace(err)
			}
			if err := st.SetAdminMongoPassword(icfg.APIInfo.Password); err != nil {
				return errors.Trace(err)
			}
			if err := st.MongoSession().DB("admin").Login("admin", icfg.APIInfo.Password); err != nil {
				return err
			}
			env, err := st.Model()
			if err != nil {
				return errors.Trace(err)
			}
			owner, err := st.User(env.Owner())
			if err != nil {
				return errors.Trace(err)
			}
			// We log this out for test purposes only. No one in real life can use
			// a dummy provider for anything other than testing, so logging the password
			// here is fine.
			logger.Debugf("setting password for %q to %q", owner.Name(), icfg.APIInfo.Password)
			_ = owner.SetPassword(icfg.APIInfo.Password)
			statePool := controller.StatePool()
			stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, clock.WallClock)
			if err != nil {
				return errors.Trace(err)
			}
			errH := stateAuthenticator.AddHandlers(estate.mux)
			if errH != nil {
				return errors.Trace(errH)
			}

			machineTag := names.NewMachineTag("0")
			estate.httpServer.StartTLS()
			estate.presence = &fakePresence{make(map[string]presence.Status)}
			estate.hub = centralhub.New(machineTag, centralhub.PubsubNoOpMetrics{})

			estate.leaseManager, err = leaseManager(icfg.ControllerConfig.ControllerUUID())
			if err != nil {
				return errors.Trace(err)
			}

			allWatcherBacking, err := state.NewAllWatcherBacking(statePool)
			if err != nil {
				return errors.Trace(err)
			}
			multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
				Clock:                clock.WallClock,
				Logger:               loggo.GetLogger("dummy.multiwatcher"),
				Backing:              allWatcherBacking,
				PrometheusRegisterer: noopRegisterer{},
			})
			if err != nil {
				return errors.Trace(err)
			}
			estate.multiWatcherWorker = multiWatcherWorker

			dummy.mu.Lock()
			db := dummy.db
			dummy.mu.Unlock()

			estate.apiServer, err = apiserver.NewServer(apiserver.ServerConfig{
				StatePool:                  statePool,
				MultiwatcherFactory:        multiWatcherWorker,
				LocalMacaroonAuthenticator: stateAuthenticator,
				Clock:                      clock.WallClock,
				GetAuditConfig:             func() auditlog.Config { return auditlog.Config{} },
				Tag:                        machineTag,
				DataDir:                    DataDir,
				LogDir:                     LogDir,
				Mux:                        estate.mux,
				Hub:                        estate.hub,
				Presence:                   estate.presence,
				LeaseManager:               estate.leaseManager,
				NewObserver: func() observer.Observer {
					logger := loggo.GetLogger("juju.apiserver")
					ctx := observer.RequestObserverContext{
						Clock:  clock.WallClock,
						Logger: logger,
						Hub:    estate.hub,
					}
					return observer.NewRequestObserver(ctx)
				},
				PublicDNSName: icfg.ControllerConfig.AutocertDNSName(),
				UpgradeComplete: func() bool {
					return true
				},
				MetricsCollector: apiserver.NewMetricsCollector(),
				SysLogger:        noopSysLogger{},
				DBGetter:         stubDBGetter{db: db},
				DBDeleter:        stubDBDeleter{},
			})
			if err != nil {
				panic(err)
			}
			estate.apiState = st
			estate.apiStatePool = statePool

			// Maintain the state authenticator (time out local user interactions).
			abort := make(chan struct{})
			go stateAuthenticator.Maintain(abort)
			go func(apiServer *apiserver.Server) {
				defer func() {
					close(abort)
				}()
				_ = apiServer.Wait()
			}(estate.apiServer)
		}
		estate.ops <- OpFinalizeBootstrap{Context: ctx, Env: e.name, InstanceConfig: icfg}
		return nil
	}

	bsResult := &environs.BootstrapResult{
		Arch:                    arch,
		Base:                    corebase.MakeDefaultBase("ubuntu", "22.04"),
		CloudBootstrapFinalizer: finalize,
	}
	return bsResult, nil
}

type noopSysLogger struct{}

func (noopSysLogger) Log([]corelogger.LogRecord) error { return nil }

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
	if namespace == "controller" {
		return errors.Forbiddenf(`cannot delete "controller" DB`)
	}
	return nil
}

func leaseManager(controllerUUID string) (*lease.Manager, error) {
	dummyStore := newLeaseStore(clock.WallClock)
	return lease.NewManager(lease.ManagerConfig{
		Secretary:            lease.SecretaryFinder(controllerUUID),
		Store:                dummyStore,
		Logger:               loggo.GetLogger("juju.worker.lease.dummy"),
		Clock:                clock.WallClock,
		MaxSleep:             time.Minute,
		EntityUUID:           controllerUUID,
		PrometheusRegisterer: noopRegisterer{},
	})
}

func (e *environ) ControllerInstances(context.ProviderCallContext, string) ([]instance.Id, error) {
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	if err := e.checkBroken("ControllerInstances"); err != nil {
		return nil, err
	}
	if !estate.bootstrapped {
		return nil, environs.ErrNotBootstrapped
	}
	var controllerInstances []instance.Id
	for _, v := range estate.insts {
		if v.controller {
			controllerInstances = append(controllerInstances, v.Id())
		}
	}
	return controllerInstances, nil
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) SetConfig(cfg *config.Config) error {
	if err := e.checkBroken("SetConfig"); err != nil {
		return err
	}
	ecfg, err := dummy.newConfig(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	e.ecfgUnlocked = ecfg
	e.ecfgMutex.Unlock()
	return nil
}

// AdoptResources is part of the Environ interface.
func (e *environ) AdoptResources(context.ProviderCallContext, string, version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

func (e *environ) Destroy(context.ProviderCallContext) (res error) {
	defer delay()
	estate, err := e.state()
	if err != nil {
		if err == errNotPrepared {
			return nil
		}
		return err
	}
	defer func() {
		// The estate is a pointer to a structure that is stored in the dummy global.
		// The Listen method can change the ops channel of any state, and will do so
		// under the covers. What we need to do is use the state mutex to add a memory
		// barrier such that the ops channel we see here is the latest.
		estate.mu.Lock()
		ops := estate.ops
		name := estate.name
		delete(dummy.state, e.modelUUID)
		estate.mu.Unlock()
		ops <- OpDestroy{
			Env:         name,
			Cloud:       e.cloud.Name,
			CloudRegion: e.cloud.Region,
			Error:       res,
		}
	}()
	if err := e.checkBroken("Destroy"); err != nil {
		return err
	}
	if !e.ecfg().controller() {
		return nil
	}
	estate.destroy()
	return nil
}

func (e *environ) DestroyController(ctx context.ProviderCallContext, _ string) error {
	if err := e.Destroy(ctx); err != nil {
		return err
	}
	dummy.mu.Lock()
	dummy.controllerState = nil
	dummy.mu.Unlock()
	return nil
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.VirtType,
	constraints.ImageID,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator(context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
	validator.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64, arch.PPC64EL, arch.S390X, arch.RISCV64})
	return validator, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(_ context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	defer delay()
	machineId := args.InstanceConfig.MachineId
	logger.Infof("dummy startinstance, machine %s", machineId)
	if err := e.checkBroken("StartInstance"); err != nil {
		return nil, err
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// check if an error has been injected on the transientErrorInjection channel (testing purposes)
	select {
	case injectedError := <-transientErrorInjection:
		return nil, injectedError
	default:
	}

	if args.InstanceConfig.MachineNonce == "" {
		return nil, errors.New("cannot start instance: missing machine nonce")
	}
	if args.InstanceConfig.IsController() {
		if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
			return nil, errors.New("entity tag must match started machine")
		}
	}
	if args.InstanceConfig.APIInfo.Tag != names.NewMachineTag(machineId) {
		return nil, errors.New("entity tag must match started machine")
	}
	logger.Infof("would pick agent binaries from %s", args.Tools)

	idString := fmt.Sprintf("%s-%d", e.name, estate.maxId)
	// Add the addresses we want to see in the machine doc. This means both
	// IPv4 and IPv6 loopback, as well as the DNS name.
	addrs := network.NewMachineAddresses([]string{idString + ".dns", "127.0.0.1", "::1"}).AsProviderAddresses()
	logger.Debugf("StartInstance addresses: %v", addrs)
	i := &dummyInstance{
		id:           instance.Id(idString),
		addresses:    addrs,
		machineId:    machineId,
		firewallMode: e.Config().FirewallMode(),
		state:        estate,
	}

	var hc *instance.HardwareCharacteristics
	// To match current system capability, only provide hardware characteristics for
	// environ machines, not containers.
	if container.ParentId(machineId) == "" {
		// Assume that the provided Availability Zone won't fail,
		// though one is required.
		var zone string
		if args.Placement != "" {
			split := strings.Split(args.Placement, "=")
			if len(split) == 2 && split[0] == "zone" {
				zone = split[1]
			}
		}
		if zone == "" && args.AvailabilityZone != "" {
			zone = args.AvailabilityZone
		}

		// We will just assume the instance hardware characteristics exactly matches
		// the supplied constraints (if specified).
		hc = &instance.HardwareCharacteristics{
			Arch:             args.Constraints.Arch,
			Mem:              args.Constraints.Mem,
			RootDisk:         args.Constraints.RootDisk,
			CpuCores:         args.Constraints.CpuCores,
			CpuPower:         args.Constraints.CpuPower,
			Tags:             args.Constraints.Tags,
			AvailabilityZone: &zone,
		}
		// Fill in some expected instance hardware characteristics if constraints not specified.
		if hc.Arch == nil {
			defaultArch := arch.DefaultArchitecture
			hc.Arch = &defaultArch
		}
		if hc.Mem == nil {
			mem := uint64(1024)
			hc.Mem = &mem
		}
		if hc.RootDisk == nil {
			disk := uint64(8192)
			hc.RootDisk = &disk
		}
		if hc.CpuCores == nil {
			cores := uint64(1)
			hc.CpuCores = &cores
		}
	}
	// Simulate subnetsToZones gets populated when spaces given in constraints.
	spaces := args.Constraints.IncludeSpaces()
	var subnetsToZones map[network.Id][]string
	for isp := range spaces {
		// Simulate 2 subnets per space.
		if subnetsToZones == nil {
			subnetsToZones = make(map[network.Id][]string)
		}
		for isn := 0; isn < 2; isn++ {
			providerId := fmt.Sprintf("subnet-%d", isp+isn)
			zone := fmt.Sprintf("zone%d", isp+isn)
			subnetsToZones[network.Id(providerId)] = []string{zone}
		}
	}
	// Simulate creating volumes when requested.
	volumes := make([]storage.Volume, len(args.Volumes))
	for iv, v := range args.Volumes {
		persistent, _ := v.Attributes["persistent"].(bool)
		volumes[iv] = storage.Volume{
			Tag: v.Tag,
			VolumeInfo: storage.VolumeInfo{
				Size:       v.Size,
				Persistent: persistent,
			},
		}
	}
	// Simulate attaching volumes when requested.
	volumeAttachments := make([]storage.VolumeAttachment, len(args.VolumeAttachments))
	for iv, v := range args.VolumeAttachments {
		volumeAttachments[iv] = storage.VolumeAttachment{
			Volume:  v.Volume,
			Machine: v.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceName: fmt.Sprintf("sd%c", 'b'+rune(iv)),
				ReadOnly:   v.ReadOnly,
			},
		}
	}
	estate.insts[i.id] = i
	estate.maxId++
	estate.ops <- OpStartInstance{
		Env:               e.name,
		MachineId:         machineId,
		MachineNonce:      args.InstanceConfig.MachineNonce,
		PossibleTools:     args.Tools,
		Constraints:       args.Constraints,
		SubnetsToZones:    subnetsToZones,
		RootDisk:          args.RootDisk,
		Volumes:           volumes,
		VolumeAttachments: volumeAttachments,
		Instance:          i,
		Jobs:              args.InstanceConfig.Jobs,
		APIInfo:           args.InstanceConfig.APIInfo,
		AgentEnvironment:  args.InstanceConfig.AgentEnvironment,
		Secret:            e.ecfg().secret(),
	}
	return &environs.StartInstanceResult{
		Instance: i,
		Hardware: hc,
	}, nil
}

func (e *environ) StopInstances(_ context.ProviderCallContext, ids ...instance.Id) error {
	defer delay()
	if err := e.checkBroken("StopInstance"); err != nil {
		return err
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, id := range ids {
		delete(estate.insts, id)
	}
	estate.ops <- OpStopInstances{
		Env: e.name,
		Ids: ids,
	}
	return nil
}

func (e *environ) Instances(_ context.ProviderCallContext, ids []instance.Id) (insts []instances.Instance, err error) {
	defer delay()
	if err := e.checkBroken("Instances"); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	notFound := 0
	for _, id := range ids {
		inst := estate.insts[id]
		if inst == nil {
			err = environs.ErrPartialInstances
			notFound++
			insts = append(insts, nil)
		} else {
			insts = append(insts, inst)
		}
	}
	if notFound == len(ids) {
		return nil, environs.ErrNoInstances
	}
	return
}

// SupportsSpaces is specified on environs.Networking.
func (env *environ) SupportsSpaces(_ context.ProviderCallContext) (bool, error) {
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaces {
		return false, errors.NotSupportedf("spaces")
	}
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *environ) SupportsSpaceDiscovery(_ context.ProviderCallContext) (bool, error) {
	if err := env.checkBroken("SupportsSpaceDiscovery"); err != nil {
		return false, err
	}
	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	if !dummy.supportsSpaceDiscovery {
		return false, nil
	}
	return true, nil
}

// SupportsContainerAddresses is specified on environs.Networking.
func (env *environ) SupportsContainerAddresses(_ context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("container addresses")
}

// Spaces is specified on environs.Networking.
func (env *environ) Spaces(_ context.ProviderCallContext) (network.SpaceInfos, error) {
	if err := env.checkBroken("Spaces"); err != nil {
		return []network.SpaceInfo{}, err
	}
	return []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("0"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("1"),
			AvailabilityZones: []string{"zone1"},
		}, {
			ProviderId:        network.Id("2"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "Another Foo 99!",
		ProviderId: "1",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-",
		ProviderId: "2",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "---",
		ProviderId: "3",
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("5"),
			AvailabilityZones: []string{"zone1"},
		}}}}, nil
}

// NetworkInterfaces implements Environ.NetworkInterfaces().
func (env *environ) NetworkInterfaces(_ context.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {
	if err := env.checkBroken("NetworkInterfaces"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	// Simulate 3 NICs - primary and secondary enabled plus a disabled NIC.
	// all configured using DHCP and having fake DNS servers and gateway.
	infos := make([]network.InterfaceInfos, len(ids))
	for idIndex, instId := range ids {
		infos[idIndex] = make(network.InterfaceInfos, 3)
		for i, netName := range []string{"private", "public", "disabled"} {
			infos[idIndex][i] = network.InterfaceInfo{
				DeviceIndex:      i,
				ProviderId:       network.Id(fmt.Sprintf("dummy-eth%d", i)),
				ProviderSubnetId: network.Id("dummy-" + netName),
				InterfaceType:    network.EthernetDevice,
				InterfaceName:    fmt.Sprintf("eth%d", i),
				VLANTag:          i,
				MACAddress:       fmt.Sprintf("aa:bb:cc:dd:ee:f%d", i),
				Disabled:         i == 2,
				NoAutoStart:      i%2 != 0,
				Addresses: network.ProviderAddresses{
					network.NewMachineAddress(
						fmt.Sprintf("0.%d.0.%d", (i+1)*10+idIndex, estate.maxAddr+2),
						network.WithCIDR(fmt.Sprintf("0.%d.0.0/24", (i+1)*10)),
						network.WithConfigType(network.ConfigDHCP),
					).AsProviderAddress(),
				},
				DNSServers: network.NewMachineAddresses([]string{"ns1.dummy", "ns2.dummy"}).AsProviderAddresses(),
				GatewayAddress: network.NewMachineAddress(
					fmt.Sprintf("0.%d.0.1", (i+1)*10+idIndex),
				).AsProviderAddress(),
				Origin: network.OriginProvider,
			}
		}

		estate.ops <- OpNetworkInterfaces{
			Env:        env.name,
			InstanceId: instId,
			Info:       infos[idIndex],
		}
	}

	return infos, nil
}

type azShim struct {
	name      string
	available bool
}

func (az azShim) Name() string {
	return az.name
}

func (az azShim) Available() bool {
	return az.available
}

// AvailabilityZones implements environs.ZonedEnviron.
func (env *environ) AvailabilityZones(context.ProviderCallContext) (network.AvailabilityZones, error) {
	return network.AvailabilityZones{
		azShim{"zone1", true},
		azShim{"zone2", false},
		azShim{"zone3", true},
		azShim{"zone4", true},
	}, nil
}

// InstanceAvailabilityZoneNames implements environs.ZonedEnviron.
func (env *environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	if err := env.checkBroken("InstanceAvailabilityZoneNames"); err != nil {
		return nil, errors.NotSupportedf("instance availability zones")
	}
	availabilityZones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, err
	}
	azMaxIndex := len(availabilityZones) - 1
	azIndex := 0
	returnValue := make(map[instance.Id]string, 0)
	for _, id := range ids {
		if availabilityZones[azIndex].Available() {
			returnValue[id] = availabilityZones[azIndex].Name()
		} else {
			// Based on knowledge of how the AZs are set up above
			// in AvailabilityZones()
			azIndex++
			returnValue[id] = availabilityZones[azIndex].Name()
		}
		azIndex++
		if azIndex == azMaxIndex {
			azIndex = 0
		}
	}
	return returnValue, nil
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (env *environ) DeriveAvailabilityZones(context.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
	return nil, nil
}

// Subnets implements environs.Environ.Subnets.
func (env *environ) Subnets(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []network.Id,
) ([]network.SubnetInfo, error) {
	if err := env.checkBroken("Subnets"); err != nil {
		return nil, err
	}

	estate, err := env.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()

	if ok, _ := env.SupportsSpaceDiscovery(ctx); ok {
		// Space discovery needs more subnets to work with.
		return env.subnetsForSpaceDiscovery(estate)
	}

	allSubnets := []network.SubnetInfo{{
		CIDR:              "0.10.0.0/24",
		ProviderId:        "dummy-private",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		CIDR:       "0.20.0.0/24",
		ProviderId: "dummy-public",
	}}

	// Filter result by ids, if given.
	var result []network.SubnetInfo
	for _, subId := range subnetIds {
		switch subId {
		case "dummy-private":
			result = append(result, allSubnets[0])
		case "dummy-public":
			result = append(result, allSubnets[1])
		}
	}
	if len(subnetIds) == 0 {
		result = append([]network.SubnetInfo{}, allSubnets...)
	}
	if len(result) == 0 {
		// No results, so just return them now.
		estate.ops <- OpSubnets{
			Env:        env.name,
			InstanceId: instId,
			SubnetIds:  subnetIds,
			Info:       result,
		}
		return result, nil
	}

	estate.ops <- OpSubnets{
		Env:        env.name,
		InstanceId: instId,
		SubnetIds:  subnetIds,
		Info:       result,
	}
	return result, nil
}

func (env *environ) subnetsForSpaceDiscovery(estate *environState) ([]network.SubnetInfo, error) {
	result := []network.SubnetInfo{{
		ProviderId:        network.Id("1"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.1.0/24",
	}, {
		ProviderId:        network.Id("2"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.2.0/24",
		VLANTag:           1,
	}, {
		ProviderId:        network.Id("3"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.3.0/24",
	}, {
		ProviderId:        network.Id("4"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.4.0/24",
	}, {
		ProviderId:        network.Id("5"),
		AvailabilityZones: []string{"zone1"},
		CIDR:              "192.168.5.0/24",
	}}
	estate.ops <- OpSubnets{
		Env:        env.name,
		InstanceId: instance.UnknownId,
		SubnetIds:  []network.Id{},
		Info:       result,
	}
	return result, nil
}

func (e *environ) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllInstances")
}

func (e *environ) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return e.instancesForMethod(ctx, "AllRunningInstances")
}

func (e *environ) instancesForMethod(_ context.ProviderCallContext, method string) ([]instances.Instance, error) {
	defer delay()
	if err := e.checkBroken(method); err != nil {
		return nil, err
	}
	var insts []instances.Instance
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, v := range estate.insts {
		insts = append(insts, v)
	}
	return insts, nil
}

func (e *environ) OpenPorts(_ context.ProviderCallContext, rules firewall.IngressRules) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		if len(r.SourceCIDRs) == 0 {
			r.SourceCIDRs.Add(firewall.AllNetworksIPV4CIDR)
			r.SourceCIDRs.Add(firewall.AllNetworksIPV6CIDR)
		}
		found := false
		for _, rule := range estate.globalRules {
			if r.String() == rule.String() {
				found = true
			}
		}
		if !found {
			estate.globalRules = append(estate.globalRules, r)
		}
	}

	return nil
}

func (e *environ) ClosePorts(_ context.ProviderCallContext, rules firewall.IngressRules) error {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		for i, rule := range estate.globalRules {
			if len(r.SourceCIDRs) == 0 {
				r.SourceCIDRs.Add(firewall.AllNetworksIPV4CIDR)
				r.SourceCIDRs.Add(firewall.AllNetworksIPV6CIDR)
			}
			if r.String() == rule.String() {
				estate.globalRules = estate.globalRules[:i+copy(estate.globalRules[i:], estate.globalRules[i+1:])]
			}
		}
	}
	return nil
}

func (e *environ) IngressRules(context.ProviderCallContext) (rules firewall.IngressRules, err error) {
	if mode := e.ecfg().FirewallMode(); mode != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from model", mode)
	}
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range estate.globalRules {
		rules = append(rules, r)
	}
	rules.Sort()
	return
}

func (e *environ) OpenModelPorts(_ context.ProviderCallContext, rules firewall.IngressRules) error {
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		if len(r.SourceCIDRs) == 0 {
			r.SourceCIDRs.Add(firewall.AllNetworksIPV4CIDR)
			r.SourceCIDRs.Add(firewall.AllNetworksIPV6CIDR)
		}
		found := false
		for _, rule := range estate.modelRules {
			if r.String() == rule.String() {
				found = true
			}
		}
		if !found {
			estate.modelRules = append(estate.modelRules, r)
		}
	}

	return nil
}

func (e *environ) CloseModelPorts(_ context.ProviderCallContext, rules firewall.IngressRules) error {
	estate, err := e.state()
	if err != nil {
		return err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range rules {
		for i, rule := range estate.modelRules {
			if len(r.SourceCIDRs) == 0 {
				r.SourceCIDRs.Add(firewall.AllNetworksIPV4CIDR)
				r.SourceCIDRs.Add(firewall.AllNetworksIPV6CIDR)
			}
			if r.String() == rule.String() {
				estate.modelRules = estate.modelRules[:i+copy(estate.modelRules[i:], estate.modelRules[i+1:])]
			}
		}
	}
	return nil
}

func (e *environ) ModelIngressRules(context.ProviderCallContext) (rules firewall.IngressRules, err error) {
	estate, err := e.state()
	if err != nil {
		return nil, err
	}
	estate.mu.Lock()
	defer estate.mu.Unlock()
	for _, r := range estate.modelRules {
		rules = append(rules, r)
	}
	rules.Sort()
	return
}

// SupportsRulesWithIPV6CIDRs returns true if the environment supports ingress
// rules containing IPV6 CIDRs. It is part of the FirewallFeatureQuerier
// interface.
func (e *environ) SupportsRulesWithIPV6CIDRs(context.ProviderCallContext) (bool, error) {
	if err := e.checkBroken("SupportsRulesWithIPV6CIDRs"); err != nil {
		return false, err
	}

	dummy.mu.Lock()
	defer dummy.mu.Unlock()
	return dummy.supportsRulesWithIPV6CIDRs, nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &dummy
}

type dummyInstance struct {
	state        *environState
	rules        firewall.IngressRules
	id           instance.Id
	status       string
	machineId    string
	firewallMode string
	controller   bool

	mu        sync.Mutex
	addresses []network.ProviderAddress
	broken    []string
}

func (inst *dummyInstance) Id() instance.Id {
	return inst.id
}

func (inst *dummyInstance) Status(context.ProviderCallContext) instance.Status {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	// TODO(perrito666) add a provider status -> juju status mapping.
	jujuStatus := status.Pending
	if inst.status != "" {
		dummyStatus := status.Status(inst.status)
		if dummyStatus.KnownInstanceStatus() {
			jujuStatus = dummyStatus
		}
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: inst.status,
	}

}

// SetInstanceAddresses sets the addresses associated with the given
// dummy instance.
func SetInstanceAddresses(inst instances.Instance, addrs []network.ProviderAddress) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.addresses = append(inst0.addresses[:0], addrs...)
	logger.Debugf("setting instance %q addresses to %v", inst0.Id(), addrs)
	inst0.mu.Unlock()
}

// SetInstanceStatus sets the status associated with the given
// dummy instance.
func SetInstanceStatus(inst instances.Instance, status string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.status = status
	inst0.mu.Unlock()
}

// SetInstanceBroken marks the named methods of the instance as broken.
// Any previously broken methods not in the set will no longer be broken.
func SetInstanceBroken(inst instances.Instance, methods ...string) {
	inst0 := inst.(*dummyInstance)
	inst0.mu.Lock()
	inst0.broken = methods
	inst0.mu.Unlock()
}

func (inst *dummyInstance) checkBroken(method string) error {
	for _, m := range inst.broken {
		if m == method {
			return fmt.Errorf("dummyInstance.%s is broken", method)
		}
	}
	return nil
}

func (inst *dummyInstance) Addresses(context.ProviderCallContext) (network.ProviderAddresses, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if err := inst.checkBroken("Addresses"); err != nil {
		return nil, err
	}
	return append([]network.ProviderAddress{}, inst.addresses...), nil
}

func (inst *dummyInstance) OpenPorts(_ context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	defer delay()
	logger.Infof("openPorts %s, %#v", machineId, rules)
	if inst.firewallMode != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("OpenPorts with mismatched machine id, expected %q got %q", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("OpenPorts"); err != nil {
		return err
	}
	inst.state.ops <- OpOpenPorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Rules:      rules,
	}
	for _, newRule := range rules {
		if len(newRule.SourceCIDRs) == 0 {
			newRule.SourceCIDRs.Add(firewall.AllNetworksIPV4CIDR)
			newRule.SourceCIDRs.Add(firewall.AllNetworksIPV6CIDR)
		}
		found := false

		for i, existingRule := range inst.rules {
			if newRule.PortRange != existingRule.PortRange {
				continue
			}

			// Append CIDRs from incoming rule
			inst.rules[i].SourceCIDRs = existingRule.SourceCIDRs.Union(newRule.SourceCIDRs)
			found = true
			break
		}

		if !found {
			inst.rules = append(inst.rules, newRule)
		}
	}
	return nil
}

func (inst *dummyInstance) ClosePorts(_ context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	defer delay()
	if inst.firewallMode != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("ClosePorts with mismatched machine id, expected %s got %s", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("ClosePorts"); err != nil {
		return err
	}
	inst.state.ops <- OpClosePorts{
		Env:        inst.state.name,
		MachineId:  machineId,
		InstanceId: inst.Id(),
		Rules:      rules,
	}

	var updatedRules firewall.IngressRules

nextRule:
	for _, existingRule := range inst.rules {
		for _, removeRule := range rules {
			if removeRule.PortRange != existingRule.PortRange {
				continue // port not matched
			}

			existingRule.SourceCIDRs = existingRule.SourceCIDRs.Difference(removeRule.SourceCIDRs)

			// If the rule is empty, OR the entry to be removed
			// has no CIDRs, drop the rule.
			if len(existingRule.SourceCIDRs) == 0 || len(removeRule.SourceCIDRs) == 0 {
				continue nextRule // drop existing rule
			}
		}

		updatedRules = append(updatedRules, existingRule)
	}
	inst.rules = updatedRules
	return nil
}

func (inst *dummyInstance) IngressRules(_ context.ProviderCallContext, machineId string) (rules firewall.IngressRules, err error) {
	defer delay()
	if inst.firewallMode != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ingress rules from instance",
			inst.firewallMode)
	}
	if inst.machineId != machineId {
		panic(fmt.Errorf("Rules with mismatched machine id, expected %q got %q", inst.machineId, machineId))
	}
	inst.state.mu.Lock()
	defer inst.state.mu.Unlock()
	if err := inst.checkBroken("IngressRules"); err != nil {
		return nil, err
	}
	for _, r := range inst.rules {
		rules = append(rules, r)
	}
	rules.Sort()
	return
}

// providerDelay controls the delay before dummy responds.
// non empty values in JUJU_DUMMY_DELAY will be parsed as
// time.Durations into this value.
var providerDelay, _ = time.ParseDuration(os.Getenv("JUJU_DUMMY_DELAY")) // parse errors are ignored

// pause execution to simulate the latency of a real provider
func delay() {
	if providerDelay > 0 {
		logger.Infof("pausing for %v", providerDelay)
		<-time.After(providerDelay)
	}
}

func (e *environ) AllocateContainerAddresses(context.ProviderCallContext, instance.Id, names.MachineTag, network.InterfaceInfos) (network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("container address allocation")
}

func (e *environ) ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container address allocation")
}

// ProviderSpaceInfo implements NetworkingEnviron.
func (*environ) ProviderSpaceInfo(context.ProviderCallContext, *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements NetworkingEnviron.
func (*environ) AreSpacesRoutable(_ context.ProviderCallContext, _, _ *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// MaybeWriteLXDProfile implements environs.LXDProfiler.
func (*environ) MaybeWriteLXDProfile(string, lxdprofile.Profile) error {
	return nil
}

// LXDProfileNames implements environs.LXDProfiler.
func (*environ) LXDProfileNames(string) ([]string, error) {
	return nil, nil
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (*environ) AssignLXDProfiles(_ string, profilesNames []string, _ []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}

// SuperSubnets implements environs.SuperSubnets
func (*environ) SuperSubnets(context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

// SetAgentStatus sets the presence for a particular agent in the fake presence implementation.
func (e *environ) SetAgentStatus(agent string, status presence.Status) {
	estate, err := e.state()
	if err != nil {
		panic(err)
	}
	estate.presence.agent[agent] = status
}

// fakePresence returns alive for all agent alive requests.
type fakePresence struct {
	agent map[string]presence.Status
}

func (*fakePresence) Disable()        {}
func (*fakePresence) Enable()         {}
func (*fakePresence) IsEnabled() bool { return true }
func (*fakePresence) Connect(_, _, _ string, _ uint64, _ bool, _ string) {
}
func (*fakePresence) Disconnect(string, uint64)                   {}
func (*fakePresence) Activity(string, uint64)                     {}
func (*fakePresence) ServerDown(string)                           {}
func (*fakePresence) UpdateServer(string, []presence.Value) error { return nil }
func (f *fakePresence) Connections() presence.Connections         { return f }

func (f *fakePresence) ForModel(string) presence.Connections  { return f }
func (f *fakePresence) ForServer(string) presence.Connections { return f }
func (f *fakePresence) ForAgent(string) presence.Connections  { return f }
func (*fakePresence) Count() int                              { return 0 }
func (*fakePresence) Models() []string                        { return nil }
func (*fakePresence) Servers() []string                       { return nil }
func (*fakePresence) Agents() []string                        { return nil }
func (*fakePresence) Values() []presence.Value                { return nil }

func (f *fakePresence) AgentStatus(agent string) (presence.Status, error) {
	if status, found := f.agent[agent]; found {
		return status, nil
	}
	return presence.Alive, nil
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

// NewCleanDB returns a new sql.DB reference.
func (e *environ) newCleanDB() (changestream.WatchableDB, error) {
	dir, err := os.MkdirTemp("", "dummy")
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)

	db, err := sql.Open("sqlite3", url)
	if err != nil {
		return nil, err
	}

	runner := &trackedDB{db: db}

	schema := domainschema.ControllerDDL(0x2dc171858c3155be)
	_, err = schema.Ensure(stdcontext.Background(), runner)
	return runner, errors.Trace(err)
}

var defaultTransactionRunner = txn.NewRetryingTxnRunner()

// trackedDB is used for testing purposes.
type trackedDB struct {
	db *sql.DB
}

func (t *trackedDB) Txn(ctx stdcontext.Context, fn func(stdcontext.Context, *sqlair.TX) error) error {
	db := sqlair.NewDB(t.db)
	return defaultTransactionRunner.Retry(ctx, func() error {
		return errors.Trace(defaultTransactionRunner.Txn(ctx, db, fn))
	})
}

func (t *trackedDB) StdTxn(ctx stdcontext.Context, fn func(stdcontext.Context, *sql.Tx) error) error {
	return defaultTransactionRunner.Retry(ctx, func() error {
		return errors.Trace(defaultTransactionRunner.StdTxn(ctx, t.db, fn))
	})
}

func (t *trackedDB) Subscribe(...changestream.SubscriptionOption) (changestream.Subscription, error) {
	return nil, nil
}
