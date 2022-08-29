// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/secrets"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registrySuite{})

func (*registrySuite) TestStore(c *gc.C) {
	_, err := secrets.Store("bad")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = secrets.Store("juju")
	c.Assert(err, jc.ErrorIsNil)
}
