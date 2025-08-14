// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	charmtesting "github.com/juju/juju/core/charm/testing"
	coreerrors "github.com/juju/juju/core/errors"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	storetesting "github.com/juju/juju/core/resource/store/testing"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type resourceServiceSuite struct {
	testhelpers.IsolationSuite

	state               *MockState
	resourceStoreGetter *MockResourceStoreGetter
	resourceStore       *MockResourceStore

	service *Service
}

func TestResourceServiceSuite(t *testing.T) {
	tc.Run(t, &resourceServiceSuite{})
}

func (s *resourceServiceSuite) TestDeleteApplicationResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := coreapplication.GenID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(nil)

	err := s.service.DeleteApplicationResources(c.Context(),
		appUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesBadArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteApplicationResources(c.Context(),
		"not an application ID")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationIDNotValid,
		tc.Commentf("Application ID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesUnexpectedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")

	appUUID := coreapplication.GenID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(stateError)

	err := s.service.DeleteApplicationResources(c.Context(),
		appUUID)
	c.Assert(err, tc.ErrorIs, stateError,
		tc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestDeleteUnitResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(nil)

	err := s.service.DeleteUnitResources(c.Context(),
		unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesBadArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteUnitResources(c.Context(),
		"not an unit UUID")
	c.Assert(err, tc.ErrorIs, resourceerrors.UnitUUIDNotValid,
		tc.Commentf("Unit UUID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesUnexpectedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")
	unitUUID := coreunit.GenUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(stateError)

	err := s.service.DeleteUnitResources(c.Context(),
		unitUUID)
	c.Assert(err, tc.ErrorIs, stateError,
		tc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestGetApplicationResourceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	retID := coreresource.GenUUID(c)
	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: coreapplication.GenID(c),
		Name:          "test-resource",
	}
	s.state.EXPECT().GetApplicationResourceID(gomock.Any(), args).Return(retID, nil)

	ret, err := s.service.GetApplicationResourceID(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ret, tc.Equals, retID)
}

func (s *resourceServiceSuite) TestGetApplicationResourceIDBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: "",
		Name:          "test-resource",
	}
	_, err := s.service.GetApplicationResourceID(c.Context(), args)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestGetApplicationResourceIDBadName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.GetApplicationResourceIDArgs{
		ApplicationID: coreapplication.GenID(c),
		Name:          "",
	}
	_, err := s.service.GetApplicationResourceID(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	retID := coreresource.GenUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, nil)

	ret, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "app-id", "res-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ret, tc.Equals, retID)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameResourceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	retID := coreresource.GenUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, resourceerrors.ResourceNotFound)

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "app-id", "res-name")
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	retID := coreresource.GenUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, resourceerrors.ApplicationNotFound)
	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "app-id", "res-name")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameEmptyAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "", "res-name")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameBadAppID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "9", "res-name")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameBadName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(c.Context(), "app-id", "")
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestListResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	expectedList := coreresource.ApplicationResources{
		Resources: []coreresource.Resource{{
			RetrievedBy: "admin",
		}},
	}
	s.state.EXPECT().ListResources(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.ListResources(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedList, tc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestGetResourcesByApplicationIDBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResourcesByApplicationID(c.Context(), "")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestGetResourcesByApplicationID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreapplication.GenID(c)
	expectedList := []coreresource.Resource{{
		RetrievedBy: "admin",
	}}
	s.state.EXPECT().GetResourcesByApplicationID(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.GetResourcesByApplicationID(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedList, tc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestListResourcesBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.ListResources(c.Context(), "")
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestGetResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreresource.GenUUID(c)
	expectedRes := coreresource.Resource{
		RetrievedBy: "admin",
	}
	s.state.EXPECT().GetResource(gomock.Any(), id).Return(expectedRes, nil)

	obtainedRes, err := s.service.GetResource(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedRes, tc.DeepEquals, expectedRes)
}

func (s *resourceServiceSuite) TestGetResourceBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResource(c.Context(), "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

func (s *resourceServiceSuite) TestStoreResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := coreresource.GenUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	storeFP := coreresourcestore.NewFingerprint(fp.Fingerprint)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := coreresource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		storeFP,
	).Return(storageID, size, storeFP, nil)
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: false,
		Size:                          size,
		SHA384:                        fp.String(),
	})

	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			Size:            size,
			Fingerprint:     fp,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceRemovedOnRecordError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := coreresource.GenUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	storeFP := coreresourcestore.NewFingerprint(fp.Fingerprint)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := coreresource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		storeFP,
	).Return(storageID, size, storeFP, nil)

	// Return an error from recording the stored resource.
	expectedErr := errors.New("recording failed with massive error")
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: false,
		Size:                          size,
		SHA384:                        fp.String(),
	}).Return(expectedErr)

	// Expect the removal of the resource.
	s.resourceStore.EXPECT().Remove(gomock.Any(), resourceUUID.String())

	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			Size:            size,
			Fingerprint:     fp,
		},
	)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *resourceServiceSuite) TestStoreResourceDoesNotStoreIdenticalBlobContainer(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: We only expect a call to GetResource, not to
	// RecordStoredResource since the blob is identical.
	resourceUUID := coreresource.GenUUID(c)

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: charmresource.TypeContainerImage,
				},
				Fingerprint: fp,
			},
		}, nil,
	)

	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeContainerImage).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		int64(0),
		coreresourcestore.NewFingerprint(fp.Fingerprint),
	).Return(coreresourcestore.ID{}, 0, coreresourcestore.Fingerprint{},
		containerimageresourcestoreerrors.ContainerImageMetadataAlreadyStored)

	// Act:
	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: resourceUUID,
			Reader:       reader,
			Fingerprint:  fp,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceServiceSuite) TestStoreResourceDoesNotStoreIdenticalBlobFile(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	resourceUUID := coreresource.GenUUID(c)

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: charmresource.TypeFile,
				},
				Fingerprint: fp,
			},
		}, nil,
	)

	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeFile).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		int64(0),
		coreresourcestore.NewFingerprint(fp.Fingerprint),
	).Return(coreresourcestore.ID{}, 0, coreresourcestore.Fingerprint{},
		objectstoreerrors.ObjectAlreadyExists)

	// Act:
	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: resourceUUID,
			Reader:       reader,
			Fingerprint:  fp,
		},
	)

	// Assert:
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceServiceSuite) TestStoreResourceBadUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceNilReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, tc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceNegativeSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Size:         -1,
		},
	)
	c.Assert(err, tc.ErrorMatches, "invalid size: -1")
}

