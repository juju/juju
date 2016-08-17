// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/common"
)

var _ = gc.Suite(&cloudCredentialSuite{})

type cloudCredentialSuite struct {
	testing.IsolationSuite
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTag(c *gc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"foo",
		"aws/admin@local/foo",
	)
}

func (*cloudCredentialSuite) TestResolveCloudCredentialTagOtherUser(c *gc.C) {
	testResolveCloudCredentialTag(c,
		names.NewUserTag("admin@local"),
		names.NewCloudTag("aws"),
		"brenda@local/foo",
		"aws/brenda@local/foo",
	)
}

func testResolveCloudCredentialTag(
	c *gc.C,
	user names.UserTag,
	cloud names.CloudTag,
	credentialName string,
	expect string,
) {
	tag, err := common.ResolveCloudCredentialTag(user, cloud, credentialName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag.Id(), gc.Equals, expect)
}
