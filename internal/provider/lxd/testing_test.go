// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"

	lxdclient "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

// Ensure LXD provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*environProvider)(nil)
)

// These are stub config values for use in tests.
var (
	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type": "lxd",
		"uuid": "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
	})
)

// We test these here since they are not exported.
var (
	_ environs.Environ   = (*environ)(nil)
	_ instances.Instance = (*environInstance)(nil)
)

type BaseSuiteUnpatched struct {
	testing.BaseSuite

	osPathOrig string

	Config    *config.Config
	EnvConfig *environConfig
	Provider  *environProvider
	Env       *environ

	Addresses     network.ProviderAddresses
	Instance      *environInstance
	Container     *lxd.Container
	InstName      string
	HWC           *instance.HardwareCharacteristics
	Metadata      map[string]string
	StartInstArgs environs.StartInstanceParams

	Rules          firewall.IngressRules
	EndpointAddrs  []string
	InterfaceAddr  string
	InterfaceAddrs []net.Addr

	Invalidator *MockCredentialInvalidator
}

func (s *BaseSuiteUnpatched) SetUpSuite(c *tc.C) {
	s.osPathOrig = os.Getenv("PATH")
	if s.osPathOrig == "" {
		// TODO(ericsnow) This shouldn't happen. However, an undiagnosed
		// bug in testing.BaseSuite is causing $PATH to remain unset
		// sometimes.  Once that is cleared up this special-case can go
		// away.
		s.osPathOrig =
			"/sbin:/bin:/usr/sbin:/usr/bin:/usr/local/sbin:/usr/local/bin"
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *BaseSuiteUnpatched) SetupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.Invalidator = NewMockCredentialInvalidator(ctrl)

	s.initProvider()
	s.initEnv(c)
	s.initInst(c)
	s.initNet(c)

	return ctrl
}

func (s *BaseSuiteUnpatched) initProvider() {
	s.Provider = &environProvider{}
	s.EndpointAddrs = []string{"1.2.3.4"}
	s.InterfaceAddr = "1.2.3.4"
	s.InterfaceAddrs = []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1")},
		&net.IPNet{IP: net.ParseIP("1.2.3.4")},
	}
}

func (s *BaseSuiteUnpatched) initEnv(c *tc.C) {
	certCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": testing.CACert,
		"client-key":  testing.CAKey,
		"server-cert": testing.ServerCert,
	})
	s.Env = &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(s.Invalidator, IsAuthorisationFailure),
		cloud: environscloudspec.CloudSpec{
			Name:       "localhost",
			Type:       "lxd",
			Credential: &certCred,
		},
		provider: s.Provider,
		name:     "lxd",
	}
	cfg := s.NewConfig(c, nil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) Prefix() string {
	return s.Env.namespace.Prefix()
}

func (s *BaseSuiteUnpatched) initInst(c *tc.C) {
	tools := []*coretools.Tools{
		{
			Version: semversion.Binary{Arch: arch.AMD64, Release: "ubuntu"},
			URL:     "https://example.org/amd",
		},
		{
			Version: semversion.Binary{Arch: arch.ARM64, Release: "ubuntu"},
			URL:     "https://example.org/arm",
		},
	}

	cons := constraints.Value{}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons,
		jujuversion.DefaultSupportedLTSBase(), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	err = instanceConfig.SetTools(coretools.List{
		tools[0],
	})
	c.Assert(err, tc.ErrorIsNil)
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	userData, err := providerinit.ComposeUserData(instanceConfig, nil, lxdRenderer{})
	c.Assert(err, tc.ErrorIsNil)

	var archName = arch.ARM64
	var numCores uint64 = 1
	var memoryMB uint64 = 3750
	s.HWC = &instance.HardwareCharacteristics{
		Arch:     &archName,
		CpuCores: &numCores,
		Mem:      &memoryMB,
	}

	s.Metadata = map[string]string{
		lxd.UserNamespacePrefix + tags.JujuIsController: "true",
		lxd.UserNamespacePrefix + tags.JujuController:   testing.ControllerTag.Id(),
		lxd.JujuModelKey: s.Config.UUID(),
		lxd.UserDataKey:  string(userData),
		"limits.cpu":     "1",
		"limits.memory":  strconv.Itoa(3750 * 1024 * 1024),
	}
	s.Addresses = network.ProviderAddresses{
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}

	// NOTE: the instance ids used throughout this package are not at all
	// representative of what they would normally be. They would normally
	// determined by the instance namespace and the machine id.
	s.Instance = s.NewInstance(c, "spam")
	s.Container = s.Instance.container
	s.InstName, err = s.Env.namespace.Hostname("42")
	c.Assert(err, tc.ErrorIsNil)

	s.StartInstArgs = environs.StartInstanceParams{
		ControllerUUID: instanceConfig.ControllerConfig.ControllerUUID(),
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
	}
}

