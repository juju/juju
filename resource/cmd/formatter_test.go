// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/cmd"
)

var _ = gc.Suite(&FormatterSuite{})

type FormatterSuite struct {
	testing.IsolationSuite
}

func (s *FormatterSuite) TestFormatSpecOkay(c *gc.C) {
	spec := cmd.NewSpec(c, "spam", ".tgz", "X")
	formatted := cmd.FormatSpec(spec)

	c.Check(formatted, jc.DeepEquals, cmd.FormattedSpec{
		Name:     "spam",
		Type:     "file",
		Path:     "spam.tgz",
		Comment:  "X",
		Origin:   "upload",
		Revision: "",
	})
}
