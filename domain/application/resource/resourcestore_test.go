// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

type resourceStoreSuite struct {
	objectStore            *MockObjectStore
	modelObjectStoreGetter *MockModelObjectStoreGetter
}

var _ = gc.Suite(&resourceStoreSuite{})

func (s *resourceStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)

	return ctrl
}

func (s *resourceStoreSuite) TestObjectStoreGetter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	factory := NewResourceStoreFactory(s.modelObjectStoreGetter)
	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil)

	store, err := factory.GetResourceStore(context.Background(), charmresource.TypeFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store, gc.Equals, fileResourceStore{s.objectStore})
}

func (s *resourceStoreSuite) TestObjectStoreGetterError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	factory := NewResourceStoreFactory(s.modelObjectStoreGetter)
	kaboom := errors.Errorf("kaboom")
	s.modelObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, kaboom)

	_, err := factory.GetResourceStore(context.Background(), charmresource.TypeFile)
	c.Assert(err, jc.ErrorIs, kaboom)
}

func (s *resourceStoreSuite) TestObjectStoreGetterAddStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	factory := NewResourceStoreFactory(s.modelObjectStoreGetter)
	newStore := fileResourceStore{objectStore: s.objectStore}
	factory.AddStore(charmresource.TypeContainerImage, newStore)

	store, err := factory.GetResourceStore(context.Background(), charmresource.TypeContainerImage)
	c.Assert(err, gc.IsNil)
	c.Assert(store, gc.Equals, newStore)
}

func (s *resourceStoreSuite) TestObjectStoreGetterNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	factory := NewResourceStoreFactory(s.modelObjectStoreGetter)

	_, err := factory.GetResourceStore(context.Background(), charmresource.Type(0))
	c.Assert(err, jc.ErrorIs, applicationerrors.UnknownResourceType)
}
