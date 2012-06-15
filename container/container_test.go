package container_test
import (
	. "launchpad.net/gocheck"
	stdtesting "testing"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/charm"
	"launchpad.net/juju-core/juju/container"
	"launchpad.net/gozk/zookeeper"
	"net/url"
	"fmt"
	"strings"
	"path/filepath"
	"time"
	"io/ioutil"
	"os"
)

type suite struct{
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

var script = `#!/bin/bash
echo > $DIR/start
sleep 1
echo > $DIR/end
`

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

	// mock jujud
	dir := c.MkDir()
	path := os.Getenv("PATH")
	defer os.Setenv("PATH", path)
	os.Setenv("PATH", dir+":"+path)
	data := []byte(strings.Replace(script, "$DIR", dir, -1))
	err = ioutil.WriteFile(filepath.Join(dir, "jujud"), data, 0777)
	c.Assert(err, IsNil)

	cont := container.Simple(unit)
	err = cont.Deploy()
	c.Assert(err, IsNil)

	time.Sleep(500 * time.Millisecond)
	_, err = os.Stat(filepath.Join(dir, "start"))
	c.Assert(err, IsNil, Commentf("jujud has not run"))

	err = cont.Destroy()
	c.Assert(err, IsNil)

	time.Sleep(1 * time.Second)
	_, err = os.Stat(filepath.Join(dir, "end"))
	c.Assert(err, NotNil)
}
