// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

var _ = gc.Suite(&FormatterSuite{})

type FormatterSuite struct {
	testing.IsolationSuite
}

func (s *FormatterSuite) TestFormatInfoOkay(c *gc.C) {
	data := []byte("spamspamspam")
	fp, err := charmresource.GenerateFingerprint(data)
	c.Assert(err, jc.ErrorIsNil)
	fingerprint := string(fp.Bytes())
	res := newCharmResource(c, "spam", ".tgz", "X", fingerprint)
	formatted := FormatCharmResource(res)

	c.Check(formatted, jc.DeepEquals, FormattedCharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Comment:     "X",
		Revision:    0,
		Fingerprint: fp.String(),
		Origin:      "upload",
	})
}
