// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub_test

import (
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/deployment"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub/transport"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/internal/testhelpers"
)

type CharmHubSuite struct {
	testhelpers.IsolationSuite

	client     *MockCharmHub
	downloader *MockDownloader
}

func TestCharmHubSuite(t *testing.T) {
	tc.Run(t, &CharmHubSuite{})
}

func (s *CharmHubSuite) TestGetResource(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = NewMockCharmHub(ctrl)
	s.downloader = NewMockDownloader(ctrl)

	fingerprint := "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b"
	fp, _ := charmresource.ParseFingerprint(fingerprint)
	size := int64(42)
	reader := io.NopCloser(strings.NewReader("blob"))
	resURL := s.expectRefresh(size, fingerprint)
	parsedURL, err := url.Parse(resURL)
	c.Assert(err, tc.ErrorIsNil)

	s.downloader.EXPECT().Download(gomock.Any(), parsedURL, fingerprint, size).Return(reader, nil)

	cl := s.newCharmHubClient(c)
	rev := 42
	result, err := cl.GetResource(
		c.Context(),
		charmhub.ResourceRequest{
			CharmID: charmhub.CharmID{
				Origin: application.CharmOrigin{
					CharmhubIdentifier: "mycharmhubid",
					Channel:            &deployment.Channel{Risk: "stable"},
					Revision:           rev,
					Platform: deployment.Platform{
						Architecture: architecture.AMD64,
						OSType:       deployment.Ubuntu,
						Channel:      "20.04/stable",
					},
				},
			},
			Name:     "wal-e",
			Revision: 8,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.Resource, tc.DeepEquals, charmresource.Resource{
		Meta: charmresource.Meta{
			Name: "wal-e",
			Type: 1,
		},
		Origin:      2,
		Revision:    8,
		Fingerprint: fp,
		Size:        size,
	})

	c.Assert(result.ReadCloser, tc.Equals, reader)
}

func (s *CharmHubSuite) newCharmHubClient(c *tc.C) *charmhub.CharmHubClient {
	return charmhub.NewCharmHubClientForTest(s.client, s.downloader, loggertesting.WrapCheckLog(c))
}

func (s *CharmHubSuite) expectRefresh(size int64, hash string) (url string) {
	url = "https://api.staging.charmhub.io/api/v1/charms/download/jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD_208.charm"
	resp := []transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{
				Download: transport.Download{
					HashSHA256: "c97e1efc5367d2fdcfdf29f4a2243b13765cc9cbdfad19627a29ac903c01ae63",
					Size:       5487460,
					URL:        url},
				ID:   "jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD",
				Name: "ubuntu",
				Resources: []transport.ResourceRevision{
					{
						Download: transport.Download{
							HashSHA384: hash,
							Size:       int(size),
							URL:        url},
						Name:     "wal-e",
						Revision: 8,
						Type:     "file",
					},
				},
				Summary: "PostgreSQL object-relational SQL database (supported version)",
				Version: "208",
			},
			EffectiveChannel: "latest/stable",
			Error:            (*transport.APIError)(nil),
			Name:             "postgresql",
			Result:           "download",
		},
	}
	s.client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return(resp, nil)
	return url
}
