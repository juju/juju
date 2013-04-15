package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type NewConnSuite struct {
	coretesting.LoggingSuite
}

var _ = Suite(&NewConnSuite{})

func (cs *NewConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewConnSuite) TestNewConnWithoutAdminSecret(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "really",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)

	delete(attrs, "admin-secret")
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	conn, err := juju.NewConn(env1)
	c.Check(conn, IsNil)
	c.Assert(err, ErrorMatches, "cannot connect without admin-secret")
}

func (*NewConnSuite) TestNewConnFromNameGetUnbootstrapped(c *C) {
	defer coretesting.MakeSampleHome(c).Restore()
	_, err := juju.NewConnFromName("")
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")
}

func bootstrapEnv(c *C, envName string) {
	environ, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, constraints.Value{})
	c.Assert(err, IsNil)
}

func (*NewConnSuite) TestConnMultipleCloseOk(c *C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "")
	// Error return from here is tested in TestNewConnFromNameNotSetGetsDefault.
	conn, _ := juju.NewConnFromName("")
	conn.Close()
	conn.Close()
	conn.Close()
}

func (*NewConnSuite) TestNewConnFromNameNotSetGetsDefault(c *C) {
	defer coretesting.MakeSampleHome(c).Restore()
	bootstrapEnv(c, "")
	conn, err := juju.NewConnFromName("")
	c.Assert(err, IsNil)
	defer conn.Close()
	c.Assert(conn.Environ.Name(), Equals, coretesting.SampleEnvName)
}

func (*NewConnSuite) TestNewConnFromNameNotDefault(c *C) {
	defer coretesting.MakeMultipleEnvHome(c).Restore()
	// The default environment is "erewhemos", so make sure we get what we ask for.
	const envName = "erewhemos-2"
	bootstrapEnv(c, envName)
	conn, err := juju.NewConnFromName(envName)
	c.Assert(err, IsNil)
	defer conn.Close()
	c.Assert(conn.Environ.Name(), Equals, envName)
}

func (cs *NewConnSuite) TestConnStateSecretsSideEffect(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "side-effect secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
	info, _, err := env.StateInfo()
	c.Assert(err, IsNil)
	info.Password = utils.PasswordHash("side-effect secret")
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, IsNil)

	// Verify we have no secret in the environ config
	cfg, err := st.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	cfg, err = conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
}

func (cs *NewConnSuite) TestConnStateDoesNotUpdateExistingSecrets(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "some secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)

	// Make a new Conn, which will push the secrets.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	// Make another env with a different secret.
	attrs["secret"] = "squirrel"
	env1, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)

	// Connect with the new env and check that the secret has not changed
	conn, err = juju.NewConn(env1)
	c.Assert(err, IsNil)
	defer conn.Close()
	cfg, err := conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
}

func (cs *NewConnSuite) TestConnWithPassword(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "squirrel",
		"admin-secret":    "nutkin",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	})
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)

	// Check that Bootstrap has correctly used a hash
	// of the admin password.
	info, _, err := env.StateInfo()
	c.Assert(err, IsNil)
	info.Password = utils.PasswordHash("nutkin")
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	st.Close()

	// Check that we can connect with the original environment.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	conn.Close()

	// Check that the password has now been changed to the original
	// admin password.
	info.Password = "nutkin"
	st1, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	st1.Close()

	// Check that we can still connect with the original
	// environment.
	conn, err = juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()

	// Reset the admin password so the state db can be reused.
	err = conn.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
}

type ConnSuite struct {
	coretesting.LoggingSuite
	coretesting.MgoSuite
	conn *juju.Conn
	repo *charm.LocalRepository
}

var _ = Suite(&ConnSuite{})

func (s *ConnSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"admin-secret":    "deploy-test-secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	environ, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, constraints.Value{})
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.repo = &charm.LocalRepository{Path: c.MkDir()}
}

func (s *ConnSuite) TearDownTest(c *C) {
	if s.conn == nil {
		return
	}
	err := s.conn.State.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
	err = s.conn.Environ.Destroy(nil)
	c.Check(err, IsNil)
	s.conn.Close()
	s.conn = nil
	dummy.Reset()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *ConnSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *ConnSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *ConnSuite) TestNewConnFromState(c *C) {
	conn, err := juju.NewConnFromState(s.conn.State)
	c.Assert(err, IsNil)
	c.Assert(conn.Environ.Name(), Equals, "erewhemos")
}

