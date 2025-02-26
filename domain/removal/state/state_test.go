// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	gc "gopkg.in/check.v1"

	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestRelationExists(c *gc.C) {
	_, err := s.DB().Exec("INSERT INTO relation (uuid, life_id, relation_id) VALUES (?, ?, ?)",
		"some-relation-uuid", 0, "some-relation-id")
	c.Assert(err, gc.IsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RelationExists(context.Background(), "some-relation-uuid")
	c.Assert(err, gc.IsNil)
	c.Check(exists, gc.Equals, true)

	exists, err = st.RelationExists(context.Background(), "not-today-henry")
	c.Assert(err, gc.IsNil)
	c.Check(exists, gc.Equals, false)
}
