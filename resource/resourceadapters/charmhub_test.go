// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters_test

import (
	"bytes"
	"io/ioutil"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/resource/repositories"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/resource/resourceadapters/mocks"
	"github.com/juju/juju/state"
)

type CharmHubSuite struct {
	client *mocks.MockCharmHub
}

var _ = gc.Suite(&CharmHubSuite{})

func (s *CharmHubSuite) TestGetResource(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = mocks.NewMockCharmHub(ctrl)
	s.expectRefresh()
	s.expectDownloadResource()

	cl := s.newCharmHubClient()
	curl, _ := charm.ParseURL("ch:postgresql")
	result, err := cl.GetResource(repositories.ResourceRequest{
		CharmID: repositories.CharmID{
			URL: curl,
			Origin: state.CharmOrigin{
				Channel: &state.Channel{Risk: "stable"},
				Platform: &state.Platform{
					Architecture: "amd64",
					OS:           "ubuntu",
					Series:       "focal",
				},
			},
		},
		Name:     "wal-e",
		Revision: 0,
	})
	c.Assert(err, jc.ErrorIsNil)

	fp, _ := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	c.Assert(result.Resource, gc.DeepEquals, charmresource.Resource{
		Meta: charmresource.Meta{
			Name: "wal-e",
			Type: 1,
		},
		Origin:      2,
		Revision:    0,
		Fingerprint: fp,
		Size:        0,
	})
}

func (s *CharmHubSuite) newCharmHubClient() *resourceadapters.CharmHubClient {
	return resourceadapters.NewCharmHubClientForTest(s.client, &noopLogger{})
}

func (s *CharmHubSuite) expectDownloadResource() {
	s.client.EXPECT().DownloadResource(gomock.Any(), gomock.Any()).Return(ioutil.NopCloser(bytes.NewBuffer([]byte{})), nil)
}

func (s *CharmHubSuite) expectRefresh() {
	resp := []transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{
				Download: transport.Download{
					HashSHA256: "c97e1efc5367d2fdcfdf29f4a2243b13765cc9cbdfad19627a29ac903c01ae63",
					Size:       5487460,
					URL:        "https://api.staging.charmhub.io/api/v1/charms/download/jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD_208.charm"},
				ID:   "jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD",
				Name: "ubuntu",
				Resources: []transport.ResourceRevision{
					{
						Download: transport.ResourceDownload{
							HashSHA256:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
							HashSHA3384: "0c63a75b845e4f7d01107d852e4c2485c51a50aaaa94fc61995e71bbee983a2ac3713831264adb47fb6bd1e058d5f004",
							HashSHA384:  "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
							HashSHA512:  "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
							Size:        0,
							URL:         "https://api.staging.charmhub.io/api/v1/resources/download/charm_jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD.wal-e_0"},
						Name:     "wal-e",
						Revision: 0,
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
}

type noopLogger struct{}

func (noopLogger) Tracef(string, ...interface{}) {}