func (s *resourceServiceSuite) TestStoreResourceZeroFingerprint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Fingerprint:  charmresource.Fingerprint{},
		},
	)
	c.Assert(err, tc.ErrorMatches, "invalid fingerprint")
}

func (s *resourceServiceSuite) TestStoreResourceBadRetrievedBy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    coreresource.GenUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.Unknown,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceRevisionNotValidOriginUpload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Skip("FIXME: this test is broken")

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    coreresource.GenUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.User,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceRevisionNotValidOriginStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Skip("FIXME: this test is broken")

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	err = s.service.StoreResource(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    coreresource.GenUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.User,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := coreresource.GenUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	storeFP := coreresourcestore.NewFingerprint(fp.Fingerprint)
	size := int64(42)

	retrievedBy := "bob"
	retrievedByType := coreresource.User

	storageID := storetesting.GenFileResourceStoreID(c, objectstoretesting.GenObjectStoreUUID(c))
	s.state.EXPECT().GetResource(gomock.Any(), resourceUUID).Return(
		coreresource.Resource{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Type: resourceType,
				},
			},
		}, nil,
	)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Put(
		gomock.Any(),
		resourceUUID.String(),
		reader,
		size,
		storeFP,
	).Return(storageID, size, storeFP, nil)
	s.state.EXPECT().RecordStoredResource(gomock.Any(), resource.RecordStoredResourceArgs{
		ResourceUUID:                  resourceUUID,
		StorageID:                     storageID,
		RetrievedBy:                   retrievedBy,
		RetrievedByType:               retrievedByType,
		ResourceType:                  resourceType,
		IncrementCharmModifiedVersion: true,
		Size:                          size,
		SHA384:                        fp.String(),
	})

	err = s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			Size:            size,
			Fingerprint:     fp,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionNilReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, tc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionNegativeSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Size:         -1,
		},
	)
	c.Assert(err, tc.ErrorMatches, "invalid size: -1")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionZeroFingerprint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID: coreresource.GenUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Fingerprint:  charmresource.Fingerprint{},
		},
	)
	c.Assert(err, tc.ErrorMatches, "invalid fingerprint")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadRetrievedBy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	err = s.service.StoreResourceAndIncrementCharmModifiedVersion(
		c.Context(),
		resource.StoreResourceArgs{
			ResourceUUID:    coreresource.GenUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.Unknown,
		},
	)
	c.Assert(err, tc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestSetUnitResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := coreresource.GenUUID(c)
	unitUUID := coreunit.GenUUID(c)

	s.state.EXPECT().SetUnitResource(gomock.Any(), resourceUUID, unitUUID).Return(nil)

	err := s.service.SetUnitResource(c.Context(), resourceUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadResourceUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)

	err := s.service.SetUnitResource(c.Context(), "bad-uuid", unitUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := coreresource.GenUUID(c)

	err := s.service.SetUnitResource(c.Context(), resourceUUID, "bad-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestOpenResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreresource.GenUUID(c)
	reader := io.NopCloser(bytes.NewBufferString("spam"))
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	size := int64(42)
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
		UUID: id,
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(reader, size, nil)

	obtainedRes, obtainedReader, err := s.service.OpenResource(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedRes, tc.DeepEquals, res)
	c.Assert(obtainedReader, tc.DeepEquals, reader)
}

func (s *resourceServiceSuite) TestOpenResourceFileNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreresource.GenUUID(c)
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	size := int64(42)
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(nil, 0, objectstoreerrors.ObjectNotFound)

	_, _, err = s.service.OpenResource(c.Context(), id)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceServiceSuite) TestOpenResourceContainerImageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreresource.GenUUID(c)
	resourceType := charmresource.TypeContainerImage
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	size := int64(42)
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(nil, 0, containerimageresourcestoreerrors.ContainerImageMetadataNotFound)

	_, _, err = s.service.OpenResource(c.Context(), id)
	c.Assert(err, tc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

// TestOpenResourceUnexpectedSize checks that an error is returned if the size
// of the resource in the object store is not what was expected.
func (s *resourceServiceSuite) TestOpenResourceUnexpectedSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := coreresource.GenUUID(c)
	resourceType := charmresource.TypeContainerImage
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	size := int64(42)
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Type: resourceType,
			},
			Fingerprint: fp,
			Size:        size,
		},
	}

	s.state.EXPECT().GetResource(gomock.Any(), id).Return(res, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Get(
		gomock.Any(),
		id.String(),
	).Return(nil, size-1, nil)

	_, _, err = s.service.OpenResource(c.Context(), id)
	c.Assert(err, tc.ErrorMatches, "unexpected size for stored resource.*")
}

