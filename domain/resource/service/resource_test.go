// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"io"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	charmtesting "github.com/juju/juju/core/charm/testing"
	coreerrors "github.com/juju/juju/core/errors"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	storetesting "github.com/juju/juju/core/resource/store/testing"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application/charm"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

type resourceServiceSuite struct {
	jujutesting.IsolationSuite

	state               *MockState
	resourceStoreGetter *MockResourceStoreGetter
	resourceStore       *MockResourceStore

	service *Service
}

var _ = gc.Suite(&resourceServiceSuite{})

func (s *resourceServiceSuite) TestDeleteApplicationResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(nil)

	err := s.service.DeleteApplicationResources(context.
		Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesBadArgs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteApplicationResources(context.
		Background(), "not an application ID")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationIDNotValid,
		gc.Commentf("Application ID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteApplicationResourcesUnexpectedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().DeleteApplicationResources(gomock.Any(),
		appUUID).Return(stateError)

	err := s.service.DeleteApplicationResources(context.
		Background(), appUUID)
	c.Assert(err, jc.ErrorIs, stateError,
		gc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestDeleteUnitResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(nil)

	err := s.service.DeleteUnitResources(context.
		Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesBadArgs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteUnitResources(context.
		Background(), "not an unit UUID")
	c.Assert(err, jc.ErrorIs, resourceerrors.UnitUUIDNotValid,
		gc.Commentf("Unit UUID should be stated as not valid"))
}

func (s *resourceServiceSuite) TestDeleteUnitResourcesUnexpectedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	stateError := errors.New("unexpected error")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().DeleteUnitResources(gomock.Any(),
		unitUUID).Return(stateError)

	err := s.service.DeleteUnitResources(context.
		Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, stateError,
		gc.Commentf("Should return underlying error"))
}

func (s *resourceServiceSuite) TestGetApplicationResourceID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcetesting.GenResourceUUID(c)
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
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, nil)

	ret, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "app-id", "res-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ret, gc.Equals, retID)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameResourceNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, resourceerrors.ResourceNotFound)

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "app-id", "res-name")
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameApplicationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	retID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().GetResourceUUIDByApplicationAndResourceName(gomock.Any(), "app-id", "res-name").Return(retID, resourceerrors.ApplicationNotFound)
	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "app-id", "res-name")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNotFound)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameEmptyAppID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "", "res-name")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameBadAppID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "9", "res-name")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

func (s *resourceServiceSuite) TestGetResourceUUIDByApplicationAndResourceNameBadName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetResourceUUIDByApplicationAndResourceName(context.Background(), "app-id", "")
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestListResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	expectedList := coreresource.ApplicationResources{
		Resources: []coreresource.Resource{{
			RetrievedBy: "admin",
		}},
	}
	s.state.EXPECT().ListResources(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.ListResources(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedList, gc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestGetResourcesByApplicationIDBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.GetResourcesByApplicationID(context.Background(), "")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestGetResourcesByApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	expectedList := []coreresource.Resource{{
		RetrievedBy: "admin",
	}}
	s.state.EXPECT().GetResourcesByApplicationID(gomock.Any(), id).Return(expectedList, nil)

	obtainedList, err := s.service.GetResourcesByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedList, gc.DeepEquals, expectedList)
}

func (s *resourceServiceSuite) TestListResourcesBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service.ListResources(context.Background(), "")
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestGetResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := resourcetesting.GenResourceUUID(c)
	expectedRes := coreresource.Resource{
		RetrievedBy: "admin",
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

func (s *resourceServiceSuite) TestSetApplicationResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().SetApplicationResource(gomock.Any(), resourceUUID)

	err := s.service.SetApplicationResource(
		context.Background(),
		resourceUUID,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetApplicationResourceBadResourceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationResource(context.Background(), "bad-uuid")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			Size:            size,
			Fingerprint:     fp,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceRemovedOnRecordError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
			Size:            size,
			Fingerprint:     fp,
		},
	)
	c.Assert(err, jc.ErrorIs, expectedErr)
}

func (s *resourceServiceSuite) TestStoreResourceDoesNotStoreIdenticalBlobContainer(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: We only expect a call to GetResource, not to
	// RecordStoredResource since the blob is identical.
	resourceUUID := resourcetesting.GenResourceUUID(c)

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)

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
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourceUUID,
			Reader:       reader,
			Fingerprint:  fp,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceServiceSuite) TestStoreResourceDoesNotStoreIdenticalBlobFile(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	resourceUUID := resourcetesting.GenResourceUUID(c)

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)

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
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourceUUID,
			Reader:       reader,
			Fingerprint:  fp,
		},
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceAlreadyExists)
}

func (s *resourceServiceSuite) TestStoreResourceBadUUID(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceNilReader(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, gc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceNegativeSize(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Size:         -1,
		},
	)
	c.Assert(err, gc.ErrorMatches, "invalid size: -1")
}

