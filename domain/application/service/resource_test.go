// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	coreerrors "github.com/juju/juju/core/errors"
	resourcestesting "github.com/juju/juju/core/resource/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type resourceServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&resourceServiceSuite{})

func (s *resourceServiceSuite) TestGetApplicationResourceID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcestesting.GenResourceUUID(c)
	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		Name:          "test-resource",
	}
	s.state.EXPECT().GetApplicationResourceID(gomock.Any(), args).Return(retID, nil)

	ret, err := s.service.GetApplicationResourceID(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ret, gc.Equals, retID)
}

func (s *resourceServiceSuite) TestGetApplicationResourceIDBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: "",
		Name:          "test-resource",
	}
	_, err := s.service.GetApplicationResourceID(context.Background(), args)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestGetApplicationResourceIDBadName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		Name:          "",
	}
	_, err := s.service.GetApplicationResourceID(context.Background(), args)
	c.Assert(err, jc.ErrorIs, applicationerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestListResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	expectedList := resource.ApplicationResources{
		Resources: []resource.Resource{{
			RetrievedBy:     "admin",
			RetrievedByType: resource.Application,
		}},
	}
	s.state.EXPECT().ListResources(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.ListResources(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedList, gc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestListResourcesBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.ListResources(context.Background(), "")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestGetResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := resourcestesting.GenResourceUUID(c)
	expectedRes := resource.Resource{
		RetrievedBy:     "admin",
		RetrievedByType: resource.Application,
	}
	s.state.EXPECT().GetResource(gomock.Any(), id).Return(expectedRes, nil)

	obtainedRes, err := s.service.GetResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, expectedRes)
}

func (s *resourceServiceSuite) TestGetResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResource(context.Background(), "")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

func (s *resourceServiceSuite) TestSetResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	args := resource.SetResourceArgs{
		ApplicationID:  applicationtesting.GenApplicationUUID(c),
		SuppliedBy:     "admin",
		SuppliedByType: resource.User,
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "my-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Revision:    1,
			Fingerprint: fp,
			Size:        1,
		},
		Reader:    nil,
		Increment: false,
	}
	expectedRes := resource.Resource{
		RetrievedBy:     "admin",
		RetrievedByType: resource.User,
	}
	s.state.EXPECT().SetResource(gomock.Any(), args).Return(expectedRes, nil)

	obtainedRes, err := s.service.SetResource(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, expectedRes)
}

func (s *resourceServiceSuite) TestSetResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.SetResourceArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
	}
	_, err := s.service.SetResource(context.Background(), args)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetResourceBadSuppliedBy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.SetResourceArgs{
		ApplicationID:  applicationtesting.GenApplicationUUID(c),
		SuppliedByType: resource.Application,
	}
	_, err := s.service.SetResource(context.Background(), args)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetResourceBadResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.SetResourceArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		Resource: charmresource.Resource{
			Meta:   charmresource.Meta{},
			Origin: charmresource.OriginStore,
		},
		Reader:    nil,
		Increment: false,
	}

	_, err := s.service.SetResource(context.Background(), args)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resID := resourcestesting.GenResourceUUID(c)
	args := resource.SetUnitResourceArgs{
		ResourceUUID:    resID,
		RetrievedBy:     "admin",
		RetrievedByType: resource.User,
		UnitUUID:        unittesting.GenUnitUUID(c),
	}
	expectedRet := resource.SetUnitResourceResult{
		UUID: resourcestesting.GenResourceUUID(c),
	}
	s.state.EXPECT().SetUnitResource(gomock.Any(), args).Return(expectedRet, nil)

	obtainedRet, err := s.service.SetUnitResource(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRet, gc.DeepEquals, expectedRet)
}

func (s *resourceServiceSuite) TestOpenApplicationResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcestesting.GenResourceUUID(c)
	expectedRes := resource.Resource{
		UUID:          id,
		ApplicationID: applicationtesting.GenApplicationUUID(c),
	}
	s.state.EXPECT().OpenApplicationResource(gomock.Any(), id).Return(expectedRes, nil)

	obtainedRes, _, err := s.service.OpenApplicationResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, obtainedRes)
}

func (s *resourceServiceSuite) TestOpenApplicationResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenApplicationResource(context.Background(), "id")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestOpenUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	resourceID := resourcestesting.GenResourceUUID(c)
	unitID := unittesting.GenUnitUUID(c)
	expectedRes := resource.Resource{
		UUID:          resourceID,
		ApplicationID: applicationtesting.GenApplicationUUID(c),
	}
	s.state.EXPECT().OpenUnitResource(gomock.Any(), resourceID, unitID).Return(expectedRes, nil)

	obtainedRes, _, err := s.service.OpenUnitResource(context.Background(), resourceID, unitID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, obtainedRes)

}

func (s *resourceServiceSuite) TestOpenUnitResourceBadUnitID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	resourceID := resourcestesting.GenResourceUUID(c)

	_, _, err := s.service.OpenUnitResource(context.Background(), resourceID, "")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestOpenUnitResourceBadResourceID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenUnitResource(context.Background(), "", "")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	args := resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		Info: []charmresource.Resource{{

			Meta: charmresource.Meta{
				Name:        "my-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginStore,
			Revision:    1,
			Fingerprint: fp,
			Size:        1,
		}},
		LastPolled: time.Now(),
	}
	s.state.EXPECT().SetRepositoryResources(gomock.Any(), args).Return(nil)

	err = s.service.SetRepositoryResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}
