// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"net"
	"os"
	"strconv"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	lxdclient "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container/lxd"
	containerlxd "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujulxdclient "github.com/juju/juju/tools/lxdclient"
)

// Ensure LXD provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*environProvider)(nil)
)

// These values are stub LXD client credentials for use in tests.
const (
	PublicKey = `-----BEGIN CERTIFICATE-----
...
...
...
...
...
...
...
...
...
...
...
...
...
...
-----END CERTIFICATE-----
`
	PrivateKey = `-----BEGIN PRIVATE KEY-----
...
...
...
...
...
...
...
...
...
...
...
...
...
...
-----END PRIVATE KEY-----
`
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
	_ environs.Environ  = (*environ)(nil)
	_ instance.Instance = (*environInstance)(nil)
)

type BaseSuiteUnpatched struct {
	testing.BaseSuite

	osPathOrig string

	Config    *config.Config
	EnvConfig *environConfig
	Provider  *environProvider
	Env       *environ

	Addresses []network.Address
	Instance  *environInstance
	Container *lxd.Container
	InstName  string
	//Hardware      *jujulxdclient.InstanceHardware
	HWC           *instance.HardwareCharacteristics
	Metadata      map[string]string
	StartInstArgs environs.StartInstanceParams
	//InstanceType  instances.InstanceType

	Rules          []network.IngressRule
	EndpointAddrs  []string
	InterfaceAddr  string
	InterfaceAddrs []net.Addr
}

func (s *BaseSuiteUnpatched) SetUpSuite(c *gc.C) {
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

func (s *BaseSuiteUnpatched) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.initProvider(c)
	s.initEnv(c)
	s.initInst(c)
	s.initNet(c)
}

func (s *BaseSuiteUnpatched) initProvider(c *gc.C) {
	s.Provider = &environProvider{}
	s.EndpointAddrs = []string{"1.2.3.4"}
	s.InterfaceAddr = "1.2.3.4"
	s.InterfaceAddrs = []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1")},
		&net.IPNet{IP: net.ParseIP("1.2.3.4")},
	}
}

