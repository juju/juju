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

var _ = gc.Suite(&ResourceSuite{})

type ResourceSuite struct {
	testing.IsolationSuite
}

func (ResourceSuite) TestValidateUploadOkay(c *gc.C) {
	res := resource.Resource{
		Spec: resource.Spec{
			Definition: charmresource.Info{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need it",
			},
			Origin:   resource.OriginKindUpload,
			Revision: resource.NoRevision,
		},
		Origin: resource.Origin{
			Kind:  resource.OriginKindUpload,
			Value: "a-user",
		},
		Revision: resource.Revision{
			Type:  resource.RevisionTypeDate,
			Value: "2015-02-12",
		},
		Fingerprint: "deadbeef",
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (ResourceSuite) TestValidateBadSpec(c *gc.C) {
	res := resource.Resource{
		Origin: resource.Origin{
			Kind:  resource.OriginKindUpload,
			Value: "a-user",
		},
		Revision: resource.Revision{
			Type:  resource.RevisionTypeDate,
			Value: "2015-02-12",
		},
		Fingerprint: "deadbeef",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad spec.*`)
}

func (ResourceSuite) TestValidateBadOrigin(c *gc.C) {
	c.Assert(resource.Origin{}.Validate(), gc.NotNil)

	res := resource.Resource{
		Spec: resource.Spec{
			Definition: charmresource.Info{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:   resource.OriginKindUpload,
			Revision: resource.NoRevision,
		},
		Origin: resource.Origin{},
		Revision: resource.Revision{
			Type:  resource.RevisionTypeDate,
			Value: "2015-02-12",
		},
		Fingerprint: "deadbeef",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad origin.*`)
}

func (ResourceSuite) TestValidateBadRevision(c *gc.C) {
	c.Assert(resource.Revision{}.Validate(), gc.NotNil)

	res := resource.Resource{
		Spec: resource.Spec{
			Definition: charmresource.Info{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:   resource.OriginKindUpload,
			Revision: resource.NoRevision,
		},
		Origin: resource.Origin{
			Kind:  resource.OriginKindUpload,
			Value: "a-user",
		},
		Revision:    resource.Revision{},
		Fingerprint: "deadbeef",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad revision.*`)
}

func (ResourceSuite) TestValidateUploadNoRevision(c *gc.C) {
	res := resource.Resource{
		Spec: resource.Spec{
			Definition: charmresource.Info{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:   resource.OriginKindUpload,
			Revision: resource.NoRevision,
		},
		Origin: resource.Origin{
			Kind:  resource.OriginKindUpload,
			Value: "a-user",
		},
		Revision:    resource.NoRevision,
		Fingerprint: "deadbeef",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*origin.*does not support revision type.*`)
}

func (ResourceSuite) TestValidateBadFingerprint(c *gc.C) {
	c.Assert(resource.Revision{}.Validate(), gc.NotNil)

	res := resource.Resource{
		Spec: resource.Spec{
			Definition: charmresource.Info{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:   resource.OriginKindUpload,
			Revision: resource.NoRevision,
		},
		Origin: resource.Origin{
			Kind:  resource.OriginKindUpload,
			Value: "a-user",
		},
		Revision: resource.Revision{
			Type:  resource.RevisionTypeDate,
			Value: "2015-02-12",
		},
		Fingerprint: "",
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing fingerprint.*`)
}
