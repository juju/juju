// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/resourcetesting"
)

const fingerprint = "123456789012345678901234567890123456789012345678"

func newFingerprint(c *gc.C, data string) charmresource.Fingerprint {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	return fp
}

type HelpersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HelpersSuite{})

func (HelpersSuite) TestResource2API(c *gc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)
	apiRes := api.Resource2API(res)

	c.Check(apiRes, jc.DeepEquals, api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	})
}

func (HelpersSuite) TestAPIResult2ServiceResourcesOkay(c *gc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = unitExpected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}

	unitRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}

	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("boo!"))
	c.Assert(err, jc.ErrorIsNil)

	chRes := api.CharmResource{
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

	resources, err := api.APIResult2ServiceResources(api.ResourcesResult{
		Resources: []api.Resource{
			apiRes,
		},
		CharmStoreResources: []api.CharmResource{
			chRes,
		},
		UnitResources: []api.UnitResources{
			{
				Entity: params.Entity{
					Tag: "unit-foo-0",
				},
				Resources: []api.Resource{
					unitRes,
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	serviceResource := resource.ServiceResources{
		Resources: []resource.Resource{
			expected,
		},
		CharmStoreResources: []charmresource.Resource{
			chExpected,
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: names.NewUnitTag("foo/0"),
				Resources: []resource.Resource{
					unitExpected,
				},
			},
		},
	}

	c.Check(resources, jc.DeepEquals, serviceResource)
}

func (HelpersSuite) TestAPIResult2ServiceResourcesBadUnitTag(c *gc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = unitExpected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}

	unitRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "unitspam",
			Type:        "file",
			Path:        "unitspam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}

	_, err = api.APIResult2ServiceResources(api.ResourcesResult{
		Resources: []api.Resource{
			apiRes,
		},
		UnitResources: []api.UnitResources{
			{
				Entity: params.Entity{
					Tag: "THIS IS NOT A GOOD UNIT TAG",
				},
				Resources: []api.Resource{
					unitRes,
				},
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, ".*got bad data from server.*")
}

func (HelpersSuite) TestAPIResult2ServiceResourcesFailure(c *gc.C) {
	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		ServiceID: "a-service",
	}
	failure := errors.New("<failure>")

	_, err := api.APIResult2ServiceResources(api.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: failure.Error(),
			},
		},
		Resources: []api.Resource{
			apiRes,
		},
	})

	c.Check(err, gc.ErrorMatches, "<failure>")
	c.Check(errors.Cause(err), gc.Not(gc.Equals), failure)
}

func (HelpersSuite) TestAPIResult2ServiceResourcesNotFound(c *gc.C) {
	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		ServiceID: "a-service",
	}

	_, err := api.APIResult2ServiceResources(api.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: &params.Error{
				Message: `service "a-service" not found`,
				Code:    params.CodeNotFound,
			},
		},
		Resources: []api.Resource{
			apiRes,
		},
	})

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (HelpersSuite) TestAPI2Resource(c *gc.C) {
	now := time.Now()
	res, err := api.API2Resource(api.Resource{
		CharmResource: api.CharmResource{
			Name:        "spam",
			Type:        "file",
			Path:        "spam.tgz",
			Description: "you need it",
			Origin:      "upload",
			Revision:    1,
			Fingerprint: []byte(fingerprint),
			Size:        10,
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	})
	c.Assert(err, jc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: "a-service",
		Username:  "a-user",
		Timestamp: now,
	}
	err = expected.Validate()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, expected)
}

func (HelpersSuite) TestCharmResource2API(c *gc.C) {
	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	apiInfo := api.CharmResource2API(res)

	c.Check(apiInfo, jc.DeepEquals, api.CharmResource{
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

func (HelpersSuite) TestAPI2CharmResource(c *gc.C) {
	res, err := api.API2CharmResource(api.CharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need it",
		Origin:      "upload",
		Revision:    1,
		Fingerprint: []byte(fingerprint),
		Size:        10,
	})
	c.Assert(err, jc.ErrorIsNil)

	fp, err := charmresource.NewFingerprint([]byte(fingerprint))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, expected)
}

func (HelpersSuite) TestServiceResource2API(c *gc.C) {
	res1 := resourcetesting.NewResource(c, nil, "res1", "a-service", "data").Resource
	res2 := resourcetesting.NewResource(c, nil, "res2", "a-service", "data2").Resource

	tag0 := names.NewUnitTag("a-service/0")
	tag1 := names.NewUnitTag("a-service/1")

	chres1 := res1.Resource
	chres2 := res2.Resource
	chres1.Revision++
	chres2.Revision++

	svcRes := resource.ServiceResources{
		Resources: []resource.Resource{
			res1,
			res2,
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: tag0,
				Resources: []resource.Resource{
					res1,
					res2,
				},
			},
			// note: nothing for tag1
		},
		CharmStoreResources: []charmresource.Resource{
			chres1,
			chres2,
		},
	}

	result := api.ServiceResources2APIResult(svcRes, []names.UnitTag{tag0, tag1})

	apiRes1 := api.Resource2API(res1)
	apiRes2 := api.Resource2API(res2)

	apiChRes1 := api.CharmResource2API(chres1)
	apiChRes2 := api.CharmResource2API(chres2)

	c.Check(result, jc.DeepEquals, api.ResourcesResult{
		Resources: []api.Resource{
			apiRes1,
			apiRes2,
		},
		UnitResources: []api.UnitResources{
			{
				Entity: params.Entity{
					Tag: "unit-a-service-0",
				},
				Resources: []api.Resource{
					apiRes1,
					apiRes2,
				},
			},
			{
				// we should have a listing for every unit, even if they
				// have no resources.
				Entity: params.Entity{
					Tag: "unit-a-service-1",
				},
			},
		},
		CharmStoreResources: []api.CharmResource{
			apiChRes1,
			apiChRes2,
		},
	})

}
