// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"path/filepath"

	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator"
)

type LocalStateFileSuite struct{}

var _ = gc.Suite(&LocalStateFileSuite{})

func (s *LocalStateFileSuite) TestState(c *gc.C) {
	path := filepath.Join(c.MkDir(), "operator")
	file := caasoperator.NewStateFile(path)
	_, err := file.Read()
	c.Assert(err, gc.Equals, caasoperator.ErrNoStateFile)

	localSt := caasoperator.LocalState{
		CharmURL:             charm.MustParseURL("cs:quantal/application-name-123"),
		CharmModifiedVersion: 123,
	}
	err = file.Write(&localSt)
	c.Assert(err, jc.ErrorIsNil)
	st, err := file.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st, jc.DeepEquals, &localSt)
}
