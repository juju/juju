// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/os/ostype"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	coretesting "github.com/juju/juju/core/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/resource/charmhub"
)

type OpenerSuite struct {
	appName              string
	appID                coreapplication.UUID
	unitName             coreunit.Name
	unitUUID             coreunit.UUID
	resourceUUID         coreresource.UUID
	resourceContent      string
	resourceFingerprint  charmresource.Fingerprint
	resourceSize         int64
	resourceReader       io.ReadCloser
	resourceRevision     int
	charmOrigin          charm.Origin
	resourceClient       *MockResourceClient
	resourceClientGetter *MockResourceClientGetter
	resourceService      *MockResourceService
	applicationService   *MockApplicationService
	limiter              *MockResourceDownloadLock

	unleash sync.Mutex
}

func TestOpenerSuite(t *stdtesting.T) {
	tc.Run(t, &OpenerSuite{})
}

// TestOpenUnitResource is a happy path test for opening a unit resource that
// exists in the controllers object store. This is a regression test where we
// were not correctly passing back the resource information including the
// resource uuid.
func (s *OpenerSuite) TestOpenUnitResource(c *tc.C) {
	s.setupMocks(c, false)
	resourceUUID := tc.Must(c, coreresource.NewUUID)
	appName := "postgresql"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitName := tc.Must1(c, coreunit.NewName, "postgresql/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "wal-e",
				Type: 1,
			},
			Origin:      charmresource.OriginStore,
			Revision:    s.resourceRevision,
			Fingerprint: s.resourceFingerprint,
			Size:        s.resourceSize,
		},
		ApplicationName: "postgresql",
		RetrievedBy:     unitName.String(),
		UUID:            resourceUUID,
	}

	charmOrigin := charm.Origin{
		Source: charm.CharmHub,
	}

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.GetApplicationUUIDByUnitName(c.Context(), unitName).Return(
		appUUID, nil,
	)
	appSvcExp.GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	appSvcExp.GetApplicationCharmOrigin(
		gomock.Any(),
		appName,
	).Return(charmOrigin, nil)

	resourceSvcExp := s.resourceService.EXPECT()
	resourceSvcExp.GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationUUID: appUUID,
			Name:            "oci-image",
		},
	).Return(resourceUUID, nil).AnyTimes()

	resourceSvcExp.OpenResource(
		gomock.Any(),
		resourceUUID,
	).Return(res, io.NopCloser(bytes.NewBuffer([]byte("test"))), nil)

	opener, err := NewResourceOpenerForUnit(
		c.Context(),
		ResourceOpenerArgs{
			ResourceService:      s.resourceService,
			ApplicationService:   s.applicationService,
			CharmhubClientGetter: s.resourceClientGetter,
		},
		func() ResourceDownloadLock {
			return noopDownloadResourceLocker{}
		},
		unitName,
	)
	c.Assert(err, tc.ErrorIsNil)

	opened, err := opener.OpenResource(c.Context(), "oci-image")
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.ReadCloser", tc.Ignore)

	c.Check(opened, mc, coreresource.Opened{
		Resource: coreresource.Resource{
			ApplicationName: appName,
			RetrievedBy:     unitName.String(),
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "wal-e",
					Type: 1,
				},
				Origin:      charmresource.OriginStore,
				Revision:    s.resourceRevision,
				Fingerprint: s.resourceFingerprint,
				Size:        s.resourceSize,
			},
			UUID: resourceUUID,
		},
	})
}