func (s *BaseSuiteUnpatched) initNet(c *tc.C) {
	s.Rules = firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
}

func (s *BaseSuiteUnpatched) setConfig(c *tc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newValidConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)
	s.EnvConfig = ecfg
	uuid := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfgUnlocked = s.EnvConfig
	namespace, err := instance.NewNamespace(uuid)
	c.Assert(err, tc.ErrorIsNil)
	s.Env.namespace = namespace
}

func (s *BaseSuiteUnpatched) NewConfig(c *tc.C, updates testing.Attrs) *config.Config {
	if updates == nil {
		updates = make(testing.Attrs)
	}
	var err error
	cfg := testing.ModelConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func (s *BaseSuiteUnpatched) UpdateConfig(c *tc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) NewContainer(c *tc.C, name string) *lxd.Container {
	metadata := make(map[string]string)
	for k, v := range s.Metadata {
		metadata[k] = v
	}

	return &lxd.Container{
		Instance: api.Instance{
			Name:       name,
			StatusCode: api.Running,
			Status:     api.Running.String(),
			Config:     metadata,
			Type:       "container",
		},
	}
}

func (s *BaseSuiteUnpatched) NewInstance(c *tc.C, name string) *environInstance {
	container := s.NewContainer(c, name)
	return newInstance(container, s.Env)
}

type BaseSuite struct {
	BaseSuiteUnpatched

	Stub   *testhelpers.Stub
	Client *StubClient
	Common *stubCommon
}

func (s *BaseSuite) SetUpSuite(c *tc.C) {
	s.BaseSuiteUnpatched.SetUpSuite(c)
	// Do this *before* s.initEnv() gets called in BaseSuiteUnpatched.SetUpTest
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	testing.SkipLXDNotSupported(c)
}

func (s *BaseSuite) SetupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.BaseSuiteUnpatched.SetupMocks(c)

	s.Stub = &testhelpers.Stub{}
	s.Client = &StubClient{
		Stub:               s.Stub,
		StorageIsSupported: true,
		Server: &api.Server{
			ServerPut: api.ServerPut{
				Config: map[string]interface{}{},
			},
			Environment: api.ServerEnvironment{
				Certificate: "server-cert",
			},
		},
		Profile: &api.Profile{},
	}
	s.Common = &stubCommon{stub: s.Stub}

	// Patch out all expensive external deps.
	s.Env.serverUnlocked = s.Client
	s.Env.base = s.Common

	return ctrl
}

func (s *BaseSuite) TestingCert(c *tc.C) (lxd.Certificate, string) {
	cert := lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte(testing.CACert),
		KeyPEM:  []byte(testing.CAKey),
	}
	fingerprint, err := cert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)
	return cert, fingerprint
}

func (s *BaseSuite) CheckNoAPI(c *tc.C) {
	s.Stub.CheckCalls(c, nil)
}