func (s *resourceServiceSuite) TestOpenResourceBadID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenResource(c.Context(), "id")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	args := resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       charmtesting.GenCharmID(c),
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

	err = s.service.SetRepositoryResources(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := errors.New("application error")
	s.state.EXPECT().SetRepositoryResources(gomock.Any(), gomock.Any()).Return(expectedErr)

	// Act
	err = s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       charmtesting.GenCharmID(c),
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
	})

	// Assert
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: "Not-valid",
	})

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesCharmIDNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       "Not-valid",
	})

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.CharmIDNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationNoLastPolled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       charmtesting.GenCharmID(c),
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
		LastPolled: time.Time{},
	})

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationNoInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       charmtesting.GenCharmID(c),
		Info:          nil,
		LastPolled:    time.Now(),
	})

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationInvalidInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act
	err := s.service.SetRepositoryResources(c.Context(), resource.SetRepositoryResourcesArgs{
		ApplicationID: coreapplication.GenID(c),
		CharmID:       charmtesting.GenCharmID(c),
		Info:          []charmresource.Resource{{}, {}}, // Invalid resources
		LastPolled:    time.Now(),
	})

	// Assert
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateResourceRevision
// method for file resource types.
func (s *resourceServiceSuite) TestUpdateResourceRevisionFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resUUID := coreresource.GenUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), resUUID).Return(charmresource.TypeFile, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	expectedUUID := coreresource.GenUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeFile).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeFile).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), resUUID.String()).Return(nil)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newUUID, tc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateResourceRevision
// method for container image resource types.
func (s *resourceServiceSuite) TestUpdateResourceRevisionImage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resUUID := coreresource.GenUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), resUUID).Return(charmresource.TypeContainerImage, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	expectedUUID := coreresource.GenUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeContainerImage).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeContainerImage).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), resUUID.String()).Return(nil)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newUUID, tc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNoOldBlobToRemoveContainerImage tests that no error
// is returned when there is no prior version to delete for container images.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNoOldBlobToRemoveContainerImage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	oldUUID := coreresource.GenUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldUUID).Return(charmresource.TypeContainerImage, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	expectedUUID := coreresource.GenUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeContainerImage).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeContainerImage).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldUUID.String()).Return(containerimageresourcestoreerrors.ContainerImageMetadataNotFound)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newUUID, tc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNoOldBlobToRemoveFile tests that no error
// is returned when there is no prior version to delete for container images.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNoOldBlobToRemoveFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	oldUUID := coreresource.GenUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldUUID).Return(charmresource.TypeFile, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	expectedUUID := coreresource.GenUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeFile).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeFile).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldUUID.String()).Return(objectstoreerrors.ObjectNotFound)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newUUID, tc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNotValid tests that a NotValid error is returned
// for a bad ResourceUUID.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: "deadbeef",
		Revision:     4,
	}

	_, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpdateResourceRevisionFailValidate tests that a NotValid error is returned
