package ec2

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"regexp"
)

// Use local suite since this file lives in the ec2 package
// for testing internals.
type cloudinitSuite struct{}

var _ = Suite(cloudinitSuite{})

var verifyTests = []struct {
	err    string
	mutate func(*cloudConfig)
}{
	{"machine id", func(cfg *cloudConfig) { cfg.machineId = "" }},
	{"provider type", func(cfg *cloudConfig) { cfg.providerType = "" }},
	{"instance id accessor", func(cfg *cloudConfig) {
		cfg.zookeeper = true
		cfg.instanceIdAccessor = ""
	}},
	{"admin secret", func(cfg *cloudConfig) {
		cfg.zookeeper = true
		cfg.adminSecret = ""
	}},
	{"zookeeper hosts", func(cfg *cloudConfig) {
		cfg.zookeeper = false
		cfg.zookeeperHosts = nil
	}},
}

var cloudinitTests = []cloudConfig{
	{
		adminSecret:        "topsecret",
		instanceIdAccessor: "$instance_id",
		machineId:          "aMachine",
		origin:             &jujuOrigin{originBranch, "lp:jujubranch"},
		providerType:       "ec2",
		provisioner:        true,
		sshKeys:            []string{"sshkey1"},
		zookeeper:          true,
	},
	{
		adminSecret:    "topsecret",
		machineId:      "aMachine",
		origin:         &jujuOrigin{originDistro, ""},
		providerType:   "ec2",
		provisioner:    false,
		sshKeys:        []string{"sshkey1"},
		zookeeper:      false,
		zookeeperHosts: []string{"zk1"},
	},
}

// cloundInitCheck runs a set of tests for one of the cloudConfig
// values above.
type cloudinitCheck struct {
	c   *C
	x   map[interface{}]interface{}		// the unmarshalled YAML.
	cfg *cloudConfig			// the config being tested.
	i   int							// the index of the test in cloudinitTests.
}

// addBug takes any "bug" info provided to cloudinitCheck.Assert or
// cloudinitCheck.Check and adds contextual information.
func (c *cloudinitCheck) addBug(args []interface{}) []interface{} {
	args = append([]interface{}{}, args...)
	bug := Bug("check %d; result %v", c.i, c.x)
	if n := len(args); n > 0 {
		if b, _ := args[n-1].(BugInfo); b != nil {
			bug = Bug("check %d: %v; result %v", c.i, b.GetBugInfo(), c.x)
			args = args[:n-1]
		}
	}
	return append(args, bug)
}

func (c *cloudinitCheck) Assert(obtained interface{}, checker Checker, args ...interface{}) {
	c.c.Assert(obtained, checker, c.addBug(args)...)
}

func (c *cloudinitCheck) Check(obtained interface{}, checker Checker, args ...interface{}) bool {
	return c.c.Check(obtained, checker, c.addBug(args)...)
}

func (c *cloudinitCheck) check() {
	c.checkPackage("bzr")
	c.checkOption("apt_upgrade", true)
	c.checkOption("apt_update", true)
	c.checkScripts("python -m agents.machine")
	c.checkScripts("mkdir -p /var/lib/juju")
	c.checkMachineData()

	if c.cfg.zookeeper {
		c.checkPackage("zookeeperd")
		c.checkScripts("juju-admin initialize")
		c.checkScripts(regexp.QuoteMeta(c.cfg.instanceIdAccessor))
	}
	if c.cfg.origin != nil && c.cfg.origin.origin == originDistro {
		c.checkScripts("apt-get.*install juju")
	}
	if c.cfg.provisioner {
		c.checkScripts("python -m agents.provision")
	}
}

func (c *cloudinitCheck) checkMachineData() {
	mdata0 := c.x["machine-data"]
	c.Assert(mdata0, NotNil)
	mdata, ok := mdata0.(map[interface{}]interface{})
	c.Assert(ok, Equals, true)

	m := mdata["machine-id"]
	c.Assert(m, Equals, c.cfg.machineId)
}

func (c *cloudinitCheck) checkOption(name string, value interface{}) {
	c.Check(c.x[name], Equals, value, Bug("option %q", name))
}

func (c *cloudinitCheck) checkScripts(pattern string) {
	scripts0 := c.x["runcmd"]
	if !c.Check(scripts0, NotNil, Bug("cloudinit has no entry for runcmd")) {
		return
	}
	scripts, ok := scripts0.([]interface{})
	if !c.Check(ok, Equals, true, Bug("runcmd entry is wrong type; got %T want []interface{}", scripts0)) {
		return
	}
	re := regexp.MustCompile(pattern)
	found := false
	for _, s0 := range scripts {
		s, ok := s0.(string)
		if !c.Check(ok, Equals, true, Bug("script entry has unexpected type %T want string", s0)) {
			continue
		}
		if re.MatchString(s) {
			found = true
		}
	}
	c.Check(found, Equals, true, Bug("script %q not found", pattern))
}

func (c *cloudinitCheck) checkPackage(pkg string) {
	pkgs0 := c.x["packages"]
	if !c.Check(pkgs0, NotNil, Bug("cloudinit has no entry for packages")) {
		return
	}

	pkgs, ok := pkgs0.([]interface{})
	if !c.Check(ok, Equals, true, Bug("cloudinit packages entry is wrong type; got %T want []interface{}", pkgs0)) {
		return
	}

	found := false
	for _, p0 := range pkgs {
		p, ok := p0.(string)
		c.Check(ok, Equals, true, Bug("cloudinit package entry is wrong type; got %T want string", p0))
		if p == pkg {
			found = true
		}
	}
	c.Check(found, Equals, true, Bug("%q not found in packages", pkg))
}

func (cloudinitSuite) TestCloudInit(c *C) {
	for i, cfg := range cloudinitTests {
		t, err := newCloudInit(&cfg)
		ch := &cloudinitCheck{
			c:   c,
			cfg: &cfg,
			i:   i,
		}
		ch.Assert(err, IsNil)
		ch.Check(t, NotNil)

		data, err := t.Render()
		ch.Assert(err, IsNil)

		x := make(map[interface{}]interface{})
		err = goyaml.Unmarshal(data, &x)
		ch.Assert(err, IsNil)

		ch.x = x
		ch.check()
	}
}

func (cloudinitSuite) TestCloudInitVerify(c *C) {
	cfg := &cloudConfig{
		provisioner:        true,
		zookeeper:          true,
		instanceIdAccessor: "$instance_id",
		adminSecret:        "topsecret",
		providerType:       "ec2",
		origin:             &jujuOrigin{originBranch, "lp:jujubranch"},
		machineId:          "aMachine",
		sshKeys:            []string{"sshkey1"},
		zookeeperHosts:     []string{"zkhost"},
	}
	// check that the base configuration does not give an error
	_, err := newCloudInit(cfg)
	c.Assert(err, IsNil)

	for _, test := range verifyTests {
		cfg1 := *cfg
		test.mutate(&cfg1)
		t, err := newCloudInit(&cfg1)
		c.Assert(err, ErrorMatches, "cloud configuration requires "+test.err)
		c.Assert(t, IsNil)
	}
}
