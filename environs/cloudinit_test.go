package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/environs"
	"regexp"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct{}

var _ = Suite(cloudinitSuite{})

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []environs.MachineConfig{
	{
		InstanceIdAccessor: "$instance_id",
		MachineId:          0,
		ProviderType:       "ec2",
		Provisioner:        true,
		AuthorizedKeys:     "sshkey1",
		Tools:              newSimpleTools("1.2.3-linux-amd64"),
		Zookeeper:          true,
	},
	{
		MachineId:      99,
		ProviderType:   "ec2",
		Provisioner:    false,
		AuthorizedKeys: "sshkey1",
		Zookeeper:      false,
		Tools:          newSimpleTools("1.2.3-linux-amd64"),
		StateInfo:      &state.Info{Addrs: []string{"zk1"}},
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
	cfg *environs.MachineConfig              // the config being tested.
}

func (t *cloudinitTest) check(c *C) {
	c.Check(t.x["apt_upgrade"], Equals, true)
	c.Check(t.x["apt_update"], Equals, true)
	t.checkScripts(c, "mkdir -p "+environs.VarDir)
	t.checkScripts(c, "wget.*"+regexp.QuoteMeta(t.cfg.Tools.URL)+".*tar .*xz")

	if t.cfg.Zookeeper {
		t.checkPackage(c, "zookeeperd")
		t.checkScripts(c, "jujud initzk")
		t.checkScripts(c, regexp.QuoteMeta(t.cfg.InstanceIdAccessor))
		t.checkScripts(c, "JUJU_ZOOKEEPER='localhost"+environs.ZkPortSuffix+"'")
	} else {
		t.checkScripts(c, "JUJU_ZOOKEEPER='"+strings.Join(t.cfg.StateInfo.Addrs, ",")+"'")
	}
	t.checkPackage(c, "libzookeeper-mt2")
	t.checkScripts(c, "JUJU_MACHINE_ID=[0-9]+")

	if t.cfg.Provisioner {
		t.checkScripts(c, "jujud provisioning --zookeeper-servers 'localhost"+environs.ZkPortSuffix+"'")
	}

	if t.cfg.Zookeeper {
		t.checkScripts(c, "jujud machine --zookeeper-servers 'localhost"+environs.ZkPortSuffix+"' .* --machine-id [0-9]+")
	} else {
		t.checkScripts(c, "jujud machine --zookeeper-servers '"+strings.Join(t.cfg.StateInfo.Addrs, ",")+"' .* --machine-id [0-9]+")
	}
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
		c.Logf("check %d", i)
		ci, err := environs.NewCloudInit(&cfg)
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
		t.check(c)
	}
}

// When mutate is called on a known-good MachineConfig,
// there should be an error complaining about the missing
// field named by the adjacent err.
var verifyTests = []struct {
	err    string
	mutate func(*environs.MachineConfig)
}{
	{"negative machine id", func(cfg *environs.MachineConfig) { cfg.MachineId = -1 }},
	{"missing provider type", func(cfg *environs.MachineConfig) { cfg.ProviderType = "" }},
	{"missing instance id accessor", func(cfg *environs.MachineConfig) {
		cfg.Zookeeper = true
		cfg.InstanceIdAccessor = ""
	}},
	{"missing zookeeper hosts", func(cfg *environs.MachineConfig) {
		cfg.Zookeeper = false
		cfg.StateInfo = nil
	}},
	{"missing zookeeper hosts", func(cfg *environs.MachineConfig) {
		cfg.Zookeeper = false
		cfg.StateInfo = &state.Info{}
	}},
	{"missing tools", func(cfg *environs.MachineConfig) {
		cfg.Tools = nil
		cfg.StateInfo = &state.Info{}
	}},
	{"missing tools URL", func(cfg *environs.MachineConfig) {
		cfg.Tools = &state.Tools{}
		cfg.StateInfo = &state.Info{}
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by NewCloudInit.
func (cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &environs.MachineConfig{
		Provisioner:        true,
		Zookeeper:          true,
		InstanceIdAccessor: "$instance_id",
		ProviderType:       "ec2",
		MachineId:          99,
		Tools:              newSimpleTools("9.9.9-linux-arble"),
		AuthorizedKeys:     "sshkey1",
		StateInfo:          &state.Info{Addrs: []string{"zkhost"}},
	}
	// check that the base configuration does not give an error
	_, err := environs.NewCloudInit(cfg)
	c.Assert(err, IsNil)

	for _, test := range verifyTests {
		cfg1 := *cfg
		test.mutate(&cfg1)
		t, err := environs.NewCloudInit(&cfg1)
		c.Assert(err, ErrorMatches, "invalid machine configuration: "+test.err)
		c.Assert(t, IsNil)
	}
}
