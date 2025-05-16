// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type typesSuite struct{}

func TestTypesSuite(t *stdtesting.T) { tc.Run(t, &typesSuite{}) }
func (s *typesSuite) TestValidate(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *typesSuite) TestValidateFailEndpointsOne(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, tc.NotNil)
}

func (s *typesSuite) TestValidateFailEndpointsMoreThanTwo(c *tc.C) {
	// Arrange
	args := GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide", "dead:beef"},
	}

	// Act
	err := args.Validate()

	// Assert
	c.Assert(err, tc.NotNil)
}
