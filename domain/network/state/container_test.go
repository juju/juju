package state

import (
	"testing"

	"github.com/juju/tc"
)

type containerSuite struct {
	linkLayerBaseSuite
}

func TestContainerSuite(t *testing.T) {
	tc.Run(t, &containerSuite{})
}

func (s *containerSuite) TestGetMachineSpaceConstraints(c *tc.C) {
	db := s.DB()

	// Arrange. Set up two spaces a machine with those as
	// positive and negative constraints respectively.
	nUUID := s.addNetNode(c)
	mUUID := s.addMachine(c, "0", nUUID)
	posSpace := s.addSpace(c)
	negSpace := s.addSpace(c)
	conUUID := "constraint-uuid"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, `INSERT INTO "constraint" (uuid) VALUES (?)`, conUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(ctx, "INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)",
		mUUID, conUUID)
	c.Assert(err, tc.ErrorIsNil)

	for i, s := range []string{posSpace, negSpace} {
		exclude := i != 0

		_, err := db.ExecContext(ctx, "INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)",
			conUUID, s, exclude)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Act
	pos, neg, err := s.state.GetMachineSpaceConstraints(ctx, mUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Assert(pos, tc.HasLen, 1)
	c.Assert(neg, tc.HasLen, 1)
	c.Check(pos[0].UUID, tc.Equals, posSpace)
	c.Check(neg[0].UUID, tc.Equals, negSpace)
}
