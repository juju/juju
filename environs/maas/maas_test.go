package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"testing"
)

func TestMAAS(t *testing.T) {
	TestingT(t)
}

type _MAASProviderTestSuite struct {
	environ        *maasEnviron
	testMAASObject *gomaasapi.TestMAASObject
}

var _ = Suite(&_MAASProviderTestSuite{})

func (s *_MAASProviderTestSuite) SetUpSuite(c *C) {
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	s.environ = &maasEnviron{"test env", TestMAASObject}
}

func (s *_MAASProviderTestSuite) TearDownTest(c *C) {
	s.testMAASObject.TestServer.Clear()
}

func (s *_MAASProviderTestSuite) TearDownSuite(c *C) {
	s.testMAASObject.Close()
}

func (s *_MAASProviderTestSuite) TestId(c *C) {
	obj := s.environ.MAASServer.GetSubObject("nodes").GetSubObject("system_id")
	resourceURI, _ := obj.GetField("resource_uri")
	instance := maasInstance{&obj, s.environ}

	c.Check(string(instance.Id()), Equals, resourceURI)
}
