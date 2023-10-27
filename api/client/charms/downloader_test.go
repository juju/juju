// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/api/http/mocks"
	coretesting "github.com/juju/juju/testing"
)

type charmDownloaderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&charmDownloaderSuite{})

func (s *charmDownloaderSuite) TestCharmOpener(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)

	charmData := "charmdatablob"
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(charmData)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?file=%2A&url=ch%3Amycharm"},
	).Return(resp, nil).MinTimes(1)

	opener, err := charms.NewCharmOpener(mockCaller)
	c.Assert(err, jc.ErrorIsNil)
	reader, err := opener.OpenCharm("ch:mycharm")

	defer reader.Close()
	c.Assert(err, jc.ErrorIsNil)

	data, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(data, jc.DeepEquals, []byte(charmData))
}
