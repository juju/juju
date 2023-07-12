// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	"io"
	"sync"
	"time"

	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type OpenerSuite struct {
	appName        string
	unitName       string
	charmURL       *charm.URL
	charmOrigin    state.CharmOrigin
	resources      *mocks.MockResources
	resourceGetter *mocks.MockResourceGetter
	limiter        *mocks.MockResourceDownloadLock

	unleash sync.Mutex
}

var _ = gc.Suite(&OpenerSuite{})

func (s *OpenerSuite) TestOpenResource(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := resources.Resource{
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
	s.expectCacheMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener(0).OpenResource("wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opened.Resource, gc.DeepEquals, res)
	c.Assert(opened.Close(), jc.ErrorIsNil)
}

func (s *OpenerSuite) TestOpenResourceThrottle(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := resources.Resource{
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
	s.expectCacheMethods(res, numConcurrentRequests)
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
			opened, err := s.newOpener(maxConcurrentRequests).OpenResource("wal-e")
			c.Assert(err, jc.ErrorIsNil)
			c.Check(opened.Resource, gc.DeepEquals, res)
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
	res := resources.Resource{
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
	s.expectCacheMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener(0).OpenResource("wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opened.Resource, gc.DeepEquals, res)
	err = opened.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpenerSuite) setupMocks(c *gc.C, includeUnit bool) *gomock.Controller {
	ctrl := gomock.NewController(c)
	if includeUnit {
		s.unitName = "postgresql/0"
	}
	s.appName = "postgresql"
	s.resourceGetter = mocks.NewMockResourceGetter(ctrl)
	s.resources = mocks.NewMockResources(ctrl)
	s.limiter = mocks.NewMockResourceDownloadLock(ctrl)

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

func (s *OpenerSuite) expectCacheMethods(res resources.Resource, numConcurrentRequests int) {
	if s.unitName != "" {
		s.resources.EXPECT().OpenResourceForUniter("postgresql/0", "wal-e").DoAndReturn(func(unitName, resName string) (resources.Resource, io.ReadCloser, error) {
			s.unleash.Lock()
			defer s.unleash.Unlock()
			return resources.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e")
		})
	} else {
		s.resources.EXPECT().OpenResource("postgresql", "wal-e").Return(resources.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e"))
	}
	s.resources.EXPECT().GetResource("postgresql", "wal-e").Return(res, nil)
	s.resources.EXPECT().SetResource("postgresql", "", res.Resource, gomock.Any(), state.DoNotIncrementCharmModifiedVersion).Return(res, nil)

	other := res
	other.ApplicationID = "postgreql"
	if s.unitName != "" {
		s.resources.EXPECT().OpenResourceForUniter("postgresql/0", "wal-e").Return(other, io.NopCloser(bytes.NewBuffer([]byte{})), nil).Times(numConcurrentRequests)
	} else {
		s.resources.EXPECT().OpenResource("postgresql", "wal-e").Return(other, io.NopCloser(bytes.NewBuffer([]byte{})), nil)
	}
}

func (s *OpenerSuite) TestGetResourceErrorReleasesLock(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := resources.Resource{
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
	s.resources.EXPECT().OpenResourceForUniter("postgresql/0", "wal-e").DoAndReturn(func(unitName, resName string) (resources.Resource, io.ReadCloser, error) {
		s.unleash.Lock()
		defer s.unleash.Unlock()
		return resources.Resource{}, io.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e")
	})
	s.resources.EXPECT().GetResource("postgresql", "wal-e").Return(res, nil)
	const retryCount = 3
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(resource.ResourceData{}, errors.New("boom")).Times(retryCount)
	s.limiter.EXPECT().Acquire("uuid:postgresql")
	s.limiter.EXPECT().Release("uuid:postgresql")

	opened, err := s.newOpener(-1).OpenResource("wal-e")
	c.Assert(err, gc.ErrorMatches, "failed after retrying: boom")
	c.Check(opened, gc.NotNil)
	c.Check(opened.Resource, gc.DeepEquals, resources.Resource{})
	c.Check(opened.ReadCloser, gc.IsNil)
}

func (s *OpenerSuite) newOpener(maxRequests int) *resource.ResourceOpener {
	tag, _ := names.ParseUnitTag("postgresql/0")
	var limiter resource.ResourceDownloadLock = resource.NewResourceDownloadLimiter(maxRequests, 0)
	if maxRequests < 0 {
		limiter = s.limiter
	}
	return resource.NewResourceOpenerForTest(
		s.resources,
		tag,
		s.unitName,
		s.appName,
		s.charmURL,
		s.charmOrigin,
		resource.NewResourceRetryClientForTest(s.resourceGetter),
		limiter,
	)
}