// for revision less than zero.
func (s *resourceServiceSuite) TestUpdateResourceRevisionRevisionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: coreresource.GenUUID(c),
		Revision:     -1,
	}

	_, err := s.service.UpdateResourceRevision(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateUploadResource
// method for file resource types.
func (s *resourceServiceSuite) TestUpdateUploadResourceFile(c *tc.C) {
	s.testUpdateUploadResource(c, charmresource.TypeFile)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateUploadResource
// method for container image resource types.
func (s *resourceServiceSuite) TestUpdateUploadResourceImage(c *tc.C) {
	s.testUpdateUploadResource(c, charmresource.TypeContainerImage)
}

func (s *resourceServiceSuite) testUpdateUploadResource(c *tc.C, resourceType charmresource.Type) {
	defer s.setupMocks(c).Finish()

	oldResUUID := coreresource.GenUUID(c)
	newResUUID := coreresource.GenUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldResUUID).Return(resourceType, nil)
	expectedArgs := resource.StateUpdateUploadResourceArgs{
		ResourceType: resourceType,
		ResourceUUID: oldResUUID,
	}
	s.state.EXPECT().UpdateUploadResourceAndDeletePriorVersion(gomock.Any(), expectedArgs).Return(newResUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldResUUID.String()).Return(nil)

	obtainedResourceUUID, err := s.service.UpdateUploadResource(c.Context(), oldResUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedResourceUUID, tc.Equals, newResUUID)
}

// TestAddResourcesBeforeApplication tests the happy path for the
// AddResourcesBeforeApplication method.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rev := 7
	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator:    charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
		ApplicationName: "testme",
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name:     "one",
				Origin:   charmresource.OriginStore,
				Revision: &rev,
			}, {
				Name:   "two",
				Origin: charmresource.OriginUpload,
			},
		},
	}
	retVal := []coreresource.UUID{coreresource.GenUUID(c), coreresource.GenUUID(c)}
	s.state.EXPECT().AddResourcesBeforeApplication(gomock.Any(), args).Return(retVal, nil)

	uuids, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuids, tc.HasLen, 2)
}

// TestAddResourcesBeforeApplicationAppNameNotValid tests that a
// ApplicationNameNotValid error is returned for a bad application name.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationAppNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator: charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
	}

	_, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

// TestAddResourcesBeforeApplicationResNameNotValid tests that a
// ResourceNameNotValid error is returned for a bad resource name.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationResNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator:    charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
		ApplicationName: "testme",
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name: "",
			},
		},
	}

	_, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValid tests that a ArgumentNotValid error is
// returned for a bad Charm ID.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{}

	_, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValidStore tests that a
// ArgumentNotValid error is returned a store resource without a revision.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgumentNotValidStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator:    charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
		ApplicationName: "testme",
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name:   "test",
				Origin: charmresource.OriginStore,
			},
		},
	}

	_, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValidUpload tests that a
// ArgumentNotValid error is returned upload resource with a revision.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgumentNotValidUpload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rev := 8
	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator:    charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
		ApplicationName: "testme",
		ResourceDetails: []resource.AddResourceDetails{
			{
				Name:     "test",
				Origin:   charmresource.OriginUpload,
				Revision: &rev,
			},
		},
	}

	_, err := s.service.AddResourcesBeforeApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestDeleteResourcesAddedBeforeApplication tests the happy path for the
// DeleteResourcesAddedBeforeApplication method.
func (s *resourceServiceSuite) TestDeleteResourcesAddedBeforeApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUIDs := []coreresource.UUID{
		coreresource.GenUUID(c),
		coreresource.GenUUID(c),
	}
	s.state.EXPECT().DeleteResourcesAddedBeforeApplication(gomock.Any(), resourceUUIDs).Return(nil)

	err := s.service.DeleteResourcesAddedBeforeApplication(c.Context(), resourceUUIDs)
	c.Assert(err, tc.ErrorIsNil)
}

