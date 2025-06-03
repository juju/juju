// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"regexp"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type NamingSuite struct {
	testhelpers.IsolationSuite
}

func TestNamingSuite(t *testing.T) {
	tc.Run(t, &NamingSuite{})
}

func (*NamingSuite) TestDisambiguateName(c *tc.C) {
	for _, t := range []struct {
		name      string
		result    string
		maxLength uint
		err       string
	}{
		{"gitlab", "gitlab-06f00d", 63, ""},
		{"someverylongresourcename", "someveryl-06f00d", 16, ""},
		{"gitlab", "", 10, "maxNameLength (10) must be greater than 16"},
	} {
		result, err := model.DisambiguateResourceName(coretesting.ModelTag.Id(), t.name, t.maxLength)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(t.err))
		} else {
			c.Check(err, tc.ErrorIsNil)
			c.Check(result, tc.Equals, t.result)
		}
	}
}

func (*NamingSuite) TestDisambiguateNameWithSuffixLength(c *tc.C) {
	for _, t := range []struct {
		name         string
		result       string
		maxLength    uint
		suffixLength uint
		err          string
	}{
		{"gitlab", "gitlab-d06f00d", 63, 7, ""},
		{"someverylongresourcename", "someverylongresourcenam-80004b1d0d06f00d", 40, 16, ""},
		{"gitlab", "", 18, 20, "suffixLength (20) must be between 6 and 13"},
		{"gitlab", "", 18, 4, "suffixLength (4) must be between 6 and 13"},
	} {
		result, err := model.DisambiguateResourceNameWithSuffixLength(coretesting.ModelTag.Id(), t.name, t.maxLength, t.suffixLength)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, regexp.QuoteMeta(t.err))
		} else {
			c.Check(err, tc.ErrorIsNil)
			c.Check(result, tc.Equals, t.result)
		}
	}
}
