package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"testing"
)

func TestMAAS(t *testing.T) {
	TestingT(t)
}

type ProviderSuite struct {
	environ        *maasEnviron
	testMAASObject *gomaasapi.TestMAASObject
}

var _ = Suite(&ProviderSuite{})

func (s *ProviderSuite) SetUpSuite(c *C) {
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	s.environ = &maasEnviron{name: "test env", maasClientUnlocked: TestMAASObject}
}

func (s *ProviderSuite) TearDownTest(c *C) {
	s.testMAASObject.TestServer.Clear()
}

func (s *ProviderSuite) TearDownSuite(c *C) {
	s.testMAASObject.Close()
}
