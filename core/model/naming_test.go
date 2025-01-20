// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"regexp"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/testing"
)

type NamingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&NamingSuite{})

func (*NamingSuite) TestDisambiguateName(c *gc.C) {
	for _, t := range []struct {
		name      string
		result    string
		maxLength uint
		err       string
	}{
		{"gitlab", "gitlab-06f00d", 63, ""},
		{"someverylongresourcename", "someveryl-06f00d", 16, ""},
		{"gitlab", "", 10, "maxLength (10) must be greater than 16"},
	} {
		result, err := model.DisambiguateResourceName(coretesting.ModelTag.Id(), t.name, t.maxLength)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(t.err))
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(result, gc.Equals, t.result)
		}
	}
}
