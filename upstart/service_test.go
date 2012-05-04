package upstart_test

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type ServiceSuite struct{}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) TestFails(c *C) {
	c.Fatalf("blam")
}