func NewBaseConfig(c *tc.C) *config.Config {
	var err error
	cfg := testing.ModelConfig(c)

	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)

	return cfg
}

type ConfigValues struct{}

type Config struct {
	*environConfig
}

func NewConfig(cfg *config.Config) *Config {
	ecfg := newConfig(cfg)
	return &Config{ecfg}
}

func (ecfg *Config) Values(c *tc.C) (ConfigValues, map[string]interface{}) {
	c.Assert(ecfg.attrs, tc.DeepEquals, ecfg.UnknownAttrs())

	var values ConfigValues
	extras := make(map[string]interface{})
	for k, v := range ecfg.attrs {
		switch k {
		default:
			extras[k] = v
		}
	}
	return values, extras
}

func (ecfg *Config) Apply(c *tc.C, updates map[string]interface{}) *Config {
	cfg, err := ecfg.Config.Apply(updates)
	c.Assert(err, tc.ErrorIsNil)
	return NewConfig(cfg)
}

func (ecfg *Config) Validate() error {
	return ecfg.validate()
}

type stubCommon struct {
	stub *testhelpers.Stub

	BootstrapResult *environs.BootstrapResult
}

func (sc *stubCommon) BootstrapEnv(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	sc.stub.AddCall("Bootstrap", ctx, params)
	if err := sc.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return sc.BootstrapResult, nil
}

