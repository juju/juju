// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"net"
	"os"
	"path"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/tools/lxdclient"
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

	Addresses     []network.Address
	Instance      *environInstance
	RawInstance   *lxdclient.Instance
	InstName      string
	Hardware      *lxdclient.InstanceHardware
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

	cons := constraints.Value{
		// nothing
	}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons, "trusty", "")
	c.Assert(err, jc.ErrorIsNil)

	err = instanceConfig.SetTools(coretools.List{
		tools[0],
	})
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	userData, err := providerinit.ComposeUserData(instanceConfig, nil, lxdRenderer{})
	c.Assert(err, jc.ErrorIsNil)

	s.Hardware = &lxdclient.InstanceHardware{
		Architecture: arch.ARM64,
		NumCores:     1,
		MemoryMB:     3750,
	}
	var archName string = arch.ARM64
	var numCores uint64 = 1
	var memoryMB uint64 = 3750
	s.HWC = &instance.HardwareCharacteristics{
		Arch:     &archName,
		CpuCores: &numCores,
		Mem:      &memoryMB,
	}

	s.Metadata = map[string]string{ // userdata
		tags.JujuIsController: "true",
		tags.JujuController:   testing.ControllerTag.Id(),
		tags.JujuModel:        s.Config.UUID(),
		metadataKeyCloudInit:  string(userData),
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
	s.RawInstance = s.Instance.raw
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

func (s *BaseSuiteUnpatched) NewRawInstance(c *gc.C, name string) *lxdclient.Instance {
	metadata := make(map[string]string)
	for k, v := range s.Metadata {
		metadata[k] = v
	}
	summary := lxdclient.InstanceSummary{
		Name:     name,
		Status:   lxdclient.StatusRunning,
		Hardware: *s.Hardware,
		Metadata: metadata,
	}
	instanceSpec := lxdclient.InstanceSpec{
		Name:      name,
		Profiles:  []string{},
		Ephemeral: false,
		Metadata:  metadata,
	}
	return lxdclient.NewInstance(summary, &instanceSpec)
}

func (s *BaseSuiteUnpatched) NewInstance(c *gc.C, name string) *environInstance {
	raw := s.NewRawInstance(c, name)
	return newInstance(raw, s.Env)
}

func (s *BaseSuiteUnpatched) IsRunningLocally(c *gc.C) bool {
	restore := gitjujutesting.PatchEnvPathPrepend(s.osPathOrig)
	defer restore()

	running, err := lxdclient.IsRunningLocally()
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
		lxdCerts:     s.Client,
		lxdConfig:    s.Client,
		lxdInstances: s.Client,
		lxdProfiles:  s.Client,
		lxdImages:    s.Client,
		lxdStorage:   s.Client,
		remote: lxdclient.Remote{
			Cert: &lxdclient.Cert{
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

func (s *BaseSuite) TestingCert(c *gc.C) (lxdclient.Cert, string) {
	cert := lxdclient.Cert{
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

func (sc *stubCommon) BootstrapEnv(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	sc.stub.AddCall("Bootstrap", ctx, params)
	if err := sc.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return sc.BootstrapResult, nil
}

func (sc *stubCommon) DestroyEnv() error {
	sc.stub.AddCall("Destroy")
	if err := sc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type StubClient struct {
	*gitjujutesting.Stub

	Insts              []lxdclient.Instance
	Inst               *lxdclient.Instance
	Server             *api.Server
	StorageIsSupported bool
	Volumes            map[string][]api.StorageVolume
}

func (conn *StubClient) Instances(prefix string, statuses ...string) ([]lxdclient.Instance, error) {
	conn.AddCall("Instances", prefix, statuses)
	if err := conn.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return conn.Insts, nil
}

func (conn *StubClient) AddInstance(spec lxdclient.InstanceSpec) (*lxdclient.Instance, error) {
	conn.AddCall("AddInstance", spec)
	if err := conn.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return conn.Inst, nil
}

func (conn *StubClient) RemoveInstances(prefix string, ids ...string) error {
	conn.AddCall("RemoveInstances", prefix, ids)
	if err := conn.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (conn *StubClient) EnsureImageExists(series, arch string, _ []lxdclient.Remote, _ func(string)) (string, error) {
	conn.AddCall("EnsureImageExists", series, arch)
	if err := conn.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return path.Join("juju", series, arch), nil
}

func (conn *StubClient) Addresses(name string) ([]network.Address, error) {
	conn.AddCall("Addresses", name)
	if err := conn.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return []network.Address{network.Address{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}, nil
}

func (conn *StubClient) AddCert(cert lxdclient.Cert) error {
	conn.AddCall("AddCert", cert)
	return conn.NextErr()
}

func (conn *StubClient) RemoveCertByFingerprint(fingerprint string) error {
	conn.AddCall("RemoveCertByFingerprint", fingerprint)
	return conn.NextErr()
}

func (conn *StubClient) CertByFingerprint(fingerprint string) (api.Certificate, error) {
	conn.AddCall("CertByFingerprint", fingerprint)
	return api.Certificate{}, conn.NextErr()
}

func (conn *StubClient) ServerStatus() (*api.Server, error) {
	conn.AddCall("ServerStatus")
	if err := conn.NextErr(); err != nil {
		return nil, err
	}
	return &api.Server{
		Environment: api.ServerEnvironment{
			Certificate: "server-cert",
		},
	}, nil
}

func (conn *StubClient) ServerAddresses() ([]string, error) {
	conn.AddCall("ServerAddresses")
	return []string{
		"127.0.0.1:1234",
		"1.2.3.4:1234",
	}, conn.NextErr()
}

func (conn *StubClient) SetServerConfig(k, v string) error {
	conn.AddCall("SetServerConfig", k, v)
	return conn.NextErr()
}

func (conn *StubClient) SetContainerConfig(container, k, v string) error {
	conn.AddCall("SetContainerConfig", container, k, v)
	return conn.NextErr()
}

func (conn *StubClient) DefaultProfileBridgeName() string {
	conn.AddCall("DefaultProfileBridgeName")
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

func (conn *StubClient) AttachDisk(container, device string, disk lxdclient.DiskDevice) error {
	conn.AddCall("AttachDisk", container, device, disk)
	return conn.NextErr()
}

func (conn *StubClient) RemoveDevice(container, device string) error {
	conn.AddCall("RemoveDevice", container, device)
	return conn.NextErr()
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
