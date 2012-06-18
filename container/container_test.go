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

	*container.InitDir = c.MkDir()

	upstartScript := filepath.Join(*container.InitDir, "juju-agent-dummy-0.conf")

	cont := container.Simple(unit)
	err = cont.Deploy()
	c.Assert(err, ErrorMatches, `(.|\n)+Unknown job(.|\n)+`)

	data, err := ioutil.ReadFile(upstartScript)
	c.Assert(err, IsNil)
	c.Assert(string(data), Matches, `(.|\n)+unit --unit-name(.|\n)+`)

	err = cont.Destroy()
	c.Assert(err, IsNil)

	_, err = os.Stat(upstartScript)
	c.Assert(err, NotNil)
}