func (s *resourceServiceSuite) TestStoreResourceZeroFingerprint(c *gc.C) {
	err := s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Fingerprint:  charmresource.Fingerprint{},
		},
	)
	c.Assert(err, gc.ErrorMatches, "invalid fingerprint")
}

func (s *resourceServiceSuite) TestStoreResourceBadRetrievedBy(c *gc.C) {
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.Unknown,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceRevisionNotValidOriginUpload(c *gc.C) {
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.User,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceRevisionNotValidOriginStore(c *gc.C) {
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.StoreResource(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.User,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile

	reader := bytes.NewBufferString("spamspamspam")
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourceUUID,
			Reader:          reader,
			Size:            size,
			Fingerprint:     fp,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrievedByType,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadUUID(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: "bad-uuid",
		},
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionNilReader(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       nil,
		},
	)
	c.Assert(err, gc.ErrorMatches, "cannot have nil reader")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionNegativeSize(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Size:         -1,
		},
	)
	c.Assert(err, gc.ErrorMatches, "invalid size: -1")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionZeroFingerprint(c *gc.C) {
	err := s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID: resourcetesting.GenResourceUUID(c),
			Reader:       bytes.NewBufferString("spam"),
			Fingerprint:  charmresource.Fingerprint{},
		},
	)
	c.Assert(err, gc.ErrorMatches, "invalid fingerprint")
}

func (s *resourceServiceSuite) TestStoreResourceAndIncrementCharmModifiedVersionBadRetrievedBy(c *gc.C) {
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.StoreResourceAndIncrementCharmModifiedVersion(
		context.Background(),
		resource.StoreResourceArgs{
			ResourceUUID:    resourcetesting.GenResourceUUID(c),
			Reader:          bytes.NewBufferString("spam"),
			Fingerprint:     fp,
			RetrievedBy:     "bob",
			RetrievedByType: coreresource.Unknown,
		},
	)
	c.Assert(err, jc.ErrorIs, resourceerrors.RetrievedByTypeNotValid)
}

func (s *resourceServiceSuite) TestSetUnitResource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().SetUnitResource(gomock.Any(), resourceUUID, unitUUID).Return(nil)

	err := s.service.SetUnitResource(context.Background(), resourceUUID, unitUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadResourceUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	err := s.service.SetUnitResource(context.Background(), "bad-uuid", unitUUID)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetUnitResourceBadUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUID := resourcetesting.GenResourceUUID(c)

	err := s.service.SetUnitResource(context.Background(), resourceUUID, "bad-uuid")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestOpenResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	reader := io.NopCloser(bytes.NewBufferString("spam"))
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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

	obtainedRes, obtainedReader, err := s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedRes, gc.DeepEquals, res)
	c.Assert(obtainedReader, gc.DeepEquals, reader)
}

func (s *resourceServiceSuite) TestOpenResourceFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeFile
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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

	_, _, err = s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

func (s *resourceServiceSuite) TestOpenResourceContainerImageNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeContainerImage
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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

	_, _, err = s.service.OpenResource(context.Background(), id)
	c.Assert(err, jc.ErrorIs, resourceerrors.StoredResourceNotFound)
}

// TestOpenResourceUnexpectedSize checks that an error is returned if the size
// of the resource in the object store is not what was expected.
func (s *resourceServiceSuite) TestOpenResourceUnexpectedSize(c *gc.C) {
	defer s.setupMocks(c).Finish()
	id := resourcetesting.GenResourceUUID(c)
	resourceType := charmresource.TypeContainerImage
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
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

	_, _, err = s.service.OpenResource(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "unexpected size for stored resource.*")
}