func (s *ConnSuite) TestPutCharmBasic(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	curl.Revision = -1 // make sure we trigger the repo.Latest logic.
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutBundledCharm(c *C) {
	// Bundle the riak charm into a charm repo directory.
	dir := filepath.Join(s.repo.Path, "series")
	err := os.Mkdir(dir, 0777)
	c.Assert(err, IsNil)
	w, err := os.Create(filepath.Join(dir, "riak.charm"))
	c.Assert(err, IsNil)
	defer w.Close()
	charmDir := coretesting.Charms.Dir("riak")
	err = charmDir.BundleTo(w)
	c.Assert(err, IsNil)

	// Invent a URL that points to the bundled charm, and
	// test putting that.
	curl := &charm.URL{
		Schema:   "local",
		Series:   "series",
		Name:     "riak",
		Revision: -1,
	}
	_, err = s.conn.PutCharm(curl, s.repo, true)
	c.Assert(err, ErrorMatches, `cannot increment revision of charm "local:series/riak-7": not a directory`)

	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (s *ConnSuite) TestPutCharmUpload(c *C) {
	repo := &charm.LocalRepository{c.MkDir()}
	curl := coretesting.Charms.ClonedURL(repo.Path, "series", "riak")

	// Put charm for the first time.
	sch, err := s.conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	sha256 := sch.BundleSha256()
	rev := sch.Revision()

	// Change the charm on disk.
	ch, err := repo.Get(curl)
	c.Assert(err, IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = s.conn.PutCharm(curl, repo, false)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Equals, sha256)
	c.Assert(sch.Revision(), Equals, rev)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.
	sch, err = s.conn.PutCharm(curl, repo, true)
	c.Assert(err, IsNil)

	sch, err = s.conn.State.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Not(Equals), sha256)
	c.Assert(sch.Revision(), Equals, rev+1)
}

func (s *ConnSuite) TestAddUnits(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.State.AddService("testriak", sch)
	c.Assert(err, IsNil)
	units, err := s.conn.AddUnits(svc, 2, "")
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 2)

	id0, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	id1, err := units[1].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id0, Not(Equals), id1)

	units, err = s.conn.AddUnits(svc, 2, "0")
	c.Assert(err, ErrorMatches, `cannot add multiple units of service "testriak" to a single machine`)

	units, err = s.conn.AddUnits(svc, 1, "0")
	c.Assert(err, IsNil)
	id2, err := units[0].AssignedMachineId()
	c.Assert(id2, Equals, id0)

}

type DeployLocalSuite struct {
	testing.JujuConnSuite
	repo          *charm.LocalRepository
	defaultSeries string
	seriesPath    string
	charmUrl      *charm.URL
}

var _ = Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	repoPath := c.MkDir()
	s.defaultSeries = "precise"
	s.repo = &charm.LocalRepository{Path: repoPath}
	s.seriesPath = filepath.Join(repoPath, s.defaultSeries)
	err := os.Mkdir(s.seriesPath, 0777)
	c.Assert(err, IsNil)
	coretesting.Charms.BundlePath(s.seriesPath, "mysql")
	s.charmUrl, err = charm.InferURL("local:mysql", s.defaultSeries)
	c.Assert(err, IsNil)
}

func (s *DeployLocalSuite) TestDeploy(c *C) {
	charm, err := s.Conn.PutCharm(s.charmUrl, s.repo, false)
	c.Assert(err, IsNil)
	cons := constraints.MustParse("mem=4G")
	args := juju.DeployServiceParams{
		Charm:       charm,
		NumUnits:    3,
		ServiceName: "bob",
		Constraints: cons,
	}
	svc, err := s.Conn.DeployService(args)
	c.Assert(err, IsNil)
	scons, err := svc.Constraints()
	c.Assert(err, IsNil)
	c.Assert(scons, DeepEquals, cons)

	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 3)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		machine, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		mcons, err := machine.Constraints()
		c.Assert(err, IsNil)
		c.Assert(mcons, DeepEquals, cons)
	}
}

type InitJujuHomeSuite struct {
	originalHome     string
	originalJujuHome string
}

var _ = Suite(&InitJujuHomeSuite{})

func (s *InitJujuHomeSuite) SetUpTest(c *C) {
	s.originalHome = os.Getenv("HOME")
	s.originalJujuHome = os.Getenv("JUJU_HOME")
}

func (s *InitJujuHomeSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.originalHome)
	os.Setenv("JUJU_HOME", s.originalJujuHome)
}

func (s *InitJujuHomeSuite) TestJujuHome(c *C) {
	os.Setenv("JUJU_HOME", "/my/juju/home")
	err := juju.InitJujuHome()
	c.Assert(err, IsNil)
	c.Assert(config.JujuHome(), Equals, "/my/juju/home")
}

func (s *InitJujuHomeSuite) TestHome(c *C) {
	os.Setenv("JUJU_HOME", "")
	os.Setenv("HOME", "/my/home/")
	err := juju.InitJujuHome()
	c.Assert(err, IsNil)
	c.Assert(config.JujuHome(), Equals, "/my/home/.juju")
}

func (s *InitJujuHomeSuite) TestError(c *C) {
	os.Setenv("JUJU_HOME", "")
	os.Setenv("HOME", "")
	err := juju.InitJujuHome()
	c.Assert(err, ErrorMatches, "cannot determine juju home.*")
}

func (s *InitJujuHomeSuite) TestCacheDir(c *C) {
	os.Setenv("JUJU_HOME", "/foo/bar")
	c.Assert(charm.CacheDir, Equals, "")
	err := juju.InitJujuHome()
	c.Assert(err, IsNil)
	c.Assert(charm.CacheDir, Equals, "/foo/bar/charmcache")
}
