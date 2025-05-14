// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/resource/store"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type resourceStoreSuite struct {
	objectStore            *MockObjectStore
	modelObjectStoreGetter *MockModelObjectStoreGetter
	resourceStore          *MockResourceStore
}

var _ = tc.Suite(&resourceStoreSuite{})

func (s *resourceStoreSuite) TestGetResourceStoreTypeFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil)

	store, err := s.factory().GetResourceStore(c.Context(), charmresource.TypeFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(store, tc.Equals, fileResourceStore{s.objectStore})
}

func (s *resourceStoreSuite) TestGetResourceStoreTypeFileError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	kaboom := errors.Errorf("kaboom")
	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, kaboom)

	_, err := s.factory().GetResourceStore(c.Context(), charmresource.TypeFile)
	c.Assert(err, tc.ErrorIs, kaboom)
}

func (s *resourceStoreSuite) TestGetResourceStoreNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.factory().GetResourceStore(c.Context(), charmresource.Type(0))
	c.Assert(err, tc.ErrorIs, UnknownResourceType)
}

func (s *resourceStoreSuite) TestGetResourceStoreTypeContainerImage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.resourceStore.EXPECT().Remove(c.Context(), gomock.Any()).Return(nil)

	store, err := s.factory().GetResourceStore(c.Context(), charmresource.TypeContainerImage)
	c.Assert(err, tc.ErrorIsNil)
	err = store.Remove(c.Context(), "string")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.resourceStore = NewMockResourceStore(ctrl)

	return ctrl
}

func (s *resourceStoreSuite) factory() *ResourceStoreFactory {
	getter := func() store.ResourceStore {
		return s.resourceStore
	}
	return NewResourceStoreFactory(s.modelObjectStoreGetter, getter)
}