func (sc *stubCommon) DestroyEnv(ctx context.Context) error {
	sc.stub.AddCall("Destroy", ctx)
	if err := sc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type StubClient struct {
	*testhelpers.Stub

	Containers         []lxd.Container
	Container          *lxd.Container
	Server             *api.Server
	Profile            *api.Profile
	StorageIsSupported bool
	Volumes            map[string][]api.StorageVolume
	ServerCert         string
	ServerHostArch     string
	ServerVer          string
	NetworkNames       []string
	NetworkState       map[string]api.NetworkState
	ProfileNames       []string
}

func (conn *StubClient) FilterContainers(prefix string, statuses ...string) ([]lxd.Container, error) {
	conn.AddCall("FilterContainers", prefix, statuses)
	if err := conn.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return conn.Containers, nil
}

func (conn *StubClient) CreateContainerFromSpec(spec lxd.ContainerSpec) (*lxd.Container, error) {
	conn.AddCall("CreateContainerFromSpec", spec)
	if err := conn.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return conn.Container, nil
}

func (conn *StubClient) FindImage(
	ctx context.Context, base corebase.Base, arch string, virtType instance.VirtType, sources []lxd.ServerSpec, copyLocal bool, callback environs.StatusCallbackFunc,
) (lxd.SourcedImage, error) {
	conn.AddCall("FindImage", base.DisplayString(), arch)
	if err := conn.NextErr(); err != nil {
		return lxd.SourcedImage{}, errors.Trace(err)
	}

	return lxd.SourcedImage{}, nil
}

func (conn *StubClient) CreateCertificate(cert api.CertificatesPost) error {
	conn.AddCall("CreateCertificate", cert)
	return conn.NextErr()
}

func (conn *StubClient) CreateClientCertificate(cert *lxd.Certificate) error {
	conn.AddCall("CreateClientCertificate", cert)
	return conn.NextErr()
}

func (conn *StubClient) DeleteCertificate(fingerprint string) error {
	conn.AddCall("RemoveCertByFingerprint", fingerprint)
	return conn.NextErr()
}

func (conn *StubClient) GetCertificate(fingerprint string) (*api.Certificate, string, error) {
	conn.AddCall("GetCertificate", fingerprint)
	return &api.Certificate{}, "", conn.NextErr()
}

func (conn *StubClient) GetServer() (*api.Server, string, error) {
	conn.AddCall("ServerStatus")
	if err := conn.NextErr(); err != nil {
		return nil, "", err
	}
	return &api.Server{
		Environment: api.ServerEnvironment{
			Certificate: "server-cert",
		},
	}, "etag", nil
}

func (conn *StubClient) ServerVersion() string {
	conn.AddCall("ServerVersion")
	return conn.ServerVer
}

func (conn *StubClient) GetConnectionInfo() (info *lxdclient.ConnectionInfo, err error) {
	conn.AddCall("ServerAddresses")
	return &lxdclient.ConnectionInfo{
		Addresses: []string{"127.0.0.1:1234", "1.2.3.4:1234"},
	}, conn.NextErr()
}

func (conn *StubClient) UpdateServerConfig(cfg map[string]string) error {
	conn.AddCall("UpdateServerConfig", cfg)
	return conn.NextErr()
}

func (conn *StubClient) UpdateContainerConfig(container string, cfg map[string]string) error {
	conn.AddCall("UpdateContainerConfig", container, cfg)
	return conn.NextErr()
}

func (conn *StubClient) LocalBridgeName() string {
	conn.AddCall("LocalBridgeName")
	return "test-bridge"
}

func (conn *StubClient) GetProfile(name string) (*api.Profile, string, error) {
	conn.AddCall("GetProfile", name)
	return conn.Profile, "etag", conn.NextErr()
}

func (conn *StubClient) GetContainerProfiles(name string) ([]string, error) {
	conn.AddCall("GetContainerProfiles", name)
	return []string{
		"default",
		"juju-model-name",
	}, conn.NextErr()
}

func (conn *StubClient) DeleteProfile(name string) error {
	conn.AddCall("DeleteProfile", name)
	return conn.NextErr()
}

func (conn *StubClient) HasProfile(name string) (bool, error) {
	conn.AddCall("HasProfile", name)
	return false, conn.NextErr()
}

func (conn *StubClient) GetProfileNames() ([]string, error) {
	conn.AddCall("GetProfileNames")
	return conn.ProfileNames, conn.NextErr()
}

func (conn *StubClient) ReplaceOrAddContainerProfile(name, oldProfile, newProfile string) error {
	conn.AddCall("ReplaceOrAddContainerProfile", name, oldProfile, newProfile)
	return conn.NextErr()
}

func (conn *StubClient) UpdateContainerProfiles(name string, profiles []string) error {
	conn.AddCall("UpdateContainerProfiles", name, profiles)
	return conn.NextErr()
}

func (conn *StubClient) VerifyNetworkDevice(profile *api.Profile, etag string) error {
	conn.AddCall("VerifyNetworkDevice", profile, etag)
	return conn.NextErr()
}

func (conn *StubClient) StorageSupported() bool {
	conn.AddCall("StorageSupported")
	return conn.StorageIsSupported
}

func (conn *StubClient) EnsureDefaultStorage(profile *api.Profile, etag string) error {
	conn.AddCall("EnsureDefaultStorage", profile, etag)
	return conn.NextErr()
}

func (conn *StubClient) GetStoragePool(name string) (pool *api.StoragePool, etag string, err error) {
	conn.AddCall("GetStoragePool", name)
	return &api.StoragePool{
		Name:   name,
		Driver: "dir",
	}, "", conn.NextErr()
}

func (conn *StubClient) GetStoragePools() ([]api.StoragePool, error) {
	conn.AddCall("GetStoragePools")
	return []api.StoragePool{{
		Name:   "juju",
		Driver: "dir",
	}, {
		Name:   "juju-zfs",
		Driver: "zfs",
	}}, conn.NextErr()
}

func (conn *StubClient) CreatePool(name, driver string, attrs map[string]string) error {
	conn.AddCall("CreatePool", name, driver, attrs)
	return conn.NextErr()
}

func (conn *StubClient) CreateVolume(pool, volume string, config map[string]string) error {
	conn.AddCall("CreateVolume", pool, volume, config)
	return conn.NextErr()
}

func (conn *StubClient) DeleteStoragePoolVolume(pool, volType, volume string) error {
	conn.AddCall("DeleteStoragePoolVolume", pool, volType, volume)
	return conn.NextErr()
}

func (conn *StubClient) GetStoragePoolVolume(
	pool string, volType string, name string,
) (*api.StorageVolume, string, error) {
	conn.AddCall("GetStoragePoolVolume", pool, volType, name)
	if err := conn.NextErr(); err != nil {
		return nil, "", err
	}
	for _, v := range conn.Volumes[pool] {
		if v.Name == name {
			return &v, "eTag", nil
		}
	}
	return nil, "", errors.NotFoundf("volume %q in pool %q", name, pool)
}

func (conn *StubClient) GetStoragePoolVolumes(pool string) ([]api.StorageVolume, error) {
	conn.AddCall("GetStoragePoolVolumes", pool)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}
	return conn.Volumes[pool], nil
}