// TestOpenUnitResourceCacheMiss tests that when a unit requests a resource and
// it is not available from the local controller cache (cache miss) it is
// downloaded from charmhub.
//
// This also acts as a regression test to show that the resource uuid is
// correctly returned as in the original implementation it was not.
func (s *OpenerSuite) TestOpenUnitResourceCacheMiss(c *tc.C) {
	s.setupMocks(c, false)
	resourceUUID := tc.Must(c, coreresource.NewUUID)
	appName := "postgresql"
	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitName := tc.Must1(c, coreunit.NewName, "postgresql/0")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "wal-e",
				Type: 1,
			},
			Origin:      charmresource.OriginStore,
			Revision:    10,
			Fingerprint: s.resourceFingerprint,
			Size:        s.resourceSize,
		},
		ApplicationName: "postgresql",
		RetrievedBy:     unitName.String(),
		UUID:            resourceUUID,
	}

	charmOrigin := charm.Origin{
		Source: charm.CharmHub,
	}

	charmHubRequest := charmhub.ResourceRequest{
		CharmID: charmhub.CharmID{
			Origin: charmOrigin,
		},
		Name:     "wal-e",
		Revision: 10,
	}

	charmHubResourceData := charmhub.ResourceData{
		ReadCloser: io.NopCloser(bytes.NewBuffer([]byte("test"))),
		Resource:   res.Resource,
	}

	storeResourceArgs := domainresource.StoreResourceArgs{
		Fingerprint:     s.resourceFingerprint,
		ResourceUUID:    resourceUUID,
		RetrievedBy:     unitName.String(),
		RetrievedByType: coreresource.Unit,
		Size:            s.resourceSize,
	}

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.GetApplicationUUIDByUnitName(c.Context(), unitName).Return(
		appUUID, nil,
	)
	appSvcExp.GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	appSvcExp.GetApplicationCharmOrigin(
		gomock.Any(),
		appName,
	).Return(charmOrigin, nil)

	resourceSvcExp := s.resourceService.EXPECT()
	resourceSvcExp.GetResource(gomock.Any(), resourceUUID).Return(res, nil)
	resourceSvcExp.GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationUUID: appUUID,
			Name:            "oci-image",
		},
	).Return(resourceUUID, nil).AnyTimes()

	// Store resource expectations. We use a tc bind here to assert the arg.
	storeResourceMC := tc.NewMultiChecker()
	storeResourceMC.AddExpr("_.Reader", tc.Ignore)
	resourceSvcExp.StoreResource(gomock.Any(), tc.Bind(storeResourceMC, storeResourceArgs))

	// Cache miss for resource.
	resourceSvcExp.OpenResource(
		gomock.Any(),
		resourceUUID,
	).Return(coreresource.Resource{}, nil, resourceerrors.StoredResourceNotFound)
	// Cache hit once downloaded from Charmhub.
	resourceSvcExp.OpenResource(
		gomock.Any(),
		resourceUUID,
	).Return(res, io.NopCloser(bytes.NewBuffer([]byte("test"))), nil)

	resourceClientGetExp := s.resourceClientGetter.EXPECT()
	resourceClientGetExp.GetResourceClient(gomock.Any(), gomock.Any()).Return(
		s.resourceClient,
		nil,
	)

	resourceClientExp := s.resourceClient.EXPECT()
	resourceClientExp.GetResource(gomock.Any(), charmHubRequest).Return(
		charmHubResourceData, nil,
	)

	opener, err := NewResourceOpenerForUnit(
		c.Context(),
		ResourceOpenerArgs{
			ResourceService:      s.resourceService,
			ApplicationService:   s.applicationService,
			CharmhubClientGetter: s.resourceClientGetter,
		},
		func() ResourceDownloadLock {
			return noopDownloadResourceLocker{}
		},
		unitName,
	)
	c.Assert(err, tc.ErrorIsNil)

	opened, err := opener.OpenResource(c.Context(), "oci-image")
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.ReadCloser", tc.Ignore)

	c.Check(opened, mc, coreresource.Opened{
		Resource: coreresource.Resource{
			ApplicationName: appName,
			RetrievedBy:     unitName.String(),
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "wal-e",
					Type: 1,
				},
				Origin:      charmresource.OriginStore,
				Revision:    10,
				Fingerprint: s.resourceFingerprint,
				Size:        s.resourceSize,
			},
			UUID: resourceUUID,
		},
	})
}

