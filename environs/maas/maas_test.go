package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestMAAS(t *stdtesting.T) {
	TestingT(t)
}

type ProviderSuite struct {
	testing.LoggingSuite
	environ        *maasEnviron
	testMAASObject *gomaasapi.TestMAASObject
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	s.environ = &maasEnviron{name: "test env", maasClientUnlocked: &TestMAASObject.MAASObject}
}

func (s *ProviderSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
}

func (s *ProviderSuite) TearDownTest(c *C) {
	s.testMAASObject.TestServer.Clear()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ProviderSuite) TearDownSuite(c *C) {
	s.testMAASObject.Close()
	s.LoggingSuite.TearDownSuite(c)
}
