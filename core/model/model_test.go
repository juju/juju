// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type ModelSuite struct {
	testhelpers.IsolationSuite
}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &ModelSuite{})
}

func (*ModelSuite) TestValidModelTypes(c *tc.C) {
	validTypes := []ModelType{
		CAAS,
		IAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), tc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypes(c *tc.C) {
	validTypes := []string{
		"caas",
		"iaas",
	}

	for _, vt := range validTypes {
		mt, err := ParseModelType(vt)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mt.IsValid(), tc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypesInvalid(c *tc.C) {
	_, err := ParseModelType("foo")
	c.Assert(err, tc.ErrorMatches, `unknown model type "foo"`)
}

func (*ModelSuite) TestUUIDValidate(c *tc.C) {
	tests := []struct {
		uuid string
		err  error
	}{
		{
			uuid: "",
			err:  coreerrors.NotValid,
		},
		{
			uuid: "invalid",
			err:  coreerrors.NotValid,
		},
		{
			uuid: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		err := UUID(test.uuid).Validate()

		if test.err == nil {
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}

func (*ModelSuite) TestQualifierValidate(c *tc.C) {
	tests := []struct {
		qualifier string
		err       error
	}{
		{
			qualifier: "",
			err:       coreerrors.NotValid,
		},
		{
			qualifier: "-invalid",
			err:       coreerrors.NotValid,
		},
		{
			qualifier: "Invalid",
			err:       coreerrors.NotValid,
		},
		{
			qualifier: "qualifier-123",
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.qualifier)
		err := Qualifier(test.qualifier).Validate()

		if test.err == nil {
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}

func (*ModelSuite) TestUserTagFromQualifier(c *tc.C) {
	tag, err := ApproximateUserTagFromQualifier("prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag.String(), tc.Equals, "user-prod")
}

func (*ModelSuite) TestQualifierFromUserTag(c *tc.C) {
	tests := []struct {
		username  string
		qualifier string
	}{
		{
			username:  "fred",
			qualifier: "fred",
		},
		{
			username:  "Fred",
			qualifier: "fred",
		},
		{
			username:  "fred@external",
			qualifier: "fred-external",
		},
		{
			username:  "fred+mary@external",
			qualifier: "fred-mary-external",
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.username)
		q := QualifierFromUserTag(names.NewUserTag(test.username))
		c.Check(q.String(), tc.Equals, test.qualifier)
	}

}
