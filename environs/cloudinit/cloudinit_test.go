package cloudinit_test

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"regexp"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct{}

var _ = Suite(cloudinitSuite{})

var envConfig = mustNewConfig(map[string]interface{}{
	"type":            "ec2",
	"name":            "foo",
	"default-series":  "series",
	"authorized-keys": "keys",
	"root-cert": certAndKey,
})

func mustNewConfig(m map[string]interface{}) *config.Config {
	cfg, err := config.New(m)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []cloudinit.MachineConfig{
	{
		InstanceIdAccessor: "$instance_id",
		MachineId:          0,
		ProviderType:       "ec2",
		AuthorizedKeys:     "sshkey1",
		Tools:              newSimpleTools("1.2.3-linux-amd64"),
		StateServer:        true,
		StateServerPEM:     serverPEM,
		StateInfo: &state.Info{
			Password: "arble",
		},
		Config:  envConfig,
		DataDir: "/var/lib/juju",
	},
	{
		MachineId:      99,
		ProviderType:   "ec2",
		AuthorizedKeys: "sshkey1",
		DataDir:        "/var/lib/juju",
		StateServer:    false,
		Tools:          newSimpleTools("1.2.3-linux-amd64"),
		StateInfo: &state.Info{
			Addrs:      []string{"state-addr.example.com"},
			EntityName: "machine-99",
			Password:   "arble",
		},
	},
}

func newSimpleTools(vers string) *state.Tools {
	return &state.Tools{
		URL:    "http://foo.com/tools/juju" + vers + ".tgz",
		Binary: version.MustParseBinary(vers),
	}
}

// cloundInitTest runs a set of tests for one of the MachineConfig
// values above.
type cloudinitTest struct {
	x   map[interface{}]interface{} // the unmarshalled YAML.
	cfg *cloudinit.MachineConfig    // the config being tested.
}

func (t *cloudinitTest) check(c *C, cfg *cloudinit.MachineConfig) {
	c.Check(t.x["apt_upgrade"], Equals, true)
	c.Check(t.x["apt_update"], Equals, true)
	t.checkScripts(c, "mkdir -p "+cfg.DataDir)
	t.checkScripts(c, "wget.*"+regexp.QuoteMeta(t.cfg.Tools.URL)+".*tar .*xz")

	if t.cfg.StateServer {
		t.checkScripts(c, regexp.QuoteMeta(t.cfg.InstanceIdAccessor))
	}
	if t.cfg.Config != nil {
		t.checkScripts(c, "tools/mongo-.*tgz")
		t.checkEnvConfig(c)
	}
	t.checkPackage(c, "git")

	if t.cfg.StateServer {
		t.checkScripts(c, "jujud bootstrap-state"+
			".* --state-servers localhost:37017"+
			".*--initial-password '"+t.cfg.StateInfo.Password+"'")
		t.checkScripts(c, "jujud machine"+
			" --state-servers 'localhost:37017' "+
			".*--initial-password '"+t.cfg.StateInfo.Password+"'"+
			".* --machine-id [0-9]+"+
			".*>> /var/log/juju/.*log 2>&1")
	} else {
		t.checkScripts(c, "jujud machine"+
			" --state-servers '"+strings.Join(t.cfg.StateInfo.Addrs, ",")+"'"+
			".*--initial-password '"+t.cfg.StateInfo.Password+"'"+
			" .*--machine-id [0-9]+"+
			".*>> /var/log/juju/.*log 2>&1")
	}
}

// check that any --env-config $base64 is valid and matches t.cfg.Config
func (t *cloudinitTest) checkEnvConfig(c *C) {
	scripts0 := t.x["runcmd"]
	if scripts0 == nil {
		c.Errorf("cloudinit has no entry for runcmd")
		return
	}
	scripts := scripts0.([]interface{})
	re := regexp.MustCompile(`--env-config '([\w,=]+)'`)
	found := false
	for _, s0 := range scripts {
		m := re.FindStringSubmatch(s0.(string))
		if m == nil {
			continue
		}
		found = true
		buf, err := base64.StdEncoding.DecodeString(m[1])
		c.Assert(err, IsNil)
		var actual map[string]interface{}
		err = goyaml.Unmarshal(buf, &actual)
		c.Assert(err, IsNil)
		c.Assert(t.cfg.Config.AllAttrs(), DeepEquals, actual)
	}
	c.Assert(found, Equals, true)
}

func (t *cloudinitTest) checkScripts(c *C, pattern string) {
	CheckScripts(c, t.x, pattern, true)
}

// If match is true, CheckScripts checks that at least one script started
// by the cloudinit data matches the given regexp pattern, otherwise it
// checks that no script matches.  It's exported so it can be used by tests
// defined in ec2_test.
func CheckScripts(c *C, x map[interface{}]interface{}, pattern string, match bool) {
	scripts0 := x["runcmd"]
	if scripts0 == nil {
		c.Errorf("cloudinit has no entry for runcmd")
		return
	}
	scripts := scripts0.([]interface{})
	re := regexp.MustCompile(pattern)
	found := false
	for _, s0 := range scripts {
		s := s0.(string)
		if re.MatchString(s) {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("script %q not found in %q", pattern, scripts)
	case !match && found:
		c.Errorf("script %q found but not expected in %q", pattern, scripts)
	}
}

func (t *cloudinitTest) checkPackage(c *C, pkg string) {
	CheckPackage(c, t.x, pkg, true)
}

// CheckPackage checks that the cloudinit will or won't install the given
// package, depending on the value of match.  It's exported so it can be
// used by tests defined outside the ec2 package.
func CheckPackage(c *C, x map[interface{}]interface{}, pkg string, match bool) {
	pkgs0 := x["packages"]
	if pkgs0 == nil {
		if match {
			c.Errorf("cloudinit has no entry for packages")
		}
		return
	}

	pkgs := pkgs0.([]interface{})

	found := false
	for _, p0 := range pkgs {
		p := p0.(string)
		if p == pkg {
			found = true
		}
	}
	switch {
	case match && !found:
		c.Errorf("package %q not found in %v", pkg, pkgs)
	case !match && found:
		c.Errorf("%q found but not expected in %v", pkg, pkgs)
	}
}

// TestCloudInit checks that the output from the various tests
// in cloudinitTests is well formed.
func (cloudinitSuite) TestCloudInit(c *C) {
	for i, cfg := range cloudinitTests {
		c.Logf("test %d", i)
		ci, err := cloudinit.New(&cfg)
		c.Assert(err, IsNil)
		c.Check(ci, NotNil)

		// render the cloudinit config to bytes, and then
		// back to a map so we can introspect it without
		// worrying about internal details of the cloudinit
		// package.

		data, err := ci.Render()
		c.Assert(err, IsNil)

		x := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &x)
		c.Assert(err, IsNil)

		t := &cloudinitTest{
			cfg: &cfg,
			x:   x,
		}
		t.check(c, &cfg)
	}
}

// When mutate is called on a known-good MachineConfig,
// there should be an error complaining about the missing
// field named by the adjacent err.
var verifyTests = []struct {
	err    string
	mutate func(*cloudinit.MachineConfig)
}{
	{"negative machine id", func(cfg *cloudinit.MachineConfig) {
		cfg.MachineId = -1
	}},
	{"missing provider type", func(cfg *cloudinit.MachineConfig) {
		cfg.ProviderType = ""
	}},
	{"missing instance id accessor", func(cfg *cloudinit.MachineConfig) {
		cfg.InstanceIdAccessor = ""
	}},
	{"missing environment configuration", func(cfg *cloudinit.MachineConfig) {
		cfg.Config = nil
	}},
	{"missing state info", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo = nil
	}},
	{"missing state hosts", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		cfg.StateInfo = &state.Info{EntityName: "machine-99"}
	}},
	{"missing state server PEM", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServerPEM = []byte{}
	}},
	{"missing var directory", func(cfg *cloudinit.MachineConfig) {
		cfg.DataDir = ""
	}},
	{"missing tools", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = nil
	}},
	{"missing tools URL", func(cfg *cloudinit.MachineConfig) {
		cfg.Tools = &state.Tools{}
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.EntityName = "machine-0"
		cfg.StateInfo = &info
	}},
	{"entity name must match started machine", func(cfg *cloudinit.MachineConfig) {
		cfg.StateServer = false
		info := *cfg.StateInfo
		info.EntityName = ""
		cfg.StateInfo = &info
	}},
	{"entity name must be blank when starting a state server", func(cfg *cloudinit.MachineConfig) {
		info := *cfg.StateInfo
		info.EntityName = "machine-0"
		cfg.StateInfo = &info
	}},
	{"password has disallowed characters", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo.Password = "'"
	}},
	{"password has disallowed characters", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo.Password = "\\"
	}},
	{"password has disallowed characters", func(cfg *cloudinit.MachineConfig) {
		cfg.StateInfo.Password = "\n"
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &cloudinit.MachineConfig{
		StateServer:        true,
		StateServerPEM:     serverPEM,
		InstanceIdAccessor: "$instance_id",
		ProviderType:       "ec2",
		MachineId:          99,
		Tools:              newSimpleTools("9.9.9-linux-arble"),
		AuthorizedKeys:     "sshkey1",
		StateInfo: &state.Info{
			Addrs: []string{"host"},
		},
		Config:  envConfig,
		DataDir: "/var/lib/juju",
	}
	// check that the base configuration does not give an error
	_, err := cloudinit.New(cfg)
	c.Assert(err, IsNil)

	for i, test := range verifyTests {
		c.Logf("test %d. %s", i, test.err)
		cfg1 := *cfg
		test.mutate(&cfg1)
		t, err := cloudinit.New(&cfg1)
		c.Assert(err, ErrorMatches, "invalid machine configuration: "+test.err)
		c.Assert(t, IsNil)
	}
}

var serverPEM = []byte(`
-----BEGIN CERTIFICATE-----
MIIBdzCCASOgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDgxNjIyMzRaFw0xMzExMDgxNjI3MzRaMBwx
DDAKBgNVBAoTA2htbTEMMAoGA1UEAxMDYW55MFowCwYJKoZIhvcNAQEBA0sAMEgC
QQCACqz6JPwM7nbxAWub+APpnNB7myckWJ6nnsPKi9SipP1hyhfzkp8RGMJ5Uv7y
8CSTtJ8kg/ibka1VV8LvP9tnAgMBAAGjUjBQMA4GA1UdDwEB/wQEAwIAsDAdBgNV
HQ4EFgQU6G1ERaHCgfAv+yoDMFVpDbLOmIQwHwYDVR0jBBgwFoAUP/mfUdwOlHfk
fR+gLQjslxf64w0wCwYJKoZIhvcNAQEFA0EAbn0MaxWVgGYBomeLYfDdb8vCq/5/
G/2iCUQCXsVrBparMLFnor/iKOkJB5n3z3rtu70rFt+DpX6L8uBR3LB3+A==
-----END CERTIFICATE-----
-----BEGIN RSA PRIVATE KEY-----
MIIBPAIBAAJBAIAKrPok/AzudvEBa5v4A+mc0HubJyRYnqeew8qL1KKk/WHKF/OS
nxEYwnlS/vLwJJO0nySD+JuRrVVXwu8/22cCAwEAAQJBAJsk1F0wTRuaIhJ5xxqw
FIWPFep/n5jhrDOsIs6cSaRbfIBy3rAl956pf/MHKvf/IXh7KlG9p36IW49hjQHK
7HkCIQD2CqyV1ppNPFSoCI8mSwO8IZppU3i2V4MhpwnqHz3H0wIhAIU5XIlhLJW8
TNOaFMEia/TuYofdwJnYvi9t0v4UKBWdAiEA76AtvjEoTpi3in/ri0v78zp2/KXD
JzPMDvZ0fYS30ukCIA1stlJxpFiCXQuFn0nG+jH4Q52FTv8xxBhrbLOFvHRRAiEA
2Vc9NN09ty+HZgxpwqIA1fHVuYJY9GMPG1LnTnZ9INg=
-----END RSA PRIVATE KEY-----
`
