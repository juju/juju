package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}

type ConnSuite struct{
	testing.ZkSuite
}

var _ = Suite(ConnSuite{})

func (ConnSuite) TestNewConn(c *C) {
	home := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)
	conn, err := juju.NewConn("")
	c.Assert(conn, IsNil)
	c.Assert(err, ErrorMatches, ".*: no such file or directory")

	if err := os.Mkdir(filepath.Join(home, ".juju"), 0755); err != nil {
		c.Log("Could not create directory structure")
		c.Fail()
	}
	envs := filepath.Join(home, ".juju", "environments.yaml")
	err = ioutil.WriteFile(envs, []byte(`
default:
    erewhemos
environments:
    erewhemos:
        type: dummy
        zookeeper: true
        authorized-keys: i-am-a-key
`), 0644)
	if err != nil {
		c.Log("Could not create environments.yaml")
		c.Fail()
	}

	// Just run through a few operations on the dummy provider and verify that
	// they behave as expected.
	conn, err = juju.NewConn("")
	c.Assert(err, IsNil)
	defer conn.Close()
	st, err := conn.State()
	c.Assert(st, IsNil)
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")
	err = conn.Bootstrap(false)
	c.Assert(err, IsNil)
	defer conn.Destroy()
	st, err = conn.State()
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	err = conn.Destroy()
	c.Assert(err, IsNil)

	// Close the conn (thereby closing its state) a couple of times to
	// verify that multiple closes are safe.
	c.Assert(conn.Close(), IsNil)
	c.Assert(conn.Close(), IsNil)
}

func newConn(c *C) *juju.Conn {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	conn, err := juju.NewConnFromAttrs(attrs)
	c.Assert(err, IsNil)
	return conn
}

func (ConnSuite) TestNewConnFromAttrs(c *C) {
	conn := newConn(c)
	defer conn.Close()
	st, err := conn.State()
	c.Assert(st, IsNil)
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")
}

func (ConnSuite) TestPutCharmBasic(c *C) {
	conn := newConn(c)
	defer conn.Close()
	err := conn.Bootstrap(false)
	c.Assert(err, IsNil)
	defer conn.Destroy()
	repoPath := c.MkDir()
	curl := testing.Charms.ClonedURL(repoPath, "riak")
	curl.Revision = -1			// make sure we trigger the repo.Latest logic.
	sch, err := conn.PutCharm(curl, repoPath, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	state, err := conn.State()
	c.Assert(err, IsNil)
	sch, err = state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}

func (ConnSuite) TestPutBundledCharm(c *C) {
	conn := newConn(c)
	defer conn.Close()
	err := conn.Bootstrap(false)
	c.Assert(err, IsNil)
	defer conn.Destroy()

	// Bundle the riak charm into a charm repo directory.
	repoPath := c.MkDir()
	dir := filepath.Join(repoPath, "series")
	err = os.Mkdir(dir, 0777)
	c.Assert(err, IsNil)
	w, err := os.Create(filepath.Join(dir, "riak.charm"))
	c.Assert(err, IsNil)
	defer w.Close()
	charmDir := testing.Charms.Dir("riak")
	err = charmDir.BundleTo(w)
	c.Assert(err, IsNil)

	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		c.Logf("%s %v", path, info.IsDir())
		return nil
	})
	// Invent a URL that points to the bundled charm, and
	// test putting that.
	curl := &charm.URL{
		Schema: "local",
		Series: "series",
		Name: "riak",
		Revision: -1,
	}
	sch, err := conn.PutCharm(curl, repoPath, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	// Check that we can get the charm from the state.
	state, err := conn.State()
	c.Assert(err, IsNil)
	sch, err = state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")
}


func (ConnSuite) TestPutCharmBumpRevision(c *C) {
	conn := newConn(c)
	defer conn.Close()
	err := conn.Bootstrap(false)
	c.Assert(err, IsNil)
	defer conn.Destroy()
	repo := &charm.LocalRepository{c.MkDir()}
	curl := testing.Charms.ClonedURL(repo.Path, "riak")

	// Put charm for the first time.
	sch, err := conn.PutCharm(curl, repo.Path, false)
	c.Assert(err, IsNil)
	c.Assert(sch.Meta().Summary, Equals, "K/V storage engine")

	state, err := conn.State()
	c.Assert(err, IsNil)
	sch, err = state.Charm(sch.URL())
	c.Assert(err, IsNil)
	sha256 := sch.BundleSha256()

	// Change the charm on disk.
	ch, err := repo.Get(curl)
	c.Assert(err, IsNil)
	chd := ch.(*charm.Dir)
	err = ioutil.WriteFile(filepath.Join(chd.Path, "extra"), []byte("arble"), 0666)
	c.Assert(err, IsNil)

	// Put charm again and check that it has not changed in the state.
	sch, err = conn.PutCharm(curl, repo.Path, false)
	c.Assert(err, IsNil)

	sch, err = state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Equals, sha256)

	// Put charm again, with bumpRevision this time, and check that
	// it has changed.

	sch, err = conn.PutCharm(curl, repo.Path, true)
	c.Assert(err, IsNil)

	sch, err = state.Charm(sch.URL())
	c.Assert(err, IsNil)
	c.Assert(sch.BundleSha256(), Not(Equals), sha256)
}

func (ConnSuite) TestValidRegexps(c *C) {
	assertService := func(s string, expect bool) {
		c.Assert(juju.ValidService.MatchString(s), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/0"), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/99"), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/-1"), Equals, false)
		c.Assert(juju.ValidUnit.MatchString(s+"/blah"), Equals, false)
	}
	assertService("", false)
	assertService("33", false)
	assertService("wordpress", true)
	assertService("w0rd-pre55", true)
	assertService("foo2", true)
	assertService("foo-2", false)
	assertService("foo-2foo", true)
}