func (conn *StubClient) UpdateStoragePoolVolume(
	pool string, volType string, name string, volume api.StorageVolumePut, etag string,
) error {
	conn.AddCall("UpdateStoragePoolVolume", pool, volType, name, volume, etag)
	return conn.NextErr()
}

func (conn *StubClient) AliveContainers(prefix string) ([]lxd.Container, error) {
	conn.AddCall("AliveContainers", prefix)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}
	return conn.Containers, nil
}

func (conn *StubClient) ContainerAddresses(name string) ([]network.ProviderAddress, error) {
	conn.AddCall("ContainerAddresses", name)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}

	return network.ProviderAddresses{
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}, nil
}

func (conn *StubClient) RemoveContainer(name string) error {
	conn.AddCall("RemoveContainer", name)
	return conn.NextErr()
}

func (conn *StubClient) RemoveContainers(names []string) error {
	conn.AddCall("RemoveContainers", names)
	return conn.NextErr()
}

func (conn *StubClient) WriteContainer(container *lxd.Container) error {
	conn.AddCall("WriteContainer", container)
	return conn.NextErr()
}

func (conn *StubClient) CreateProfileWithConfig(name string, cfg map[string]string) error {
	conn.AddCall("CreateProfileWithConfig", name, cfg)
	return conn.NextErr()
}

func (conn *StubClient) CreateProfile(post api.ProfilesPost) error {
	conn.AddCall("CreateProfile", post)
	return conn.NextErr()
}

func (conn *StubClient) ServerCertificate() string {
	conn.AddCall("ServerCertificate")
	return conn.ServerCert
}

func (conn *StubClient) HostArch() string {
	conn.AddCall("HostArch")
	return conn.ServerHostArch
}

func (conn *StubClient) SupportedArches() []string {
	conn.AddCall("SupportedArches")
	return []string{conn.ServerHostArch}
}

func (conn *StubClient) EnableHTTPSListener() error {
	conn.AddCall("EnableHTTPSListener")
	return conn.NextErr()
}

func (conn *StubClient) GetNICsFromProfile(profName string) (map[string]map[string]string, error) {
	conn.AddCall("GetNICsFromProfile", profName)
	return conn.Profile.Devices, conn.NextErr()
}

func (conn *StubClient) IsClustered() bool {
	conn.AddCall("IsClustered")
	return true
}

func (conn *StubClient) Name() string {
	conn.AddCall("Name")
	return "server"
}

func (conn *StubClient) UseProject(string) {
	panic("this stub is deprecated; use mocks instead")
}

func (*StubClient) HasExtension(_ string) bool {
	panic("this stub is deprecated; use mocks instead")
}

func (conn *StubClient) GetNetworks() ([]api.Network, error) {
	panic("this stub is deprecated; use mocks instead")
}

func (*StubClient) GetNetworkState(string) (*api.NetworkState, error) {
	panic("this stub is deprecated; use mocks instead")
}

func (*StubClient) GetInstance(string) (*api.Instance, string, error) {
	panic("this stub is deprecated; use mocks instead")
}