func (s *OpenerSuite) TestOpenResourceThrottle(c *tc.C) {
	defer s.setupMocks(c, true).Finish()
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "wal-e",
				Type: 1,
			},
			Origin:      charmresource.OriginStore,
			Revision:    s.resourceRevision,
			Fingerprint: s.resourceFingerprint,
			Size:        s.resourceSize,
		},
		ApplicationName: "postgreql",
	}
	const (
		numConcurrentRequests = 10
		maxConcurrentRequests = 5
	)
	s.expectServiceMethods(res, numConcurrentRequests)
	s.resourceClientGetter.EXPECT().GetResourceClient(
		gomock.Any(), gomock.Any(),
	).Return(
		newResourceRetryClientForTest(c, s.resourceClient),
		nil,
	)
	s.resourceClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(
		charmhub.ResourceData{
			ReadCloser: s.resourceReader,
			Resource:   res.Resource,
		}, nil,
	)

	s.unleash.Lock()
	start := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < numConcurrentRequests; i++ {
		start.Add(1)
		finished.Add(1)
		s.expectNewUnitResourceOpener(c)
		go func() {
			defer finished.Done()
			start.Done()
			opened, err := s.newUnitResourceOpener(
				c,
				maxConcurrentRequests,
			).OpenResource(c.Context(), "wal-e")
			c.Assert(err, tc.ErrorIsNil)
			c.Check(opened.Size, tc.Equals, res.Size)
			c.Check(
				opened.Fingerprint.String(),
				tc.Equals,
				res.Fingerprint.String(),
			)
			c.Assert(opened.Close(), tc.ErrorIsNil)
		}()
	}
	// Let all the test routines queue up then unleash.
	start.Wait()
	s.unleash.Unlock()

	done := make(chan bool)
	go func() {
		finished.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for resources to be fetched")
	}
}

func (s *OpenerSuite) TestOpenResourceApplication(c *tc.C) {
	defer s.setupMocks(c, false).Finish()
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "wal-e",
				Type: 1,
			},
			Origin:      charmresource.OriginStore,
			Revision:    s.resourceRevision,
			Fingerprint: s.resourceFingerprint,
			Size:        s.resourceSize,
		},
		ApplicationName: "postgreql",
	}
	s.expectServiceMethods(res, 1)
	s.resourceClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(
		charmhub.ResourceData{
			ReadCloser: s.resourceReader,
			Resource:   res.Resource,
		}, nil,
	)
	s.resourceClientGetter.EXPECT().GetResourceClient(
		gomock.Any(), gomock.Any(),
	).Return(
		newResourceRetryClientForTest(c, s.resourceClient),
		nil,
	)

	s.applicationService.EXPECT().GetApplicationCharmOrigin(
		gomock.Any(),
		s.appName,
	).Return(s.charmOrigin, nil)

	opened, err := s.newApplicationResourceOpener(c).OpenResource(
		c.Context(),
		"wal-e",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opened.Size, tc.Equals, res.Size)
	c.Check(opened.Fingerprint.String(), tc.Equals, res.Fingerprint.String())
	err = opened.Close()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpenerSuite) setupMocks(c *tc.C, includeUnit bool) *gomock.Controller {
	ctrl := gomock.NewController(c)
	if includeUnit {
		s.unitName = "postgresql/0"
		s.unitUUID = coreunittesting.GenUnitUUID(c)
	} else {
		s.unitName = ""
		s.unitUUID = ""
	}
	s.appName = "postgresql"
	s.appID = tc.Must(c, coreapplication.NewUUID)
	s.resourceUUID = coreresourcetesting.GenResourceUUID(c)
	s.resourceClient = NewMockResourceClient(ctrl)
	s.resourceClientGetter = NewMockResourceClientGetter(ctrl)
	s.limiter = NewMockResourceDownloadLock(ctrl)

	s.resourceService = NewMockResourceService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	s.resourceContent = "the resource content"
	s.resourceSize = int64(len(s.resourceContent))
	s.resourceRevision = 3
	var err error
	s.resourceFingerprint, err = charmresource.GenerateFingerprint(strings.NewReader(s.resourceContent))
	c.Assert(err, tc.ErrorIsNil)
	s.resourceReader = io.NopCloser(strings.NewReader(s.resourceContent))

	rev := 0
	s.charmOrigin = charm.Origin{
		Source:   charm.CharmHub,
		Revision: &rev,
		Channel:  &internalcharm.Channel{Risk: "stable"},
		Platform: charm.Platform{
			Architecture: arch.AMD64,
			OS:           ostype.Ubuntu.String(),
			Channel:      "20.04/stable",
		},
	}
	return ctrl
}

