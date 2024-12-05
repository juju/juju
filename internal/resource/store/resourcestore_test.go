// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resource/store"
	charmresource "github.com/juju/juju/internal/charm/resource"
)

type resourceStoreSuite struct {
	objectStore            *MockObjectStore
	modelObjectStoreGetter *MockModelObjectStoreGetter
	resourceStore          *MockResourceStore
}

var _ = gc.Suite(&resourceStoreSuite{})

func (s *resourceStoreSuite) TestGetResourceStoreTypeFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil)

	store, err := s.factory().GetResourceStore(context.Background(), charmresource.TypeFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store, gc.Equals, fileResourceStore{s.objectStore})
}

func (s *resourceStoreSuite) TestGetResourceStoreTypeFileError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	kaboom := errors.Errorf("kaboom")
	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, kaboom)

	_, err := s.factory().GetResourceStore(context.Background(), charmresource.TypeFile)
	c.Assert(err, jc.ErrorIs, kaboom)
}

func (s *resourceStoreSuite) TestGetResourceStoreNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.factory().GetResourceStore(context.Background(), charmresource.Type(0))
	c.Assert(err, jc.ErrorIs, UnknownResourceType)
}

func (s *resourceStoreSuite) TestGetResourceStoreTypeContainerImage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.resourceStore.EXPECT().Remove(context.Background(), gomock.Any()).Return(nil)

	store, err := s.factory().GetResourceStore(context.Background(), charmresource.TypeContainerImage)
	c.Assert(err, jc.ErrorIsNil)
	err = store.Remove(context.Background(), "string")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
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
