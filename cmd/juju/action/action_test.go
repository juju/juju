// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (

	gc "gopkg.in/check.v1"

)

type ActionCommandSuite struct {
	BaseActionSuite
}

var _ = gc.Suite(&ActionCommandSuite{})

func (s *ActionCommandSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
}