func (s *resourceServiceSuite) TestOpenResourceBadID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.OpenResource(context.Background(), "id")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	args := resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
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

	err = s.service.SetRepositoryResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := errors.New("application error")
	s.state.EXPECT().SetRepositoryResources(gomock.Any(), gomock.Any()).Return(expectedErr)

	// Act
	err = s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
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
	c.Assert(err, jc.ErrorIs, expectedErr)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationIDNotValid(c *gc.C) {
	// Act
	err := s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: "Not-valid",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationIDNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesCharmIDNotValid(c *gc.C) {
	// Act
	err := s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		CharmID:       "Not-valid",
	})

	// Assert
	c.Assert(err, jc.ErrorIs, resourceerrors.CharmIDNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationNoLastPolled(c *gc.C) {
	// Arrange
	fp, err := charmresource.NewFingerprint(fingerprint)
	c.Assert(err, jc.ErrorIsNil)

	// Act
	err = s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
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
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationNoInfo(c *gc.C) {
	// Act
	err := s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		CharmID:       charmtesting.GenCharmID(c),
		Info:          nil,
		LastPolled:    time.Now(),
	})

	// Assert
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) TestSetRepositoryResourcesApplicationInvalidInfo(c *gc.C) {
	// Act
	err := s.service.SetRepositoryResources(context.Background(), resource.SetRepositoryResourcesArgs{
		ApplicationID: applicationtesting.GenApplicationUUID(c),
		CharmID:       charmtesting.GenCharmID(c),
		Info:          []charmresource.Resource{{}, {}}, // Invalid resources
		LastPolled:    time.Now(),
	})

	// Assert
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateResourceRevision
// method for file resource types.
func (s *resourceServiceSuite) TestUpdateResourceRevisionFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resUUID := resourcetesting.GenResourceUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), resUUID).Return(charmresource.TypeFile, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	expectedUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeFile).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeFile).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), resUUID.String()).Return(nil)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newUUID, gc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateResourceRevision
// method for container image resource types.
func (s *resourceServiceSuite) TestUpdateResourceRevisionImage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resUUID := resourcetesting.GenResourceUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), resUUID).Return(charmresource.TypeContainerImage, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	expectedUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeContainerImage).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeContainerImage).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), resUUID.String()).Return(nil)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newUUID, gc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNoOldBlobToRemoveContainerImage tests that no error
// is returned when there is no prior version to delete for container images.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNoOldBlobToRemoveContainerImage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oldUUID := resourcetesting.GenResourceUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldUUID).Return(charmresource.TypeContainerImage, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	expectedUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeContainerImage).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeContainerImage).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldUUID.String()).Return(containerimageresourcestoreerrors.ContainerImageMetadataNotFound)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newUUID, gc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNoOldBlobToRemoveFile tests that no error
// is returned when there is no prior version to delete for container images.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNoOldBlobToRemoveFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	oldUUID := resourcetesting.GenResourceUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldUUID).Return(charmresource.TypeFile, nil)
	expectedArgs := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	expectedUUID := resourcetesting.GenResourceUUID(c)
	s.state.EXPECT().UpdateResourceRevisionAndDeletePriorVersion(gomock.Any(), expectedArgs, charmresource.TypeFile).Return(expectedUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), charmresource.TypeFile).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldUUID.String()).Return(objectstoreerrors.ObjectNotFound)

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: oldUUID,
		Revision:     4,
	}
	newUUID, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newUUID, gc.Equals, expectedUUID)
}

// TestUpdateResourceRevisionNotValid tests that a NotValid error is returned
// for a bad ResourceUUID.
func (s *resourceServiceSuite) TestUpdateResourceRevisionNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: "deadbeef",
		Revision:     4,
	}

	_, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestUpdateResourceRevisionFailValidate tests that a NotValid error is returned
// for revision less than zero.
func (s *resourceServiceSuite) TestUpdateResourceRevisionRevisionNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.UpdateResourceRevisionArgs{
		ResourceUUID: resourcetesting.GenResourceUUID(c),
		Revision:     -1,
	}

	_, err := s.service.UpdateResourceRevision(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateUploadResource
// method for file resource types.
func (s *resourceServiceSuite) TestUpdateUploadResourceFile(c *gc.C) {
	s.testUpdateUploadResource(c, charmresource.TypeFile)
}

// TestUpdateResourceRevisionFile tests the happy path for the UpdateUploadResource
// method for container image resource types.
func (s *resourceServiceSuite) TestUpdateUploadResourceImage(c *gc.C) {
	s.testUpdateUploadResource(c, charmresource.TypeContainerImage)
}

func (s *resourceServiceSuite) testUpdateUploadResource(c *gc.C, resourceType charmresource.Type) {
	defer s.setupMocks(c).Finish()

	oldResUUID := resourcetesting.GenResourceUUID(c)
	newResUUID := resourcetesting.GenResourceUUID(c)

	s.state.EXPECT().GetResourceType(gomock.Any(), oldResUUID).Return(resourceType, nil)
	expectedArgs := resource.StateUpdateUploadResourceArgs{
		ResourceType: resourceType,
		ResourceUUID: oldResUUID,
	}
	s.state.EXPECT().UpdateUploadResourceAndDeletePriorVersion(gomock.Any(), expectedArgs).Return(newResUUID, nil)
	s.resourceStoreGetter.EXPECT().GetResourceStore(gomock.Any(), resourceType).Return(s.resourceStore, nil)
	s.resourceStore.EXPECT().Remove(gomock.Any(), oldResUUID.String()).Return(nil)

	obtainedResourceUUID, err := s.service.UpdateUploadResource(context.Background(), oldResUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedResourceUUID, gc.Equals, newResUUID)
}

// TestAddResourcesBeforeApplication tests the happy path for the
// AddResourcesBeforeApplication method.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplication(c *gc.C) {
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
	retVal := []coreresource.UUID{resourcetesting.GenResourceUUID(c), resourcetesting.GenResourceUUID(c)}
	s.state.EXPECT().AddResourcesBeforeApplication(gomock.Any(), args).Return(retVal, nil)

	uuids, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuids, gc.HasLen, 2)
}

