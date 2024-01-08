// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/http/mocks"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type addCharmSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&addCharmSuite{})

func (s *addCharmSuite) TestAddLocalCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	curl, charmArchive := s.testCharm(c)
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)
	vers := version.MustParse("2.6.6")
	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("ch:wordpress-1"), nil, false, vers)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "ch:wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	resp.Body = io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-42"}`))
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	err = charmDir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)
	savedURL, err = client.AddLocalCharm(curl, charmDir, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	resp.Body = io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-43"}`))
	savedURL, err = client.AddLocalCharm(curl, charmDir, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *addCharmSuite) TestAddLocalCharmFindingHooksError(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return true, fmt.Errorf("bad zip")
		},
		`bad zip`)
}

func (s *addCharmSuite) TestAddLocalCharmNoHooks(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return false, nil
		},
		`invalid charm \"dummy\": has no hooks nor dispatch file`)
}

func (s *addCharmSuite) TestAddLocalCharmWithLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/lxd-profile-0"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=0&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, "local:quantal/lxd-profile-0")
}

func (s *addCharmSuite) TestAddLocalCharmWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}
	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.ErrorMatches, "invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *addCharmSuite) TestAddLocalCharmWithValidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile", c)
}

func (s *addCharmSuite) TestAddLocalCharmWithInvalidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile-fail", c)
}

func (s *addCharmSuite) testAddLocalCharmWithForceSucceeds(name string, c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/lxd-profile-0"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=0&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, "local:quantal/lxd-profile-0")
}

func (s *addCharmSuite) assertAddLocalCharmFailed(c *gc.C, f func(string) (bool, error), msg string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, ch := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, f)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}
	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)
	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *addCharmSuite) TestAddLocalCharmDefinitelyWithHooks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, ch := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	vers := version.MustParse("2.6.6")
	savedCURL, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCURL.String(), gc.Equals, curl.String())
}

func (s *addCharmSuite) testCharm(c *gc.C) (*charm.URL, charm.Charm) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	return curl, charmArchive
}

func (s *addCharmSuite) TestAddLocalCharmError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, charmArchive := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(nil, errors.New("boom")).MinTimes(1)

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.ErrorMatches, `.*boom$`)
}

func (s *addCharmSuite) TestMinVersionLocalCharm(c *gc.C) {
	tests := []minverTest{
		{"2.0.0", "1.0.0", false, true},
		{"1.0.0", "2.0.0", false, false},
		{"1.25.0", "1.24.0", false, true},
		{"1.24.0", "1.25.0", false, false},
		{"1.25.1", "1.25.0", false, true},
		{"1.25.0", "1.25.1", false, false},
		{"1.25.0", "1.25.0", false, true},
		{"1.25.0", "1.25-alpha1", false, true},
		{"1.25-alpha1", "1.25.0", false, true},
		{"2.0.0", "1.0.0", true, true},
		{"1.0.0", "2.0.0", true, false},
		{"1.25.0", "1.24.0", true, true},
		{"1.24.0", "1.25.0", true, false},
		{"1.25.1", "1.25.0", true, true},
		{"1.25.0", "1.25.1", true, false},
		{"1.25.0", "1.25.0", true, true},
		{"1.25.0", "1.25-alpha1", true, true},
		{"1.25-alpha1", "1.25.0", true, true},
	}
	for _, t := range tests {
		testMinVer(t, c)
	}
}

type minverTest struct {
	juju  string
	charm string
	force bool
	ok    bool
}

func testMinVer(t minverTest, c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.Background()).AnyTimes()
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).AnyTimes()

	httpPutter := charms.NewHTTPPutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, httpPutter)

	charmMinVer := version.MustParse(t.charm)
	jujuVer := version.MustParse(t.juju)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	charmArchive.Meta().MinJujuVersion = charmMinVer

	_, err := client.AddLocalCharm(curl, charmArchive, t.force, jujuVer)

	if t.ok {
		if err != nil {
			c.Errorf("Unexpected non-nil error for jujuver %v, minver %v: %#v", t.juju, t.charm, err)
		}
	} else {
		if err == nil {
			c.Errorf("Unexpected nil error for jujuver %v, minver %v", t.juju, t.charm)
		} else if !jujuversion.IsMinVersionError(err) {
			c.Errorf("Wrong error for jujuver %v, minver %v: expected minVersionError, got: %#v", t.juju, t.charm, err)
		}
	}
}