func (*StubClient) GetInstanceState(string) (*api.InstanceState, string, error) {
	panic("this stub is deprecated; use mocks instead")
}

// TODO (manadart 2018-07-20): This exists to satisfy the testing stub
// interface. It is temporary, pending replacement with mocks and
// should not be called in tests.
func (conn *StubClient) UseTargetServer(ctx context.Context, name string) (*lxd.Server, error) {
	conn.AddCall("UseTargetServer", name)
	return nil, conn.NextErr()
}

func (conn *StubClient) GetClusterMembers() (members []api.ClusterMember, err error) {
	conn.AddCall("GetClusterMembers")
	return nil, conn.NextErr()
}

type MockClock struct {
	clock.Clock
	now time.Time
}

func (m *MockClock) Now() time.Time {
	return m.now
}

func (m *MockClock) After(delay time.Duration) <-chan time.Time {
	return time.After(time.Millisecond)
}

// TODO (manadart 2018-07-20): All of the above logic should ultimately be
// replaced by what follows (in some form). The stub usage will be abandoned
// and replaced by mocks.

type EnvironSuite struct {
	testing.BaseSuite
}

func (s *EnvironSuite) NewEnviron(c *tc.C,
	srv Server,
	cfgEdit map[string]interface{},
	cloudSpec environscloudspec.CloudSpec,
	invalidator environs.CredentialInvalidator,
) environs.Environ {
	cfg, err := testing.ModelConfig(c).Apply(ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)

	if cfgEdit != nil {
		var err error
		cfg, err = cfg.Apply(cfgEdit)
		c.Assert(err, tc.ErrorIsNil)
	}

	eCfg, err := newValidConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	namespace, err := instance.NewNamespace(cfg.UUID())
	c.Assert(err, tc.ErrorIsNil)

	return &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, IsAuthorisationFailure),
		serverUnlocked:        srv,
		ecfgUnlocked:          eCfg,
		namespace:             namespace,
		cloud:                 cloudSpec,
		uuid:                  eCfg.UUID(),
		name:                  "model",
	}
}

func (s *EnvironSuite) NewEnvironWithServerFactory(c *tc.C,
	srv ServerFactory,
	cfgEdit map[string]interface{},
	invalidator environs.CredentialInvalidator,
) environs.Environ {
	cfg, err := testing.ModelConfig(c).Apply(ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)

	if cfgEdit != nil {
		var err error
		cfg, err = cfg.Apply(cfgEdit)
		c.Assert(err, tc.ErrorIsNil)
	}

	eCfg, err := newValidConfig(c.Context(), cfg)
	c.Assert(err, tc.ErrorIsNil)

	namespace, err := instance.NewNamespace(cfg.UUID())
	c.Assert(err, tc.ErrorIsNil)

	provid := environProvider{
		serverFactory: srv,
	}

	return &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, IsAuthorisationFailure),
		name:                  "controller",
		provider:              &provid,
		ecfgUnlocked:          eCfg,
		namespace:             namespace,
		uuid:                  eCfg.UUID(),
	}
}

func (s *EnvironSuite) GetStartInstanceArgs(c *tc.C) environs.StartInstanceParams {
	tools := []*coretools.Tools{
		{
			Version: semversion.Binary{Arch: arch.AMD64, Release: "ubuntu"},
			URL:     "https://example.org/amd",
		},
		{
			Version: semversion.Binary{Arch: arch.ARM64, Release: "ubuntu"},
			URL:     "https://example.org/arm",
		},
	}

	cons := constraints.Value{}
	iConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons,
		jujuversion.DefaultSupportedLTSBase(), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	return environs.StartInstanceParams{
		ControllerUUID: iConfig.ControllerConfig.ControllerUUID(),
		InstanceConfig: iConfig,
		Tools:          tools,
		Constraints:    cons,
	}
}
