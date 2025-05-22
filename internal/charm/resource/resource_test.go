// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package resource_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/resource"
)

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

func TestResourceSuite(t *stdtesting.T) {
	tc.Run(t, &ResourceSuite{})
}

type ResourceSuite struct{}

func (s *ResourceSuite) TestValidateFull(c *tc.C) {
	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        1,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateZeroValue(c *tc.C) {
	var res resource.Resource
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *ResourceSuite) TestValidateBadMetadata(c *tc.C) {
	var meta resource.Meta
	c.Assert(meta.Validate(), tc.NotNil)

	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta:        meta,
		Origin:      resource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `bad metadata: .*`)
}

func (s *ResourceSuite) TestValidateBadOrigin(c *tc.C) {
	var origin resource.Origin
	c.Assert(origin.Validate(), tc.NotNil)
	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      origin,
		Revision:    1,
		Fingerprint: fp,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `bad origin: .*`)
}

func (s *ResourceSuite) TestValidateUploadNegativeRevision(c *tc.C) {
	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginUpload,
		Revision:    -1,
		Fingerprint: fp,
		Size:        10,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateStoreNegativeRevisionNoFile(c *tc.C) {
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:   resource.OriginStore,
		Revision: -1,
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateBadRevision(c *tc.C) {
	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginStore,
		Revision:    -1,
		Fingerprint: fp,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `bad revision: must be non-negative, got -1`)
}

func (s *ResourceSuite) TestValidateZeroValueFingerprint(c *tc.C) {
	var fp resource.Fingerprint
	c.Assert(fp.Validate(), tc.NotNil)

	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateMissingFingerprint(c *tc.C) {
	var fp resource.Fingerprint
	c.Assert(fp.Validate(), tc.NotNil)

	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        10,
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `bad file info: missing fingerprint`)
}

func (s *ResourceSuite) TestValidateDockerType(c *tc.C) {
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeContainerImage,
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:   resource.OriginStore,
		Revision: 1,
	}
	err := res.Validate()

	c.Check(err, tc.ErrorIsNil)
}

func (s *ResourceSuite) TestValidateBadSize(c *tc.C) {
	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	res := resource.Resource{
		Meta: resource.Meta{
			Name:        "my-resource",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		Origin:      resource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        -1,
	}
	err = res.Validate()

	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, `bad file info: negative size`)
}
