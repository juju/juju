// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/httprequest.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/http/mocks"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
)

type addCharmSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&addCharmSuite{})

func (s *addCharmSuite) TestAddLocalCharm(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	curl, charmArchive := s.testCharm(c)
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/dummy-1")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/dummy-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(resp, nil).MinTimes(1)

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)
	vers := semversion.MustParse("2.6.6")
	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("ch:wordpress-1"), nil, false, vers)
	c.Assert(err, tc.ErrorMatches, `expected charm URL with local: schema, got "ch:wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), tc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	resp.Header.Set(params.JujuCharmURLHeader, "local:quantal/dummy-42")
	savedURL, err = client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, tc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	resp.Header.Set(params.JujuCharmURLHeader, "local:quantal/dummy-43")
	savedURL, err = client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), tc.Equals, curl.WithRevision(43).String())
}

func (s *addCharmSuite) TestAddLocalCharmFindingHooksError(c *tc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return true, fmt.Errorf("bad zip")
		},
		`bad zip`)
}

func (s *addCharmSuite) TestAddLocalCharmNoHooks(c *tc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return false, nil
		},
		`invalid charm \"dummy\": has no hooks nor dispatch file`)
}

func (s *addCharmSuite) TestAddLocalCharmWithLXDProfile(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/lxd-profile-0")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/lxd-profile-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(resp, nil).MinTimes(1)

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := semversion.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), tc.Equals, "local:quantal/lxd-profile-0")
}

func (s *addCharmSuite) TestAddLocalCharmWithInvalidLXDProfile(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}
	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := semversion.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, tc.ErrorMatches, "invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *addCharmSuite) TestAddLocalCharmWithValidLXDProfileWithForceSucceeds(c *tc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile", c)
}

func (s *addCharmSuite) TestAddLocalCharmWithInvalidLXDProfileWithForceSucceeds(c *tc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile-fail", c)
}

func (s *addCharmSuite) testAddLocalCharmWithForceSucceeds(name string, c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/lxd-profile-0")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/lxd-profile-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(resp, nil).MinTimes(1)

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := semversion.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), tc.Equals, "local:quantal/lxd-profile-0")
}

func (s *addCharmSuite) assertAddLocalCharmFailed(c *tc.C, f func(string) (bool, error), msg string) {
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
	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)
	vers := semversion.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, tc.ErrorMatches, msg)
}

func (s *addCharmSuite) TestAddLocalCharmDefinitelyWithHooks(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, ch := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/dummy-1")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/dummy-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(resp, nil).MinTimes(1)

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	vers := semversion.MustParse("2.6.6")
	savedCURL, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCURL.String(), tc.Equals, curl.String())
}

func (s *addCharmSuite) testCharm(c *tc.C) (*charm.URL, *charm.CharmArchive) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	return curl, charmArchive
}

func (s *addCharmSuite) TestAddLocalCharmError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, charmArchive := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/dummy-1")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/dummy-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(nil, errors.New("boom")).MinTimes(1)

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	vers := semversion.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, tc.ErrorMatches, `.*boom$`)
}

func (s *addCharmSuite) TestMinVersionLocalCharm(c *tc.C) {
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

func testMinVer(t minverTest, c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).AnyTimes()
	mockCaller.EXPECT().ModelTag().Return(testing.ModelTag, false).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
	}
	resp.Header.Add("Content-Type", "application/json")
	resp.Header.Add(params.JujuCharmURLHeader, "local:quantal/dummy-1")
	mockHttpDoer.EXPECT().Do(
		&httpURLMatcher{fmt.Sprintf("http://somewhere.invalid/model-%s/charms/dummy-[a-f0-9]{7}", testing.ModelTag.Id())},
	).Return(resp, nil).AnyTimes()

	putter := charms.NewS3PutterWithHTTPClient(reqClient)
	client := charms.NewLocalCharmClientWithFacade(mockFacadeCaller, nil, putter)

	charmMinVer := semversion.MustParse(t.charm)
	jujuVer := semversion.MustParse(t.juju)

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

type httpURLMatcher struct {
	expectedURL string
}

func (m httpURLMatcher) Matches(x interface{}) bool {
	req, ok := x.(*http.Request)
	if !ok {
		return false
	}
	match, err := regexp.MatchString(m.expectedURL, req.URL.String())
	if err != nil {
		panic("httpURLMatcher regexp invalid")
	}
	return match
}

func (m httpURLMatcher) String() string {
	return fmt.Sprintf("Request URL to match %s", m.expectedURL)
}
