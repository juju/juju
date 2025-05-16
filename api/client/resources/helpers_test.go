// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

const fingerprint = "123456789012345678901234567890123456789012345678"

type HelpersSuite struct {
	testhelpers.IsolationSuite
}

func TestHelpersSuite(t *stdtesting.T) { tc.Run(t, &HelpersSuite{}) }
func (s *HelpersSuite) TestResource2API(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	apiRes := Resource2API(res)

	c.Check(apiRes, tc.DeepEquals, params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	})
}

func (s *HelpersSuite) TestAPIResult2ApplicationResourcesOkay(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	resUUID, err := resource.NewUUID()
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) cannot create resource UUID"))

	now := time.Now()
	expected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            resUUID,
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	unitExpected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "unitspam",
				Type:        charmresource.TypeFile,
				Path:        "unitspam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            resUUID,
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = unitExpected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            resUUID.String(),
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	}

	unitRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            resUUID.String(),
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	}

	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("boo!"))
	c.Assert(err, tc.ErrorIsNil)

	chRes := params.CharmResource{
		Name:        "unitspam2",
		Type:        "file",
		Path:        "unitspam.tgz2",
		Description: "you need it2",
		Origin:      "upload",
		Revision:    2,
		Fingerprint: fp2.Bytes(),
		Size:        11,
	}

	chExpected := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "unitspam2",
			Type:        charmresource.TypeFile,
			Path:        "unitspam.tgz2",
			Description: "you need it2",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    2,
		Fingerprint: fp2,
		Size:        11,
	}

	res, err := apiResult2ApplicationResources(params.ResourcesResult{
		Resources: []params.Resource{
			apiRes,
		},
		CharmStoreResources: []params.CharmResource{
			chRes,
		},
		UnitResources: []params.UnitResources{
			{
				Entity: params.Entity{
					Tag: "unit-foo-0",
				},
				Resources: []params.Resource{
					unitRes,
				},
				DownloadProgress: map[string]int64{
					unitRes.Name: 8,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	serviceResource := resource.ApplicationResources{
		Resources: []resource.Resource{
			expected,
		},
		RepositoryResources: []charmresource.Resource{
			chExpected,
		},
		UnitResources: []resource.UnitResources{
			{
				Name: "foo/0",
				Resources: []resource.Resource{
					unitExpected,
				},
			},
		},
	}

	c.Check(res, tc.DeepEquals, serviceResource)
}

func (s *HelpersSuite) TestAPIResult2ApplicationResourcesBadUnitTag(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	now := time.Now()
	expected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	unitExpected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "unitspam",
				Type:        charmresource.TypeFile,
				Path:        "unitspam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = unitExpected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	}

	unitRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	}

	_, err = apiResult2ApplicationResources(params.ResourcesResult{
		Resources: []params.Resource{
			apiRes,
		},
		UnitResources: []params.UnitResources{
			{
				Entity: params.Entity{
					Tag: "THIS IS NOT A GOOD UNIT TAG",
				},
				Resources: []params.Resource{
					unitRes,
				},
			},
		},
	})
	c.Assert(err, tc.ErrorMatches, ".*got bad data from server.*")
}

func (s *HelpersSuite) TestAPIResult2ApplicationResourcesFailure(c *tc.C) {
	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
	}
	failure := errors.New("<failure>")

	_, err := apiResult2ApplicationResources(params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: failure.Error(),
			},
		},
		Resources: []params.Resource{
			apiRes,
		},
	})

	c.Check(err, tc.ErrorMatches, "<failure>")
	c.Check(errors.Cause(err), tc.Not(tc.Equals), failure)
}

func (s *HelpersSuite) TestAPIResult2ApplicationResourcesNotFound(c *tc.C) {
	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            "a-application/spam",
		ApplicationName: "a-application",
	}

	_, err := apiResult2ApplicationResources(params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: `application "a-application" not found`,
				Code:    params.CodeNotFound,
			},
		},
		Resources: []params.Resource{
			apiRes,
		},
	})

	c.Check(err, tc.ErrorIs, errors.NotFound)
}

func (s *HelpersSuite) TestAPI2Resource(c *tc.C) {
	now := time.Now()
	resUUID, err := resource.NewUUID()
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) cannot create resource UUID"))

	res, err := API2Resource(params.Resource{
		CharmResource: params.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		UUID:            resUUID.String(),
		ApplicationName: "a-application",
		Username:        "a-user",
		Timestamp:       now,
	})
	c.Assert(err, tc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	expected := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        10,
		},
		UUID:            resUUID,
		ApplicationName: "a-application",
		RetrievedBy:     "a-user",
		Timestamp:       now,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, expected)
}

func (s *HelpersSuite) TestCharmResource2API(c *tc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "spam",
			Type:        charmresource.TypeFile,
			Path:        "spam.tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        10,
	}
	err = res.Validate()
	c.Assert(err, tc.ErrorIsNil)
	apiInfo := CharmResource2API(res)

	c.Check(apiInfo, tc.DeepEquals, params.CharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need it",
		Origin:      "upload",
		Revision:    1,
		Fingerprint: []byte(fingerprint),
		Size:        10,
	})
}

func (s *HelpersSuite) TestAPI2CharmResource(c *tc.C) {
	res, err := API2CharmResource(params.CharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need it",
		Origin:      "upload",
		Revision:    1,
		Fingerprint: []byte(fingerprint),
		Size:        10,
	})
	c.Assert(err, tc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, tc.ErrorIsNil)
	expected := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "spam",
			Type:        charmresource.TypeFile,
			Path:        "spam.tgz",
			Description: "you need it",
		},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        10,
	}
	err = expected.Validate()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, expected)
}