// TestAddResourcesBeforeApplicationAppNameNotValid tests that a
// ApplicationNameNotValid error is returned for a bad application name.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationAppNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{
		CharmLocator: charm.CharmLocator{Name: "testcharm", Source: charm.CharmHubSource},
	}

	_, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNameNotValid)
}

// TestAddResourcesBeforeApplicationResNameNotValid tests that a
// ResourceNameNotValid error is returned for a bad resource name.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationResNameNotValid(c *gc.C) {
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

	_, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValid tests that a ArgumentNotValid error is
// returned for a bad Charm ID.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	args := resource.AddResourcesBeforeApplicationArgs{}

	_, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValidStore tests that a
// ArgumentNotValid error is returned a store resource without a revision.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgumentNotValidStore(c *gc.C) {
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

	_, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestAddResourcesBeforeApplicationArgNotValidUpload tests that a
// ArgumentNotValid error is returned upload resource with a revision.
func (s *resourceServiceSuite) TestAddResourcesBeforeApplicationArgumentNotValidUpload(c *gc.C) {
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

	_, err := s.service.AddResourcesBeforeApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

// TestDeleteResourcesAddedBeforeApplication tests the happy path for the
// DeleteResourcesAddedBeforeApplication method.
func (s *resourceServiceSuite) TestDeleteResourcesAddedBeforeApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceUUIDs := []coreresource.UUID{
		resourcetesting.GenResourceUUID(c),
		resourcetesting.GenResourceUUID(c),
	}
	s.state.EXPECT().DeleteResourcesAddedBeforeApplication(gomock.Any(), resourceUUIDs).Return(nil)

	err := s.service.DeleteResourcesAddedBeforeApplication(context.Background(), resourceUUIDs)
	c.Assert(err, jc.ErrorIsNil)
}

// TestDeleteResourcesAddedBeforeApplication tests that a NotValid error is
// returned by the DeleteResourcesAddedBeforeApplication method for invalid
// resource uuids.
func (s *resourceServiceSuite) TestDeleteResourcesAddedBeforeApplicationNotValid(c *gc.C) {
	resourceUUIDs := []coreresource.UUID{resourcetesting.GenResourceUUID(c), "deadbeef"}

	err := s.service.DeleteResourcesAddedBeforeApplication(context.Background(), resourceUUIDs)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *resourceServiceSuite) TestImportResources(c *gc.C) {
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
	err := s.service.ImportResources(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestImportResourcesResourceNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.ResourceNotFound)

	err := s.service.ImportResources(context.Background(), nil)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesApplicationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.ApplicationNotFound)

	err := s.service.ImportResources(context.Background(), nil)
	c.Assert(err, jc.ErrorIs, resourceerrors.ApplicationNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportResources(gomock.Any(), gomock.Any()).Return(resourceerrors.UnitNotFound)

	err := s.service.ImportResources(context.Background(), nil)
	c.Assert(err, jc.ErrorIs, resourceerrors.UnitNotFound)
}

func (s *resourceServiceSuite) TestImportResourcesOriginNotValid(c *gc.C) {
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
	err := s.service.ImportResources(context.Background(), args)

	// Assert:
	c.Assert(err, jc.ErrorIs, resourceerrors.OriginNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesResourceNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Create arguments for ImportResources.
	args := []resource.ImportResourcesArg{{
		ApplicationName: "app-name",
		Resources: []resource.ImportResourceInfo{{
			Name: "",
		}},
	}}

	// Act:
	err := s.service.ImportResources(context.Background(), args)

	// Assert:
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesDuplicateResourceNames(c *gc.C) {
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
	err := s.service.ImportResources(context.Background(), args)

	// Assert:
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}

func (s *resourceServiceSuite) TestImportResourcesDuplicateResourceNamesDifferentApps(c *gc.C) {
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
	err := s.service.ImportResources(context.Background(), args)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourceServiceSuite) TestExportResource(c *gc.C) {
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

	exportedResources, err := s.service.ExportResources(context.Background(), "app-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exportedResources, gc.DeepEquals, expected)
}

func (s *resourceServiceSuite) TestExportResourceEmptyName(c *gc.C) {
	_, err := s.service.ExportResources(context.Background(), "")
	c.Assert(err, jc.ErrorIs, resourceerrors.ArgumentNotValid)
}

func (s *resourceServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.resourceStoreGetter = NewMockResourceStoreGetter(ctrl)
	s.resourceStore = NewMockResourceStore(ctrl)

	s.service = NewService(s.state, s.resourceStoreGetter, loggertesting.WrapCheckLog(c))

	return ctrl
}
