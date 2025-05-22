// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
)

type encodeWatchEventSuite struct {
}

func TestEncodeWatchEventSuite(t *stdtesting.T) {
	tc.Run(t, &encodeWatchEventSuite{})
}

func (s *encodeWatchEventSuite) TestEncodeApplicationUUID(c *tc.C) {
	// Arrange
	input := "app-uuid"
	expected := string(ApplicationUUID) + separator + input

	// Act
	result := EncodeApplicationUUID(coreapplication.ID(input))

	// Assert
	c.Assert(result, tc.Equals, expected)
}

func (s *encodeWatchEventSuite) TestEncodeUnitUUID(c *tc.C) {
	// Arrange
	input := "unit-uuid"
	expected := string(UnitUUID) + separator + input

	// Act
	result := EncodeUnitUUID(coreunit.UUID(input))

	// Assert
	c.Assert(result, tc.Equals, expected)
}

func (s *encodeWatchEventSuite) TestDecodeUnitUUID(c *tc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := string(UnitUUID) + separator + uuid

	// Act
	kind, value, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(kind, tc.Equals, UnitUUID)
	c.Assert(value, tc.Equals, uuid)
}

func (s *encodeWatchEventSuite) TestDecodeApplicationUUID(c *tc.C) {
	// Arrange
	uuid := "app-uuid"
	encoded := string(ApplicationUUID) + separator + uuid

	// Act
	kind, value, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(kind, tc.Equals, ApplicationUUID)
	c.Assert(value, tc.Equals, uuid)
}

func (s *encodeWatchEventSuite) TestDecodeErrorWrongKind(c *tc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := "Wrong" + separator + uuid

	// Act
	_, _, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, tc.ErrorMatches, "invalid event with uuid:.*")
}

func (s *encodeWatchEventSuite) TestDecodeErrorWrongFormat(c *tc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := string(ApplicationUUID) + "#:broken sep:#" + uuid

	// Act
	_, _, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, tc.ErrorMatches, "invalid event with uuid:.*")
}
