// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/internal/testing"
)

type ScopedContextSuite struct {
	testing.BaseSuite
}

func TestScopedContextSuite(t *stdtesting.T) {
	tc.Run(t, &ScopedContextSuite{})
}

// TestScopedContextDetachedFromCatacomb asserts that the loop's scoped context
// is not cancelled when the uniter's catacomb dies. This is what allows an
// in-flight hook (for example, a storage-detaching hook during unit removal)
// to complete and write its state instead of failing with context canceled.
func (s *ScopedContextSuite) TestScopedContextDetachedFromCatacomb(c *tc.C) {
	var u Uniter
	err := catacomb.Invoke(catacomb.Plan{
		Name: "uniter",
		Site: &u.catacomb,
		Work: func() error {
			<-u.catacomb.Dying()
			return u.catacomb.ErrDying()
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx, cancel := u.scopedContext()
	defer cancel()

	u.catacomb.Kill(nil)
	select {
	case <-u.catacomb.Dead():
	case <-c.Context().Done():
		c.Fatal("catacomb did not die")
	}

	select {
	case <-ctx.Done():
		c.Fatal("scoped context should not be cancelled by catacomb death")
	default:
	}

	cancel()
	select {
	case <-ctx.Done():
	default:
		c.Fatal("scoped context should be cancelled by its cancel func")
	}
}
