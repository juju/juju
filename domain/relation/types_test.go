// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestValidate(c *gc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

func (s *typesSuite) TestValidateFailEndpointsOne(c *gc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, gc.NotNil)
}

func (s *typesSuite) TestValidateFailEndpointsMoreThanTwo(c *gc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide", "dead:beef"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, gc.NotNil)
}
