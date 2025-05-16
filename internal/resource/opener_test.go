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
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	coretesting "github.com/juju/juju/core/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/state"
)

type OpenerSuite struct {
	appName              string
	appID                coreapplication.ID
	unitName             coreunit.Name
	unitUUID             coreunit.UUID
	resourceUUID         coreresource.UUID
	resourceContent      string
	resourceFingerprint  charmresource.Fingerprint
	resourceSize         int64
	resourceReader       io.ReadCloser
	resourceRevision     int
	charmURL             *charm.URL
	charmOrigin          state.CharmOrigin
	resourceClient       *MockResourceClient
	resourceClientGetter *MockResourceClientGetter
	resourceService      *MockResourceService
	state                *MockDeprecatedState
	stateApplication     *MockDeprecatedStateApplication
	stateUnit            *MockDeprecatedStateUnit
	applicationService   *MockApplicationService
	limiter              *MockResourceDownloadLock

	unleash sync.Mutex
}

func TestOpenerSuite(t *stdtesting.T) { tc.Run(t, &OpenerSuite{}) }
func (s *OpenerSuite) TestOpenResource(c *tc.C) {
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
	s.expectServiceMethods(res, 1)
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

	s.expectNewUnitResourceOpener(c)
	opened, err := s.newUnitResourceOpener(
		c,
		0,
	).OpenResource(c.Context(), "wal-e")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opened.Size, tc.Equals, res.Size)
	c.Check(opened.Fingerprint.String(), tc.Equals, res.Fingerprint.String())
	c.Assert(opened.Close(), tc.ErrorIsNil)
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
	s.appID = coreapplicationtesting.GenApplicationUUID(c)
	s.resourceUUID = coreresourcetesting.GenResourceUUID(c)
	s.resourceClient = NewMockResourceClient(ctrl)
	s.resourceClientGetter = NewMockResourceClientGetter(ctrl)
	s.limiter = NewMockResourceDownloadLock(ctrl)

	s.state = NewMockDeprecatedState(ctrl)
	s.stateUnit = NewMockDeprecatedStateUnit(ctrl)
	s.stateApplication = NewMockDeprecatedStateApplication(ctrl)

	s.resourceService = NewMockResourceService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	s.resourceContent = "the resource content"
	s.resourceSize = int64(len(s.resourceContent))
	s.resourceRevision = 3
	var err error
	s.resourceFingerprint, err = charmresource.GenerateFingerprint(strings.NewReader(s.resourceContent))
	c.Assert(err, tc.ErrorIsNil)
	s.resourceReader = io.NopCloser(strings.NewReader(s.resourceContent))

	s.charmURL, _ = charm.ParseURL("postgresql")
	rev := 0
	s.charmOrigin = state.CharmOrigin{
		Source:   "charm-hub",
		Type:     "charm",
		Revision: &rev,
		Channel:  &state.Channel{Risk: "stable"},
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
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
			ApplicationID: s.appID,
			Name:          "wal-e",
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
			ApplicationID: s.appID,
			Name:          "wal-e",
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
	s.limiter.EXPECT().Acquire(gomock.Any(), "uuid:postgresql").Return(nil)
	s.limiter.EXPECT().Release("uuid:postgresql")

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
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationID: s.appID,
			Name:          "wal-e",
		},
	).Return(s.resourceUUID, nil)
	s.resourceService.EXPECT().SetUnitResource(
		gomock.Any(),
		s.resourceUUID,
		s.unitUUID,
	)
	s.expectNewUnitResourceOpener(c)
	err := s.newUnitResourceOpener(c, 0).SetResourceUsed(
		c.Context(),
		"wal-e",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpenerSuite) TestSetResourceUsedUnitError(c *tc.C) {
	defer s.setupMocks(c, true).Finish()
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationID: s.appID,
			Name:          "wal-e",
		},
	).Return(s.resourceUUID, nil)

	expectedErr := errors.New("boom")
	s.resourceService.EXPECT().SetUnitResource(
		gomock.Any(),
		s.resourceUUID,
		s.unitUUID,
	).Return(expectedErr)

	s.expectNewUnitResourceOpener(c)
	err := s.newUnitResourceOpener(c, 0).SetResourceUsed(
		c.Context(),
		"wal-e",
	)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *OpenerSuite) TestSetResourceUsedApplication(c *tc.C) {
	defer s.setupMocks(c, false).Finish()
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationID: s.appID,
			Name:          "wal-e",
		},
	).Return(s.resourceUUID, nil)

	s.resourceService.EXPECT().SetApplicationResource(
		gomock.Any(),
		s.resourceUUID,
	)

	err := s.newApplicationResourceOpener(c).SetResourceUsed(
		c.Context(),
		"wal-e",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *OpenerSuite) TestSetResourceUsedApplicationError(c *tc.C) {
	defer s.setupMocks(c, false).Finish()
	s.resourceService.EXPECT().GetApplicationResourceID(
		gomock.Any(), domainresource.GetApplicationResourceIDArgs{
			ApplicationID: s.appID,
			Name:          "wal-e",
		},
	).Return(s.resourceUUID, nil)

	expectedErr := errors.New("boom")
	s.resourceService.EXPECT().SetApplicationResource(
		gomock.Any(),
		s.resourceUUID,
	).Return(expectedErr)

	err := s.newApplicationResourceOpener(c).SetResourceUsed(
		c.Context(),
		"wal-e",
	)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *OpenerSuite) expectNewUnitResourceOpener(c *tc.C) {
	// Service calls in NewResourceOpenerForUnit.
	s.applicationService.EXPECT().GetApplicationIDByUnitName(
		gomock.Any(),
		s.unitName,
	).Return(s.appID, nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(),
		s.unitName,
	).Return(s.unitUUID, nil)

	// State calls in NewResourceOpenerForUnit.
	s.state.EXPECT().Unit(s.unitName.String()).Return(s.stateUnit, nil)
	s.stateUnit.EXPECT().ApplicationName().Return(s.appName)
	s.state.EXPECT().Application(s.appName).Return(s.stateApplication, nil)
	s.stateUnit.EXPECT().CharmURL().Return(ptr(s.charmURL.String()))
	s.state.EXPECT().ModelUUID().Return("uuid")
	s.stateApplication.EXPECT().CharmOrigin().Return(&s.charmOrigin)
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

	opener, err := newResourceOpenerForUnit(
		c.Context(),
		s.state,
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
	s.applicationService.EXPECT().GetApplicationIDByName(
		gomock.Any(),
		s.appName,
	).Return(s.appID, nil)

	// State calls in NewResourceOpenerForApplication.
	s.state.EXPECT().Application(s.appName).Return(s.stateApplication, nil)
	s.stateApplication.EXPECT().CharmURL().Return(
		ptr(s.charmURL.String()),
		false,
	)
	s.state.EXPECT().ModelUUID().Return("uuid")
	s.stateApplication.EXPECT().CharmOrigin().Return(&s.charmOrigin)
	opener, err := newResourceOpenerForApplication(
		c.Context(),
		s.state,
		ResourceOpenerArgs{
			ResourceService:      s.resourceService,
			ApplicationService:   s.applicationService,
			CharmhubClientGetter: s.resourceClientGetter,
		},
		s.appName,
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

func ptr(s string) *string {
	return &s
}