func (s *BaseSuiteUnpatched) initEnv(c *gc.C) {
	certCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": testing.CACert,
		"client-key":  testing.CAKey,
		"server-cert": testing.ServerCert,
	})
	s.Env = &environ{
		cloud: environs.CloudSpec{
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

func (s *BaseSuiteUnpatched) initInst(c *gc.C) {
	tools := []*coretools.Tools{
		{
			Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
			URL:     "https://example.org/amd",
		},
		{
			Version: version.Binary{Arch: arch.ARM64, Series: "trusty"},
			URL:     "https://example.org/arm",
		},
	}

	cons := constraints.Value{}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons, "trusty", "")
	c.Assert(err, jc.ErrorIsNil)

	err = instanceConfig.SetTools(coretools.List{
		tools[0],
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	userData, err := providerinit.ComposeUserData(instanceConfig, nil, lxdRenderer{})
	c.Assert(err, jc.ErrorIsNil)

	//s.Hardware = &jujulxdclient.InstanceHardware{
	//	Architecture: arch.ARM64,
	//	NumCores:     1,
	//	MemoryMB:     3750,
	//}
	var archName = arch.ARM64
	var numCores uint64 = 1
	var memoryMB uint64 = 3750
	s.HWC = &instance.HardwareCharacteristics{
		Arch:     &archName,
		CpuCores: &numCores,
		Mem:      &memoryMB,
	}

	s.Metadata = map[string]string{
		containerlxd.UserNamespacePrefix + tags.JujuIsController: "true",
		containerlxd.UserNamespacePrefix + tags.JujuController:   testing.ControllerTag.Id(),
		containerlxd.JujuModelKey:                                s.Config.UUID(),
		containerlxd.UserDataKey:                                 string(userData),
		"limits.cpu":                                             "1",
		"limits.memory":                                          strconv.Itoa(3750 * 1024 * 1024),
	}
	s.Addresses = []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	// NOTE: the instance ids used throughout this package are not at all
	// representative of what they would normally be. They would normally
	// determined by the instance namespace and the machine id.
	s.Instance = s.NewInstance(c, "spam")
	s.Container = s.Instance.container
	s.InstName, err = s.Env.namespace.Hostname("42")
	c.Assert(err, jc.ErrorIsNil)

	s.StartInstArgs = environs.StartInstanceParams{
		ControllerUUID: instanceConfig.Controller.Config.ControllerUUID(),
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
	}
}

func (s *BaseSuiteUnpatched) initNet(c *gc.C) {
	s.Rules = []network.IngressRule{network.MustNewIngressRule("tcp", 80, 80)}
}

func (s *BaseSuiteUnpatched) setConfig(c *gc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newValidConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.EnvConfig = ecfg
	uuid := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfg = s.EnvConfig
	namespace, err := instance.NewNamespace(uuid)
	c.Assert(err, jc.ErrorIsNil)
	s.Env.namespace = namespace
}

func (s *BaseSuiteUnpatched) NewConfig(c *gc.C, updates testing.Attrs) *config.Config {
	if updates == nil {
		updates = make(testing.Attrs)
	}
	var err error
	cfg := testing.ModelConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *BaseSuiteUnpatched) UpdateConfig(c *gc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) NewContainer(c *gc.C, name string) *containerlxd.Container {
	metadata := make(map[string]string)
	for k, v := range s.Metadata {
		metadata[k] = v
	}

	return &containerlxd.Container{
		Container: api.Container{
			Name:       name,
			StatusCode: api.Running,
			Status:     api.Running.String(),
			ContainerPut: api.ContainerPut{
				Config: metadata,
			},
		},
	}
}

func (s *BaseSuiteUnpatched) NewInstance(c *gc.C, name string) *environInstance {
	container := s.NewContainer(c, name)
	return newInstance(container, s.Env)
}

func (s *BaseSuiteUnpatched) IsRunningLocally(c *gc.C) bool {
	restore := gitjujutesting.PatchEnvPathPrepend(s.osPathOrig)
	defer restore()

	running, err := jujulxdclient.IsRunningLocally()
	c.Assert(err, jc.ErrorIsNil)
	return running
}

type BaseSuite struct {
	BaseSuiteUnpatched

	Stub   *gitjujutesting.Stub
	Client *StubClient
	Common *stubCommon
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuiteUnpatched.SetUpSuite(c)
	// Do this *before* s.initEnv() gets called in BaseSuiteUnpatched.SetUpTest
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuiteUnpatched.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
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
	}
	s.Common = &stubCommon{stub: s.Stub}

	// Patch out all expensive external deps.
	raw := &rawProvider{
		newServer:   s.Client,
		lxdProfiles: s.Client,
		lxdStorage:  s.Client,
		remote: jujulxdclient.Remote{
			Cert: &lxd.Certificate{
				Name:    "juju",
				CertPEM: []byte(testing.CACert),
				KeyPEM:  []byte(testing.CAKey),
			},
		},
	}
	s.Env.raw = raw
	s.Provider.generateMemCert = func(client bool) (cert, key []byte, _ error) {
		s.Stub.AddCall("GenerateMemCert", client)
		cert = []byte(testing.CACert + "generated")
		key = []byte(testing.CAKey + "generated")
		return cert, key, s.Stub.NextErr()
	}
	s.Provider.newLocalRawProvider = func() (*rawProvider, error) {
		return raw, nil
	}
	s.Provider.lookupHost = func(host string) ([]string, error) {
		s.Stub.AddCall("LookupHost", host)
		return s.EndpointAddrs, s.Stub.NextErr()
	}
	s.Provider.interfaceAddress = func(iface string) (string, error) {
		s.Stub.AddCall("InterfaceAddress", iface)
		return s.InterfaceAddr, s.Stub.NextErr()
	}
	s.Provider.interfaceAddrs = func() ([]net.Addr, error) {
		s.Stub.AddCall("InterfaceAddrs")
		return s.InterfaceAddrs, s.Stub.NextErr()
	}
	s.Env.base = s.Common
}

func (s *BaseSuite) TestingCert(c *gc.C) (lxd.Certificate, string) {
	cert := lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte(testing.CACert),
		KeyPEM:  []byte(testing.CAKey),
	}
	fingerprint, err := cert.Fingerprint()
	c.Assert(err, jc.ErrorIsNil)
	return cert, fingerprint
}

func (s *BaseSuite) CheckNoAPI(c *gc.C) {
	s.Stub.CheckCalls(c, nil)
}

func NewBaseConfig(c *gc.C) *config.Config {
	var err error
	cfg := testing.ModelConfig(c)

	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)

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

func (ecfg *Config) Values(c *gc.C) (ConfigValues, map[string]interface{}) {
	c.Assert(ecfg.attrs, jc.DeepEquals, ecfg.UnknownAttrs())

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

func (ecfg *Config) Apply(c *gc.C, updates map[string]interface{}) *Config {
	cfg, err := ecfg.Config.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return NewConfig(cfg)
}

func (ecfg *Config) Validate() error {
	return ecfg.validate()
}

type stubCommon struct {
	stub *gitjujutesting.Stub

	BootstrapResult *environs.BootstrapResult
}

func (sc *stubCommon) BootstrapEnv(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	sc.stub.AddCall("Bootstrap", ctx, callCtx, params)
	if err := sc.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return sc.BootstrapResult, nil
}

func (sc *stubCommon) DestroyEnv(callCtx context.ProviderCallContext) error {
	sc.stub.AddCall("Destroy", callCtx)
	if err := sc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type StubClient struct {
	*gitjujutesting.Stub

	Containers         []lxd.Container
	Container          *lxd.Container
	Server             *api.Server
	StorageIsSupported bool
	Volumes            map[string][]api.StorageVolume
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
	series, arch string, sources []lxd.RemoteServer, copyLocal bool, callback environs.StatusCallbackFunc,
) (lxd.SourcedImage, error) {
	conn.AddCall("FindImage", series, arch)
	if err := conn.NextErr(); err != nil {
		return lxd.SourcedImage{}, errors.Trace(err)
	}

	return lxd.SourcedImage{}, nil
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

func (conn *StubClient) CreateProfile(name string, attrs map[string]string) error {
	conn.AddCall("CreateProfile", name, attrs)
	return conn.NextErr()
}

func (conn *StubClient) HasProfile(name string) (bool, error) {
	conn.AddCall("HasProfile", name)
	return false, conn.NextErr()
}

func (conn *StubClient) StorageSupported() bool {
	conn.AddCall("StorageSupported")
	return conn.StorageIsSupported
}

func (conn *StubClient) StoragePool(name string) (api.StoragePool, error) {
	conn.AddCall("StoragePool", name)
	return api.StoragePool{
		Name:   name,
		Driver: "dir",
	}, conn.NextErr()
}

func (conn *StubClient) StoragePools() ([]api.StoragePool, error) {
	conn.AddCall("StoragePools")
	return []api.StoragePool{{
		Name:   "juju",
		Driver: "dir",
	}, {
		Name:   "juju-zfs",
		Driver: "zfs",
	}}, conn.NextErr()
}

func (conn *StubClient) CreateStoragePool(name, driver string, attrs map[string]string) error {
	conn.AddCall("CreateStoragePool", name, driver, attrs)
	return conn.NextErr()
}

func (conn *StubClient) VolumeCreate(pool, volume string, config map[string]string) error {
	conn.AddCall("VolumeCreate", pool, volume, config)
	return conn.NextErr()
}

func (conn *StubClient) VolumeDelete(pool, volume string) error {
	conn.AddCall("VolumeDelete", pool, volume)
	return conn.NextErr()
}

func (conn *StubClient) Volume(pool, volume string) (api.StorageVolume, error) {
	conn.AddCall("Volume", pool, volume)
	if err := conn.NextErr(); err != nil {
		return api.StorageVolume{}, err
	}
	for _, v := range conn.Volumes[pool] {
		if v.Name == volume {
			return v, nil
		}
	}
	return api.StorageVolume{}, errors.NotFoundf("volume %q in pool %q", volume, pool)
}

func (conn *StubClient) VolumeList(pool string) ([]api.StorageVolume, error) {
	conn.AddCall("VolumeList", pool)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}
	return conn.Volumes[pool], nil
}

func (conn *StubClient) VolumeUpdate(pool, volume string, update api.StorageVolume) error {
	conn.AddCall("VolumeUpdate", pool, volume, update)
	return conn.NextErr()
}

func (conn *StubClient) AliveContainers(prefix string) ([]lxd.Container, error) {
	conn.AddCall("AliveContainers", prefix)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}
	return conn.Containers, nil
}

func (conn *StubClient) ContainerAddresses(name string) ([]network.Address, error) {
	conn.AddCall("ContainerAddresses", name)
	if err := conn.NextErr(); err != nil {
		return nil, err
	}

	return []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}, nil
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
