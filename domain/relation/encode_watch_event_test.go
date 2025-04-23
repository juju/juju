// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
)

type encodeWatchEventSuite struct {
}

var _ = gc.Suite(&encodeWatchEventSuite{})

func (s *encodeWatchEventSuite) TestEncodeApplicationUUID(c *gc.C) {
	// Arrange
	input := "app-uuid"
	expected := string(ApplicationUUID) + separator + input

	// Act
	result := EncodeApplicationUUID(coreapplication.ID(input))

	// Assert
	c.Assert(result, gc.Equals, expected)
}

func (s *encodeWatchEventSuite) TestEncodeUnitUUID(c *gc.C) {
	// Arrange
	input := "unit-uuid"
	expected := string(UnitUUID) + separator + input

	// Act
	result := EncodeUnitUUID(coreunit.UUID(input))

	// Assert
	c.Assert(result, gc.Equals, expected)
}

func (s *encodeWatchEventSuite) TestDecodeUnitUUID(c *gc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := string(UnitUUID) + separator + uuid

	// Act
	kind, value, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(kind, gc.Equals, UnitUUID)
	c.Assert(value, gc.Equals, uuid)
}

func (s *encodeWatchEventSuite) TestDecodeApplicationUUID(c *gc.C) {
	// Arrange
	uuid := "app-uuid"
	encoded := string(ApplicationUUID) + separator + uuid

	// Act
	kind, value, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(kind, gc.Equals, ApplicationUUID)
	c.Assert(value, gc.Equals, uuid)
}

func (s *encodeWatchEventSuite) TestDecodeErrorWrongKind(c *gc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := "Wrong" + separator + uuid

	// Act
	_, _, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, gc.ErrorMatches, "invalid event with uuid:.*")
}

func (s *encodeWatchEventSuite) TestDecodeErrorWrongFormat(c *gc.C) {
	// Arrange
	uuid := "unit-uuid"
	encoded := string(ApplicationUUID) + "#:broken sep:#" + uuid

	// Act
	_, _, err := DecodeWatchRelationUnitChangeUUID(encoded)

	// Assert
	c.Assert(err, gc.ErrorMatches, "invalid event with uuid:.*")
}
