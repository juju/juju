// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	resourcestesting "github.com/juju/juju/core/resources/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type resourceServiceSuite struct {
	testing.IsolationSuite

	state   *MockResourceState
	service *ResourceService
}

var _ = gc.Suite(&resourceServiceSuite{})

func (s *resourceServiceSuite) TestGetApplicationResourceID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcestesting.GenResourceID(c)
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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
			SuppliedBy:     "admin",
			SuppliedByType: resource.Application,
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestGetResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := resourcestesting.GenResourceID(c)
	expectedRes := resource.Resource{
		SuppliedBy:     "admin",
		SuppliedByType: resource.Application,
	}
	s.state.EXPECT().GetResource(gomock.Any(), id).Return(expectedRes, nil)

	obtainedRes, err := s.service.GetResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, expectedRes)
}

func (s *resourceServiceSuite) TestGetResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResource(context.Background(), "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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
		SuppliedBy:     "admin",
		SuppliedByType: resource.User,
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestSetResourceBadSuppliedBy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.SetResourceArgs{
		ApplicationID:  applicationtesting.GenApplicationUUID(c),
		SuppliedByType: resource.Application,
	}
	_, err := s.service.SetResource(context.Background(), args)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestSetUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	args := resource.SetUnitResourceArgs{
		UnitID:         unittesting.GenUnitUUID(c),
		SuppliedBy:     "admin",
		SuppliedByType: resource.User,
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "my-resource",
				Type:        charmresource.TypeFile,
				Path:        "filename.tgz",
				Description: "One line that is useful when operators need to push it.",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
			Size:        1,
		},
	}
	expectedRet := resource.SetUnitResourceResult{
		ID: resourcestesting.GenResourceID(c),
	}
	s.state.EXPECT().SetUnitResource(gomock.Any(), args).Return(expectedRet, nil)

	obtainedRet, err := s.service.SetUnitResource(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRet, gc.DeepEquals, expectedRet)
}

func (s *resourceServiceSuite) TestOpenApplicationResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcestesting.GenResourceID(c)
	expectedRes := resource.Resource{
		ID:            id,
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
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestOpenUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	resourceID := resourcestesting.GenResourceID(c)
	unitID := unittesting.GenUnitUUID(c)
	expectedRes := resource.Resource{
		ID:            resourceID,
		ApplicationID: applicationtesting.GenApplicationUUID(c),
	}
	s.state.EXPECT().OpenUnitResource(gomock.Any(), resourceID, unitID).Return(expectedRes, nil)

	obtainedRes, _, err := s.service.OpenUnitResource(context.Background(), resourceID, unitID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, obtainedRes)

}

func (s *resourceServiceSuite) TestOpenUnitResourceBadUnitID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	resourceID := resourcestesting.GenResourceID(c)

	_, _, err := s.service.OpenUnitResource(context.Background(), resourceID, "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *resourceServiceSuite) TestOpenUnitResourceBadResourceID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenUnitResource(context.Background(), "", "")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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

func (s *resourceServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockResourceState(ctrl)
	s.service = NewResourceService(s.state, nil, loggertesting.WrapCheckLog(c))
	return ctrl
}
