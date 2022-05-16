// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/resource/resourceadapters/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type OpenerSuite struct {
	app                 *mocks.MockApplication
	unit                *mocks.MockUnit
	resources           *mocks.MockResources
	resourceGetter      *mocks.MockResourceGetter
	resourceOpenerState *mocks.MockResourceOpenerState

	unleash sync.Mutex
}

var _ = gc.Suite(&OpenerSuite{})

func (s *OpenerSuite) TestOpenResource(c *gc.C) {
	defer s.setupMocks(c, true).Finish()
	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	res := resource.Resource{
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
	s.expectCharmOrigin(1)
	s.expectCacheMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(charmstore.ResourceData{
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
	res := resource.Resource{
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
	s.expectCharmOrigin(numConcurrentRequests)
	s.expectCacheMethods(res, numConcurrentRequests)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(charmstore.ResourceData{
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
	res := resource.Resource{
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
	s.expectCharmOrigin(1)
	s.expectCacheMethods(res, 1)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(charmstore.ResourceData{
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
		s.unit = mocks.NewMockUnit(ctrl)
	} else {
		s.unit = nil
	}
	s.app = mocks.NewMockApplication(ctrl)
	s.resourceGetter = mocks.NewMockResourceGetter(ctrl)
	s.resources = mocks.NewMockResources(ctrl)
	s.resourceOpenerState = mocks.NewMockResourceOpenerState(ctrl)
	s.resourceOpenerState.EXPECT().ModelUUID().Return("00000000-0000-0000-0000-000000000000").AnyTimes()

	curl, _ := charm.ParseURL("postgresql")
	if s.unit != nil {
		s.unit.EXPECT().ApplicationName().Return("postgresql").AnyTimes()
		s.unit.EXPECT().Application().Return(s.app, nil).AnyTimes()
		s.unit.EXPECT().CharmURL().Return(curl, nil).AnyTimes()
	} else {
		s.app.EXPECT().CharmURL().Return(curl, false).AnyTimes()
	}
	s.app.EXPECT().Name().Return("postgresql").AnyTimes()

	return ctrl
}

func (s *OpenerSuite) expectCharmOrigin(numConcurrentRequests int) {
	rev := 0
	s.app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source:   "charm-hub",
		Type:     "charm",
		Revision: &rev,
		Channel:  &state.Channel{Risk: "stable"},
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "focal",
		},
	}).Times(numConcurrentRequests)
}

func (s *OpenerSuite) expectCacheMethods(res resource.Resource, numConcurrentRequests int) {
	if s.unit != nil {
		s.resources.EXPECT().OpenResourceForUniter(gomock.Any(), gomock.Any()).DoAndReturn(func(unit resource.Unit, name string) (resource.Resource, io.ReadCloser, error) {
			s.unleash.Lock()
			defer s.unleash.Unlock()
			return resource.Resource{}, ioutil.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e")
		})
	} else {
		s.resources.EXPECT().OpenResource(gomock.Any(), gomock.Any()).Return(resource.Resource{}, ioutil.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e"))
	}
	s.resources.EXPECT().GetResource("postgresql", "wal-e").Return(res, nil)
	s.resources.EXPECT().SetResource("postgresql", "", res.Resource, gomock.Any(), state.DoNotIncrementCharmModifiedVersion).Return(res, nil)

	other := res
	other.ApplicationID = "postgreql"
	if s.unit != nil {
		s.resources.EXPECT().OpenResourceForUniter(gomock.Any(), gomock.Any()).Return(other, ioutil.NopCloser(bytes.NewBuffer([]byte{})), nil).Times(numConcurrentRequests)
	} else {
		s.resources.EXPECT().OpenResource(gomock.Any(), gomock.Any()).Return(other, ioutil.NopCloser(bytes.NewBuffer([]byte{})), nil)
	}
}

func (s *OpenerSuite) newOpener(maxRequests int) *resourceadapters.ResourceOpener {
	tag, _ := names.ParseUnitTag("postgresql/0")
	// preserve nil
	unit := resourceadapters.Unit(nil)
	if s.unit != nil {
		unit = s.unit
	}
	return resourceadapters.NewResourceOpenerForTest(
		s.resourceOpenerState,
		s.resources,
		tag,
		unit,
		s.app,
		func(st resourceadapters.ResourceOpenerState) resourceadapters.ResourceRetryClientGetter {
			return &testNewClient{
				resourceGetter: s.resourceGetter,
			}
		},
		maxRequests,
	)
}

type testNewClient struct {
	resourceGetter *mocks.MockResourceGetter
}

func (c testNewClient) NewClient() (*resourceadapters.ResourceRetryClient, error) {
	return resourceadapters.NewResourceRetryClientForTest(c.resourceGetter), nil
}
