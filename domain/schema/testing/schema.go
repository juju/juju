// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/database/schema"
)

type SchemaApplier struct {
	Schema  *schema.Schema
	Verbose bool
}

func (s *SchemaApplier) Apply(c *gc.C, ctx context.Context, runner database.TxnRunner) {
	if s.Verbose {
		s.Schema.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
		})
	}

	changeSet, err := s.Schema.Ensure(ctx, runner)
	c.Assert(err, gc.IsNil)
	c.Check(changeSet.Post, gc.Equals, s.Schema.Len())
}
