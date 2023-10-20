// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcontext

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/credential"
)

type CallContextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CallContextSuite{})

func (s *CallContextSuite) TestWithoutValidation(c *gc.C) {
	stdCtx := context.Background()
	ctx := WithoutCredentialInvalidator(stdCtx)

	invalidate := CredentialInvalidatorFromContext(ctx)
	err := invalidate(context.Background(), "bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CallContextSuite) TestWithValidation(c *gc.C) {
	stdCtx := context.Background()
	called := ""
	ctx := WithCredentialInvalidator(stdCtx, func(_ context.Context, reason string) error {
		called = reason
		return nil
	})

	invalidate := CredentialInvalidatorFromContext(ctx)
	err := invalidate(context.Background(), "bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
}

func (s *CallContextSuite) TestNewCredentialInvalidator(c *gc.C) {
	idGetter := func() (credential.ID, error) {
		return credential.ID{Name: "foo"}, nil
	}
	called := ""
	invalidate := func(ctx context.Context, id credential.ID, reason string) error {
		c.Assert(id, jc.DeepEquals, credential.ID{Name: "foo"})
		called = reason
		return nil
	}
	legacyCalled := ""
	legacyInvalidate := func(reason string) error {
		legacyCalled = reason
		return nil
	}
	invalidator := NewCredentialInvalidator(idGetter, invalidate, legacyInvalidate)
	err := invalidator.InvalidateModelCredential(context.Background(), "bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
	c.Assert(legacyCalled, gc.Equals, "bad")
}
