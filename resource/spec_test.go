// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&specSuite{})

type specSuite struct {
	testing.IsolationSuite
}

func (specSuite) TestValidateUploadOkay(c *gc.C) {
	spec := resource.Spec{
		Definition: charmresource.Info{
			Name:    "spam",
			Type:    charmresource.TypeFile,
			Path:    "spam.tgz",
			Comment: "you need it",
		},
		Origin:   resource.OriginKindUpload,
		Revision: resource.NoRevision,
	}

	err := spec.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (specSuite) TestValidateUploadHasRevision(c *gc.C) {
	spec := resource.Spec{
		Definition: charmresource.Info{
			Name: "spam",
			Type: charmresource.TypeFile,
			Path: "spam.tgz",
		},
		Origin:   resource.OriginKindUpload,
		Revision: "???",
	}

	err := spec.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*don't have revisions.*`)
}

func (specSuite) TestValidateUnknownOrigin(c *gc.C) {
	spec := resource.Spec{
		Definition: charmresource.Info{
			Name: "spam",
			Type: charmresource.TypeFile,
			Path: "spam.tgz",
		},
		Origin:   resource.OriginKindUnknown,
		Revision: resource.NoRevision,
	}

	err := spec.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*origin.*`)
}

func (specSuite) TestValidateEmptyInfo(c *gc.C) {
	spec := resource.Spec{
		Origin:   resource.OriginKindUpload,
		Revision: resource.NoRevision,
	}

	err := spec.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}
