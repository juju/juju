// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	stdcontext "context"

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
	stdCtx := stdcontext.Background()
	ctx := WithoutCredentialInvalidator(stdCtx)

	invalidate := CredentialInvalidatorFromContext(ctx)
	err := invalidate("bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CallContextSuite) TestWithValidation(c *gc.C) {
	stdCtx := stdcontext.Background()
	called := ""
	ctx := WithCredentialInvalidator(stdCtx, func(reason string) error {
		called = reason
		return nil
	})

	invalidate := CredentialInvalidatorFromContext(ctx)
	err := invalidate("bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
}

func (s *CallContextSuite) TestNewCredentialInvalidator(c *gc.C) {
	idGetter := func() (credential.ID, error) {
		return credential.ID{Name: "foo"}, nil
	}
	called := ""
	invalidate := func(ctx stdcontext.Context, id credential.ID, reason string) error {
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
	err := invalidator.InvalidateModelCredential("bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
	c.Assert(legacyCalled, gc.Equals, "bad")
}
