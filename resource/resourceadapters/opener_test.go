// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters_test

import (
	"bytes"
	"io/ioutil"

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
)

type OpenerSuite struct {
	app                 *mocks.MockApplication
	unit                *mocks.MockUnit
	resources           *mocks.MockResources
	resourceGetter      *mocks.MockResourceGetter
	resourceOpenerState *mocks.MockResourceOpenerState
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
	s.expectCacheMethods(res)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(charmstore.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener().OpenResource("wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opened.Resource, gc.DeepEquals, res)
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
	s.expectCacheMethods(res)
	s.resourceGetter.EXPECT().GetResource(gomock.Any()).Return(charmstore.ResourceData{
		ReadCloser: nil,
		Resource:   res.Resource,
	}, nil)

	opened, err := s.newOpener().OpenResource("wal-e")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opened.Resource, gc.DeepEquals, res)
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
		s.unit.EXPECT().CharmURL().Return(curl, false).AnyTimes()
	} else {
		s.app.EXPECT().CharmURL().Return(curl, false).AnyTimes()
	}
	s.app.EXPECT().Name().Return("postgresql").AnyTimes()
	s.expectCharmOrigin()

	return ctrl
}

func (s *OpenerSuite) expectCharmOrigin() {
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
	})
}

func (s *OpenerSuite) expectCacheMethods(res resource.Resource) {
	if s.unit != nil {
		s.resources.EXPECT().OpenResourceForUniter(gomock.Any(), gomock.Any()).Return(resource.Resource{}, ioutil.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e"))
	} else {
		s.resources.EXPECT().OpenResource(gomock.Any(), gomock.Any()).Return(resource.Resource{}, ioutil.NopCloser(bytes.NewBuffer([]byte{})), errors.NotFoundf("wal-e"))
	}
	s.resources.EXPECT().GetResource("postgresql", "wal-e").Return(res, nil)
	s.resources.EXPECT().SetResource("postgresql", "", res.Resource, gomock.Any(), state.DoNotIncrementCharmModifiedVersion).Return(res, nil)

	other := res
	other.ApplicationID = "postgreql"
	if s.unit != nil {
		s.resources.EXPECT().OpenResourceForUniter(gomock.Any(), gomock.Any()).Return(other, ioutil.NopCloser(bytes.NewBuffer([]byte{})), nil)
	} else {
		s.resources.EXPECT().OpenResource(gomock.Any(), gomock.Any()).Return(other, ioutil.NopCloser(bytes.NewBuffer([]byte{})), nil)
	}
}

func (s *OpenerSuite) newOpener() *resourceadapters.ResourceOpener {
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
	)
}

type testNewClient struct {
	resourceGetter *mocks.MockResourceGetter
}

func (c testNewClient) NewClient() (*resourceadapters.ResourceRetryClient, error) {
	return resourceadapters.NewResourceRetryClientForTest(c.resourceGetter), nil
}