// TestDeleteResourcesAddedBeforeApplication tests that a NotValid error is
// returned by the DeleteResourcesAddedBeforeApplication method for invalid
// resource uuids.
func (s *resourceServiceSuite) TestDeleteResourcesAddedBeforeApplicationNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUIDs := []coreresource.UUID{coreresource.GenUUID(c), "deadbeef"}

	err := s.service.DeleteResourcesAddedBeforeApplication(c.Context(), resourceUUIDs)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestImportResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name-1",
		Resources: []resource.ImportResourceInfo{{
			Name:      "app-1-resource-1",
			Origin:    charmresource.OriginStore,
			Revision:  3,
			Timestamp: time.Now().Truncate(time.Second).UTC(),
		}, {
			Name:      "app-1-resource-2",
			Origin:    charmresource.OriginUpload,
			Revision:  -1,
			Timestamp: time.Now().Truncate(time.Second).UTC(),
		}},
		UnitResources: []resource.ImportUnitResourceInfo{{
			UnitName: "unit-name",
			ImportResourceInfo: resource.ImportResourceInfo{
				Name:      "app-1-resource-1",
				Origin:    charmresource.OriginStore,
				Revision:  3,
				Timestamp: time.Now().Truncate(time.Second).UTC(),
			},
		}, {
			ImportResourceInfo: resource.ImportResourceInfo{
				Name:      "app-1-resource-2",
				Origin:    charmresource.OriginUpload,
				Revision:  -1,
				Timestamp: time.Now().Truncate(time.Second).UTC(),
			},
			UnitName: "unit-name",
		}},
	}, {
		ApplicationName: "app-name-2",
		Resources: []resource.ImportResourceInfo{{
			Name:      "app-2-resource-1",
			Origin:    charmresource.OriginStore,
			Revision:  2,
			Timestamp: time.Now().Truncate(time.Second).UTC(),
		}},
	}}

	s.state.EXPECT().ImportResources(gomock.Any(), args)

	// Act:
	err := s.service.ImportResources(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestImportResourcesResourceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.ResourceNotFound)

	err := s.service.ImportResources(c.Context(), nil)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.ApplicationNotFound)

	err := s.service.ImportResources(c.Context(), nil)
	c.Assert(err, tc.ErrorIs, resourceerrors.ApplicationNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.UnitNotFound)

	err := s.service.ImportResources(c.Context(), nil)
	c.Assert(err, tc.ErrorIs, resourceerrors.UnitNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesOriginNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name",
		Resources: []resource.ImportResourceInfo{{
			Name:   "resource-name",
			Origin: 0,
		}},
	}}

	// Act:
	err := s.service.ImportResources(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIs, resourceerrors.OriginNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesResourceNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name",
		Resources: []resource.ImportResourceInfo{{
			Name: "",
		}},
	}}

	// Act:
	err := s.service.ImportResources(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesDuplicateResourceNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name",
		Resources: []resource.ImportResourceInfo{{
			Name:   "name",
			Origin: charmresource.OriginStore,
		}, {
			Name:   "name",
			Origin: charmresource.OriginStore,
		}},
	}}

	// Act:
	err := s.service.ImportResources(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesDuplicateResourceNamesDifferentApps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name1",
		Resources: []resource.ImportResourceInfo{{
			Name:   "name",
			Origin: charmresource.OriginStore,
		}},
	}, {
		ApplicationName: "app-name2",
		Resources: []resource.ImportResourceInfo{{
			Name:   "name",
			Origin: charmresource.OriginStore,
		}},
	}}
	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any())

	// Act:
	err := s.service.ImportResources(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestExportResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := resource.ExportedResources{
		Resources: []coreresource.Resource{{
			RetrievedBy: "admin",
		}},
		UnitResources: []coreresource.UnitResources{{
			Name: "unit1",
			Resources: []coreresource.Resource{{
				RetrievedBy: "admin",
			}},
		}},
	}

	s.state.EXPECT().ExportResources(gomock.Any(), "app-name").Return(expected, nil)

	exportedResources, err := s.service.ExportResources(c.Context(), "app-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exportedResources, tc.DeepEquals, expected)
}

func (s *resourceServiceSuite) TestExportResourceEmptyName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.ExportResources(c.Context(), "")
	c.Assert(err, tc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.resourceStoreGetter = NewMockResourceStoreGetter(ctrl)
	s.resourceStore = NewMockResourceStore(ctrl)

	s.service = NewService(s.state, s.resourceStoreGetter, loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
		s.resourceStoreGetter = nil
		s.resourceStore = nil
		s.service = nil
	})

	return ctrl
}
