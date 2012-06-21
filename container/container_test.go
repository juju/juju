package container_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/charm"
	"launchpad.net/juju-core/juju/container"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/testing"
	"net/url"
	"os"
	"path/filepath"
	stdtesting "testing"
)

type suite struct {
	state *state.State
}

var zkServer *zookeeper.Server

var _ = Suite(&suite{})

func Test(t *stdtesting.T) {
	zkServer = testing.StartZkServer()
	defer zkServer.Destroy()
	TestingT(t)
}

func (s *suite) SetUpSuite(c *C) {
	addr, err := zkServer.Addr()
	c.Assert(err, IsNil)
	s.state, err = state.Initialize(&state.Info{
		Addrs: []string{addr},
	})
	c.Assert(err, IsNil)
}

func (s *suite) TestDeploy(c *C) {
	// make sure there's a jujud "executable" in the path.
	binDir := c.MkDir()
	exe := filepath.Join(binDir, "jujud")
	defer os.Setenv("PATH", os.Getenv("PATH"))
	os.Setenv("PATH", binDir)
	err := ioutil.WriteFile(exe, nil, 0777)
	c.Assert(err, IsNil)

	// create a unit to deploy
	dummyCharm := testing.Charms.Dir("dummy")
	u := fmt.Sprintf("local:series/%s-%d", dummyCharm.Meta().Name, dummyCharm.Revision())
	curl := charm.MustParseURL(u)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.state.AddCharm(dummyCharm, curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	service, err := s.state.AddService("dummy", dummy)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	oldInitDir, oldJujuDir := *container.InitDir, *container.JujuDir
	defer func() {
		*container.InitDir, *container.JujuDir = oldInitDir, oldJujuDir
	}()
	*container.InitDir, *container.JujuDir = c.MkDir(), c.MkDir()

	unitName := "juju-agent-dummy-0"
	upstartScript := filepath.Join(*container.InitDir, unitName+".conf")

	unitDir := filepath.Join(*container.JujuDir, "units", "dummy-0")

	cont := container.Simple
	err = cont.Deploy(unit)
	c.Assert(err, ErrorMatches, `(.|\n)+Unknown job(.|\n)+`)

	data, err := ioutil.ReadFile(upstartScript)
	c.Assert(err, IsNil)
	c.Assert(string(data), Matches, `(.|\n)+`+regexp.QuotaMeta(exe)+` unit --unit-name(.|\n)+`)

	// We can't check that the unit directory is created, because
	// it is removed when the call to Deploy fails, but
	// we can check that it is removed.

	err = os.MkdirAll(filepath.Join(unitDir, "foo"), 0777)
	c.Assert(err, IsNil)

	err = cont.Destroy(unit)
	c.Assert(err, IsNil)

	_, err = os.Stat(unitDir)
	c.Assert(err, NotNil)

	_, err = os.Stat(upstartScript)
	c.Assert(err, NotNil)
}