func (s *OpenerSuite) expectServiceMethods(
	res coreresource.Resource,
	numConcurrentRequests int,
) {
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationUUID: s.appID,
			Name:            "wal-e",
		},
	).Return(s.resourceUUID, nil).AnyTimes()
	var retrievedBy string
	var retrevedByType coreresource.RetrievedByType
	if s.unitName != "" {
		retrievedBy = s.unitName.String()
		retrevedByType = coreresource.Unit
		s.resourceService.EXPECT().OpenResource(
			gomock.Any(),
			s.resourceUUID,
		).DoAndReturn(
			func(
				_ context.Context,
				_ coreresource.UUID,
			) (coreresource.Resource, io.ReadCloser, error) {
				s.unleash.Lock()
				defer s.unleash.Unlock()
				return coreresource.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), resourceerrors.StoredResourceNotFound
			},
		)
	} else {
		retrievedBy = s.appName
		retrevedByType = coreresource.Application
		s.resourceService.EXPECT().OpenResource(
			gomock.Any(),
			s.resourceUUID,
		).Return(
			coreresource.Resource{},
			io.NopCloser(bytes.NewBuffer([]byte{})),
			resourceerrors.StoredResourceNotFound,
		)
	}
	s.resourceService.EXPECT().GetResource(
		gomock.Any(),
		s.resourceUUID,
	).Return(res, nil)
	s.resourceService.EXPECT().StoreResource(
		gomock.Any(), domainresource.StoreResourceArgs{
			ResourceUUID:    s.resourceUUID,
			Reader:          s.resourceReader,
			RetrievedBy:     retrievedBy,
			RetrievedByType: retrevedByType,
			Size:            s.resourceSize,
			Fingerprint:     s.resourceFingerprint,
		},
	)

	other := res
	other.ApplicationName = "postgreql"
	if s.unitName != "" {
		s.resourceService.EXPECT().OpenResource(
			gomock.Any(),
			s.resourceUUID,
		).Return(
			other,
			io.NopCloser(bytes.NewBuffer([]byte{})),
			nil,
		).Times(numConcurrentRequests)
	} else {
		s.resourceService.EXPECT().OpenResource(
			gomock.Any(),
			s.resourceUUID,
		).Return(other, io.NopCloser(bytes.NewBuffer([]byte{})), nil)
	}
}

func (s *OpenerSuite) TestGetResourceErrorReleasesLock(c *tc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := coreresource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "wal-e",
				Type: 1,
			},
			Origin:      2,
			Revision:    0,
			Fingerprint: fp,
			Size:        0,
		},
		ApplicationName: "postgreql",
	}
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationUUID: s.appID,
			Name:            "wal-e",
		},
	).Return(s.resourceUUID, nil)
	s.resourceService.EXPECT().OpenResource(
		gomock.Any(),
		s.resourceUUID,
	).DoAndReturn(
		func(_ context.Context, _ coreresource.UUID) (
			coreresource.Resource,
			io.ReadCloser,
			error,
		) {
			s.unleash.Lock()
			defer s.unleash.Unlock()
			return coreresource.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), resourceerrors.StoredResourceNotFound
		},
	)
	s.resourceClientGetter.EXPECT().GetResourceClient(
		gomock.Any(), gomock.Any(),
	).Return(
		newResourceRetryClientForTest(c, s.resourceClient),
		nil,
	)
	s.resourceService.EXPECT().GetResource(
		gomock.Any(),
		s.resourceUUID,
	).Return(res, nil)
	const retryCount = 3
	s.resourceClient.EXPECT().GetResource(gomock.Any(), gomock.Any()).Return(
		charmhub.ResourceData{},
		errors.New("boom"),
	).Times(retryCount)
	s.limiter.EXPECT().Acquire(gomock.Any(), s.appID.String()).Return(nil)
	s.limiter.EXPECT().Release(s.appID.String())

	s.expectNewUnitResourceOpener(c)
	opened, err := s.newUnitResourceOpener(
		c,
		-1,
	).OpenResource(c.Context(), "wal-e")
	c.Assert(err, tc.ErrorMatches, "failed after retrying: boom")
	c.Check(opened, tc.NotNil)
	c.Check(opened.ReadCloser, tc.IsNil)
}

