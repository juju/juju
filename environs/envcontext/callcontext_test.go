// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcontext

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
)

type CallContextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CallContextSuite{})

func (s *CallContextSuite) TestWithoutValidation(c *gc.C) {
	ctx := WithoutCredentialInvalidator(context.Background())

	err := ctx.InvalidateCredential("bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CallContextSuite) TestWithValidation(c *gc.C) {
	called := ""
	ctx := WithCredentialInvalidator(context.Background(), func(_ context.Context, reason string) error {
		called = reason
		return nil
	})

	err := ctx.InvalidateCredential("bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
}

func (s *CallContextSuite) TestNewCredentialInvalidator(c *gc.C) {
	keyGetter := func() (credential.Key, error) {
		return credential.Key{Name: "foo"}, nil
	}
	called := ""
	invalidate := func(ctx context.Context, key credential.Key, reason string) error {
		c.Assert(key, jc.DeepEquals, credential.Key{Name: "foo"})
		called = reason
		return nil
	}
	legacyCalled := ""
	legacyInvalidate := func(reason string) error {
		legacyCalled = reason
		return nil
	}
	invalidator := NewCredentialInvalidator(keyGetter, invalidate, legacyInvalidate)
	err := invalidator.InvalidateModelCredential(context.Background(), "bad")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, "bad")
	c.Assert(legacyCalled, gc.Equals, "bad")
}
