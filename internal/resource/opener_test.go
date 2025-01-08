// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcetesting "github.com/juju/juju/core/resource/testing"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/resource"
	"github.com/juju/juju/internal/resource/mocks"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type OpenerSuite struct {
	appName            string
	appID              coreapplication.ID
	unitName           coreunit.Name
	unitUUID           coreunit.UUID
	resourceUUID       coreresource.UUID
	charmURL           *charm.URL
	charmOrigin        state.CharmOrigin
	resourceGetter     *mocks.MockResourceGetter
	resourceService    *mocks.MockResourceService
	applicationService *mocks.MockApplicationService
	limiter            *mocks.MockResourceDownloadLock

	unleash sync.Mutex
}

var _ = gc.Suite(&OpenerSuite{})

func (s *OpenerSuite) TestOpenResource(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := domainresource.Resource{
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
		ApplicationID: "postgreql",
	}
	s.expectServiceMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener(0).OpenResource(context.TODO(), "wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opened.Size, gc.Equals, res.Size)
	c.Check(opened.Fingerprint.String(), gc.Equals, res.Fingerprint.String())
	c.Assert(opened.Close(), jc.ErrorIsNil)
}

func (s *OpenerSuite) TestOpenResourceThrottle(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := domainresource.Resource{
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
		ApplicationID: "postgreql",
	}
	const (
		numConcurrentRequests = 10
		maxConcurrentRequests = 5
	)
	s.expectServiceMethods(res, numConcurrentRequests)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	s.unleash.Lock()
	start := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < numConcurrentRequests; i++ {
		start.Add(1)
		finished.Add(1)
		go func() {
			defer finished.Done()
			start.Done()
			opened, err := s.newOpener(maxConcurrentRequests).OpenResource(context.TODO(), "wal-e")
			c.Assert(err, jc.ErrorIsNil)
			c.Check(opened.Size, gc.Equals, res.Size)
			c.Check(opened.Fingerprint.String(), gc.Equals, res.Fingerprint.String())
			c.Assert(opened.Close(), jc.ErrorIsNil)
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

func (s *OpenerSuite) TestOpenResourceApplication(c *gc.C) {
	defer s.setupMocks(c, false).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := domainresource.Resource{
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
		ApplicationID: "postgreql",
	}
	s.expectServiceMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener(0).OpenResource(context.TODO(), "wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opened.Size, gc.Equals, res.Size)
	c.Check(opened.Fingerprint.String(), gc.Equals, res.Fingerprint.String())
	err = opened.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpenerSuite) setupMocks(c *gc.C, includeUnit bool) *gomock.Controller {
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
	s.resourceGetter = mocks.NewMockResourceGetter(ctrl)
	s.limiter = mocks.NewMockResourceDownloadLock(ctrl)

	s.resourceService = mocks.NewMockResourceService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)

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

func (s *OpenerSuite) expectServiceMethods(res domainresource.Resource, numConcurrentRequests int) {
	s.resourceService.EXPECT().GetResourceUUID(gomock.Any(), domainresource.GetResourceUUIDArgs{
		ApplicationID: s.appID,
		Name:          "wal-e",
	}).Return(s.resourceUUID, nil).AnyTimes()
	if s.unitName != "" {
		s.resourceService.EXPECT().OpenResource(gomock.Any(), s.resourceUUID).DoAndReturn(func(_ context.Context, _ coreresource.UUID) (domainresource.Resource, io.ReadCloser, error) {
			s.unleash.Lock()
			defer s.unleash.Unlock()
			return domainresource.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), resourceerrors.StoredResourceNotFound
		})
	} else {
		s.resourceService.EXPECT().OpenResource(gomock.Any(), s.resourceUUID).Return(domainresource.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), resourceerrors.StoredResourceNotFound)
	}
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(res, nil)
	s.resourceService.EXPECT().StoreResource(gomock.Any(), gomock.Any())

	other := res
	other.ApplicationID = "postgreql"
	if s.unitName != "" {
		s.resourceService.EXPECT().OpenResource(gomock.Any(), s.resourceUUID).Return(other, io.NopCloser(bytes.NewBuffer([]byte{})), nil).Times(numConcurrentRequests)
	} else {
		s.resourceService.EXPECT().OpenResource(gomock.Any(), s.resourceUUID).Return(other, io.NopCloser(bytes.NewBuffer([]byte{})), nil)
	}
}

func (s *OpenerSuite) TestGetResourceErrorReleasesLock(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := domainresource.Resource{
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
		ApplicationID: "postgreql",
	}
	s.resourceService.EXPECT().GetResourceUUID(gomock.Any(), domainresource.GetResourceUUIDArgs{
		ApplicationID: s.appID,
		Name:          "wal-e",
	}).Return(s.resourceUUID, nil)
	s.resourceService.EXPECT().OpenResource(gomock.Any(), s.resourceUUID).DoAndReturn(func(_ context.Context, _ coreresource.UUID) (domainresource.Resource, io.ReadCloser, error) {
		s.unleash.Lock()
		defer s.unleash.Unlock()
		return domainresource.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), resourceerrors.StoredResourceNotFound
	})
	s.resourceService.EXPECT().GetResource(gomock.Any(), s.resourceUUID).Return(res, nil)
	const retryCount = 3
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{}, errors.New("boom")).Times(retryCount)
	s.limiter.EXPECT().Acquire("uuid:postgresql")
	s.limiter.EXPECT().Release("uuid:postgresql")

	opened, err := s.newOpener(-1).OpenResource(context.TODO(), "wal-e")
	c.Assert(err, gc.ErrorMatches, "failed after retrying: boom")
	c.Check(opened, gc.NotNil)
	c.Check(opened.ReadCloser, gc.IsNil)
}

func (s *OpenerSuite) TestSetResourceUnit(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	s.resourceService.EXPECT().GetResourceUUID(gomock.Any(), domainresource.GetResourceUUIDArgs{
		ApplicationID: s.appID,
		Name:          "wal-e",
	}).Return(s.resourceUUID, nil)
	s.resourceService.EXPECT().SetUnitResource(gomock.Any(), s.resourceUUID, s.unitUUID)
	err := s.newOpener(0).SetResource(context.TODO(), "wal-e")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpenerSuite) TestSetResourceUnitError(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	s.resourceService.EXPECT().GetResourceUUID(gomock.Any(), domainresource.GetResourceUUIDArgs{
		ApplicationID: s.appID,
		Name:          "wal-e",
	}).Return(s.resourceUUID, nil)

	expectedErr := errors.New("boom")
	s.resourceService.EXPECT().SetUnitResource(gomock.Any(), s.resourceUUID, s.unitUUID).Return(expectedErr)

	err := s.newOpener(0).SetResource(context.TODO(), "wal-e")
	c.Assert(err, jc.ErrorIs, expectedErr)
}

func (s *OpenerSuite) TestSetResourceApplication(c *gc.C) {
	defer s.setupMocks(c, false).Finish()
	s.resourceService.EXPECT().GetResourceUUID(gomock.Any(), domainresource.GetResourceUUIDArgs{
		ApplicationID: s.appID,
		Name:          "wal-e",
	}).Return(s.resourceUUID, nil)

	expectedErr := errors.New("boom")
	s.resourceService.EXPECT().SetApplicationResource(gomock.Any(), s.resourceUUID).Return(expectedErr)

	err := s.newOpener(0).SetResource(context.TODO(), "wal-e")
	c.Assert(err, jc.ErrorIs, expectedErr)
}

func (s *OpenerSuite) newOpener(maxRequests int) coreresource.Opener {
	var limiter resource.ResourceDownloadLock = resource.NewResourceDownloadLimiter(maxRequests, 0)
	if maxRequests < 0 {
		limiter = s.limiter
	}
	resourceFunc := func(ctx context.Context) (*resource.ResourceRetryClient, error) {
		return resource.NewResourceRetryClientForTest(s.resourceGetter), nil
	}
	return resource.NewResourceOpenerForTest(
		s.unitName,
		s.unitUUID,
		s.appName,
		s.appID,
		s.resourceService,
		s.charmURL,
		s.charmOrigin,
		resourceFunc,
		limiter,
	)
}
