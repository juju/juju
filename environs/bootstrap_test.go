package environs_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"time"
)

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	home testing.FakeHome
	testing.LoggingSuite
}

var _ = Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "foo")
}

func (s *bootstrapSuite) TearDownTest(c *C) {
	s.home.Restore()
}

func (s *bootstrapSuite) TestBootstrapNeedsConfigCert(c *C) {
	env := newEnviron("bar", noKeysDefined)
	err := environs.Bootstrap(env, state.Constraints{}, false)
	c.Assert(err, ErrorMatches, "environment configuration missing CA certificate")
}

func (s *bootstrapSuite) TestBootstrapKeyGeneration(c *C) {
	env := newEnviron("foo", useDefaultKeys)
	err := environs.Bootstrap(env, state.Constraints{}, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	caCertPEM, err := ioutil.ReadFile(testing.HomePath(".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)

	err = cert.Verify(env.certPEM, caCertPEM, time.Now())
	c.Assert(err, IsNil)
	err = cert.Verify(env.certPEM, caCertPEM, time.Now().AddDate(9, 0, 0))
	c.Assert(err, IsNil)
}

func (s *bootstrapSuite) TestBootstrapUploadTools(c *C) {
	env := newEnviron("foo", useDefaultKeys)
	err := environs.Bootstrap(env, state.Constraints{}, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, false)

	env = newEnviron("foo", useDefaultKeys)
	err = environs.Bootstrap(env, state.Constraints{}, true)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, true)
}

func (s *bootstrapSuite) TestBootstrapConstraints(c *C) {
	env := newEnviron("foo", useDefaultKeys)
	err := environs.Bootstrap(env, state.Constraints{}, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.constraints, DeepEquals, state.Constraints{})

	env = newEnviron("foo", useDefaultKeys)
	cons, err := state.ParseConstraints("cpu-cores=2 mem=4G")
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, cons, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.constraints, DeepEquals, cons)
}

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	constraints    state.Constraints
	uploadTools    bool
	certPEM        []byte
	keyPEM         []byte
}

func newEnviron(name string, defaultKeys bool) *bootstrapEnviron {
	m := map[string]interface{}{
		"name":            name,
		"type":            "test",
		"authorized-keys": "foo",
		"ca-cert":         "",
		"ca-private-key":  "",
	}
	if defaultKeys {
		m["ca-cert"] = testing.CACert
		m["ca-private-key"] = testing.CAKey
	}
	cfg, err := config.New(m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		name: name,
		cfg:  cfg,
	}
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(cons state.Constraints, uploadTools bool, certPEM, keyPEM []byte) error {
	e.bootstrapCount++
	e.constraints = cons
	e.uploadTools = uploadTools
	e.certPEM = certPEM
	e.keyPEM = keyPEM
	return nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}
