package schema_test

import (
	"testing"
	"launchpad.net/gocheck"
	"launchpad.net/ensemble/go/schema"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(S{})

func (s *S) TestEmpty(c *gocheck.C) {
	_ = schema.Nothing
}
