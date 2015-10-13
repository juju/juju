// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"path"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/meterstatus"
)

type StateFileSuite struct {
	path  string
	state *meterstatus.StateFile
}

var _ = gc.Suite(&StateFileSuite{})

func (t *StateFileSuite) SetUpTest(c *gc.C) {
	t.path = path.Join(c.MkDir(), "state.yaml")
	t.state = meterstatus.NewStateFile(t.path)
}

func (t *StateFileSuite) TestReadNonExist(c *gc.C) {
	code, info, err := t.state.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(code, gc.Equals, "")
	c.Assert(info, gc.Equals, "")
}

func (t *StateFileSuite) TestWriteRead(c *gc.C) {
	code := "GREEN"
	info := "some message"
	err := t.state.Write(code, info)
	c.Assert(err, jc.ErrorIsNil)

	rCode, rInfo, err := t.state.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rCode, gc.Equals, code)
	c.Assert(rInfo, gc.Equals, info)
}
