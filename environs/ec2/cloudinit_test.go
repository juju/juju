package ec2

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"regexp"
	"strings"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct{}

var _ = Suite(cloudinitSuite{})

// Each test gives a cloudinit config - we check the
// output to see if it looks correct.
var cloudinitTests = []machineConfig{
	{
		instanceIdAccessor: "$instance_id",
		machineId:          0,
		providerType:       "ec2",
		provisioner:        true,
		authorizedKeys:     "sshkey1",
		tools:              newTools("1.2.3-linux-amd64"),
		zookeeper:          true,
	},
	{
		machineId:      99,
		providerType:   "ec2",
		provisioner:    false,
		authorizedKeys: "sshkey1",
		zookeeper:      false,
		tools:          newTools("1.2.3-linux-amd64"),
		stateInfo:      &state.Info{Addrs: []string{"zk1"}},
	},
}

func newTools(vers string) *state.Tools {
	return &state.Tools{
		URL:    "http://foo.com/tools/juju" + vers + ".tgz",
		Binary: version.MustParseBinary(vers),
	}
}

// cloundInitTest runs a set of tests for one of the machineConfig
// values above.
type cloudinitTest struct {
	x   map[interface{}]interface{} // the unmarshalled YAML.
	cfg *machineConfig              // the config being tested.
}

func (t *cloudinitTest) check(c *C) {
	c.Check(t.x["apt_upgrade"], Equals, true)
	c.Check(t.x["apt_update"], Equals, true)
	t.checkScripts(c, "mkdir -p "+environs.VarDir)
	t.checkScripts(c, "wget.*"+regexp.QuoteMeta(t.cfg.tools.URL)+".*tar .*xz")

	if t.cfg.zookeeper {
		t.checkPackage(c, "zookeeperd")
		t.checkScripts(c, "jujud bootstrap-state")
		t.checkScripts(c, regexp.QuoteMeta(t.cfg.instanceIdAccessor))
		t.checkScripts(c, "JUJU_ZOOKEEPER='localhost"+zkPortSuffix+"'")
	} else {
		t.checkScripts(c, "JUJU_ZOOKEEPER='"+strings.Join(t.cfg.stateInfo.Addrs, ",")+"'")
	}
	t.checkPackage(c, "libzookeeper-mt2")
	t.checkScripts(c, "JUJU_MACHINE_ID=[0-9]+")

	if t.cfg.provisioner {
		t.checkScripts(c, "jujud provisioning --zookeeper-servers 'localhost"+zkPortSuffix+"'")
	}

	if t.cfg.zookeeper {
		t.checkScripts(c, "jujud machine --zookeeper-servers 'localhost"+zkPortSuffix+"' .* --machine-id [0-9]+")
	} else {
		t.checkScripts(c, "jujud machine --zookeeper-servers '"+strings.Join(t.cfg.stateInfo.Addrs, ",")+"' .* --machine-id [0-9]+")
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
		ci, err := newCloudInit(&cfg)
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

// When mutate is called on a known-good machineConfig,
// there should be an error complaining about the missing
// field named by the adjacent err.
var verifyTests = []struct {
	err    string
	mutate func(*machineConfig)
}{
	{"negative machine id", func(cfg *machineConfig) { cfg.machineId = -1 }},
	{"missing provider type", func(cfg *machineConfig) { cfg.providerType = "" }},
	{"missing instance id accessor", func(cfg *machineConfig) {
		cfg.zookeeper = true
		cfg.instanceIdAccessor = ""
	}},
	{"missing zookeeper hosts", func(cfg *machineConfig) {
		cfg.zookeeper = false
		cfg.stateInfo = nil
	}},
	{"missing zookeeper hosts", func(cfg *machineConfig) {
		cfg.zookeeper = false
		cfg.stateInfo = &state.Info{}
	}},
	{"missing tools", func(cfg *machineConfig) {
		cfg.tools = nil
		cfg.stateInfo = &state.Info{}
	}},
	{"missing tools URL", func(cfg *machineConfig) {
		cfg.tools = &state.Tools{}
		cfg.stateInfo = &state.Info{}
	}},
}

// TestCloudInitVerify checks that required fields are appropriately
// checked for by newCloudInit.
func (cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &machineConfig{
		provisioner:        true,
		zookeeper:          true,
		instanceIdAccessor: "$instance_id",
		providerType:       "ec2",
		machineId:          99,
		tools:              newTools("9.9.9-linux-arble"),
		authorizedKeys:     "sshkey1",
		stateInfo:          &state.Info{Addrs: []string{"zkhost"}},
	}
	// check that the base configuration does not give an error
	_, err := newCloudInit(cfg)
	c.Assert(err, IsNil)

	for _, test := range verifyTests {
		cfg1 := *cfg
		test.mutate(&cfg1)
		t, err := newCloudInit(&cfg1)
		c.Assert(err, ErrorMatches, "invalid machine configuration: "+test.err)
		c.Assert(t, IsNil)
	}
}
