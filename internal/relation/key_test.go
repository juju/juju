// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type relationKeySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&relationKeySuite{})

func (*relationKeySuite) TestParseKeyFromTagString(c *gc.C) {
	relationTag := names.NewRelationTag("mysql:database wordpress:mysql")
	key, err := ParseKeyFromTagString(relationTag.String())
	c.Assert(err, gc.IsNil)
	c.Check(key, jc.DeepEquals, Key(relationTag.Id()))
}

func (*relationKeySuite) TestParseKeyFromTagStringFails(c *gc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	_, err := ParseKeyFromTagString(unitTag.String())
	c.Check(err, gc.ErrorMatches, `"unit-mysql-0" is not a valid relation tag`)

	_, err = ParseKeyFromTagString("")
	c.Check(err, gc.ErrorMatches, `"" is not a valid tag`)
}
