package uniter_test

import (
	. "launchpad.net/gocheck"
)

type HookStateSuite struct{}

var _ = Suite(&HookStateSuite{})

func (s *HookStateSuite) TestFails(c *C) {
	c.Fatalf("rsahiuvfkd")
}
