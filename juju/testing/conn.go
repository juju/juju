package testing
import (
	"fmt"
	"net/url"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/state"
	"launchpad.net/juju-core/environs/dummy"
)

// StateSuite provides a pre-bootstrapped juju.Conn
// for for each test method.
type JujuConnSuite struct {
	testing.ZkSuite
	Conn *juju.Conn
	State *state.State
}

func (s *JujuConnSuite) SetUpTest(c *C) {
	config := map[string]interface{}{
		"name":            "foo",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	conn, err := juju.NewConnFromAttrs(config)
	c.Assert(err, IsNil)

	c.Assert(conn.Bootstrap(false), IsNil)
	s.Conn = conn
	s.State, err = conn.State()
	c.Assert(err, IsNil)
}

func (s *JujuConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	c.Assert(s.Conn.Close(), IsNil)
	s.Conn = nil
	s.State = nil
}

func (s *JujuConnSuite) StateInfo(c *C) *state.Info {
	return &state.Info{Addrs: []string{testing.ZkAddr}}
}

func (s *JujuConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}
