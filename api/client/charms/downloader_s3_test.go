// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"net/url"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/internal/downloader"
)

type charmS3DownloaderSuite struct {
}

func TestCharmS3DownloaderSuite(t *stdtesting.T) { tc.Run(t, &charmS3DownloaderSuite{}) }
func (s *charmS3DownloaderSuite) TestCharmOpener(c *tc.C) {
	correctURL, err := url.Parse("ch:mycharm")
	c.Assert(err, tc.IsNil)

	tests := []struct {
		name               string
		req                downloader.Request
		mocks              func(*MockCharmGetter, *basemocks.MockAPICaller)
		expectedErrPattern string
	}{
		{
			name: "invalid ArchiveSha256 in request",
			req: downloader.Request{
				ArchiveSha256: "abcd01",
			},
			expectedErrPattern: "download request with archiveSha256 length 6 not valid",
		},
		{
			name: "invalid URL in request",
			req: downloader.Request{
				ArchiveSha256: "abcd0123",
				URL: &url.URL{
					Scheme: "badscheme",
					Host:   "badhost",
				},
			},
			expectedErrPattern: "did not receive a valid charm URL.*",
		},
		{
			name: "open charm OK",
			req: downloader.Request{
				ArchiveSha256: "abcd0123",
				URL:           correctURL,
			},
			mocks: func(mockGetter *MockCharmGetter, mockCaller *basemocks.MockAPICaller) {

				modelTag := names.NewModelTag("testmodel")
				mockCaller.EXPECT().ModelTag().Return(modelTag, true)
				mockGetter.EXPECT().GetCharm(gomock.Any(), "testmodel", "mycharm-abcd012").MinTimes(1).Return(nil, nil)
			},
		},
	}

	for i, tt := range tests {
		c.Logf("test %d - %s", i, tt.name)

		ctrl := gomock.NewController(c)
		defer ctrl.Finish()

		mockCaller := basemocks.NewMockAPICaller(ctrl)
		mockGetter := NewMockCharmGetter(ctrl)
		if tt.mocks != nil {
			tt.mocks(mockGetter, mockCaller)
		}

		charmOpener := charms.NewS3CharmOpener(mockGetter, mockCaller)
		r, err := charmOpener.OpenCharm(tt.req)

		if tt.expectedErrPattern != "" {
			c.Assert(r, tc.IsNil)
			c.Assert(err, tc.ErrorMatches, tt.expectedErrPattern)
		} else {
			c.Assert(err, tc.IsNil)
		}
	}
}