func (s *OpenerSuite) TestSetResourceUsedUnit(c *tc.C) {
	defer s.setupMocks(c, true).Finish()
	s.resourceService.EXPECT().SetUnitResource(
		gomock.Any(),
		s.resourceUUID,
		s.unitUUID,
	)
	s.expectNewUnitResourceOpener(c)
	err := s.newUnitResourceOpener(c, 0).SetResourceUsed(
		c.Context(),
		s.resourceUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpenerSuite) TestSetResourceUsedUnitError(c *tc.C) {
	defer s.setupMocks(c, true).Finish()
	expectedErr := errors.New("boom")
	s.resourceService.EXPECT().SetUnitResource(
		gomock.Any(),
		s.resourceUUID,
		s.unitUUID,
	).Return(expectedErr)

	s.expectNewUnitResourceOpener(c)
	err := s.newUnitResourceOpener(c, 0).SetResourceUsed(
		c.Context(),
		s.resourceUUID,
	)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *OpenerSuite) expectNewUnitResourceOpener(c *tc.C) {
	// Service calls in NewResourceOpenerForUnit.
	s.applicationService.EXPECT().GetApplicationUUIDByUnitName(
		gomock.Any(),
		s.unitName,
	).Return(s.appID, nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(),
		s.unitName,
	).Return(s.unitUUID, nil)

	s.applicationService.EXPECT().GetApplicationCharmOrigin(
		gomock.Any(),
		s.appName,
	).Return(s.charmOrigin, nil)
}

func (s *OpenerSuite) newUnitResourceOpener(
	c *tc.C,
	maxRequests int,
) coreresource.Opener {
	var (
		limiter ResourceDownloadLock
		err     error
	)
	if maxRequests < 0 {
		limiter = s.limiter
	} else {
		limiter, err = NewResourceDownloadLimiter(maxRequests, 0)
		c.Assert(err, tc.ErrorIsNil)
	}

	opener, err := NewResourceOpenerForUnit(
		c.Context(),
		ResourceOpenerArgs{
			ResourceService:      s.resourceService,
			ApplicationService:   s.applicationService,
			CharmhubClientGetter: s.resourceClientGetter,
		},
		func() ResourceDownloadLock {
			return limiter
		},
		s.unitName,
	)
	c.Assert(err, tc.ErrorIsNil)
	return opener
}

func (s *OpenerSuite) newApplicationResourceOpener(c *tc.C) coreresource.Opener {
	// Service calls in NewResourceOpenerForApplication.
	opener, err := NewResourceOpenerForApplication(
		c.Context(),
		ResourceOpenerArgs{
			ResourceService:      s.resourceService,
			ApplicationService:   s.applicationService,
			CharmhubClientGetter: s.resourceClientGetter,
		},
		s.appName,
		s.appID,
	)
	c.Assert(err, tc.ErrorIsNil)
	return opener
}

func newResourceRetryClientForTest(
	c *tc.C,
	cl charmhub.ResourceClient,
) *charmhub.ResourceRetryClient {
	client := charmhub.NewRetryClient(cl, testing.WrapCheckLog(c))
	client.RetryArgs.Delay = time.Millisecond
	return client
}
