// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/hash"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/resources/mocks"
	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
)

var _ = gc.Suite(&CharmHubClientSuite{})

type CharmHubClientSuite struct {
	client *mocks.MockCharmHub
}

func (s *CharmHubClientSuite) TestResolveResources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.client = mocks.NewMockCharmHub(ctrl)
	s.expectRefresh()

	fp, err := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.newClient().ResolveResources([]charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        0,
	}, {
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    2,
		Fingerprint: fp,
		Size:        0,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp,
		Size:        0,
	}, {
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    2,
		Fingerprint: fp,
		Size:        0,
	}})
}

func (s *CharmHubClientSuite) TestResolveResourcesFromStore(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.client = mocks.NewMockCharmHub(ctrl)
	s.expectRefresh()
	s.expectRefreshWithRevision(1)

	fp, err := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.newClient().ResolveResources([]charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp,
		Size:        0,
	}})
}

func (s *CharmHubClientSuite) TestResolveResourcesNoMatchingRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.client = mocks.NewMockCharmHub(ctrl)
	s.expectRefresh()
	s.expectRefreshWithRevision(99)

	_, err := s.newClient().ResolveResources([]charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}})
	c.Assert(err, gc.ErrorMatches, `charm resource "wal-e" at revision 1 not found`)
}

func (s *CharmHubClientSuite) TestResolveResourcesUpload(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = mocks.NewMockCharmHub(ctrl)
	s.expectRefresh()

	result, err := s.newClient().ResolveResources([]charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   1,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   1,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}})
}

func (s *CharmHubClientSuite) newClient() NewCharmRepository {
	curl := charm.MustParseURL("ubuntu")
	channel, _ := corecharm.ParseChannel("stable")
	c := &charmHubClient{
		client: s.client,
	}
	c.resourceClient = resourceClient{
		client: c,
		id: CharmID{
			URL: curl,
			Origin: corecharm.Origin{
				ID:      "meshuggah",
				Source:  corecharm.CharmHub,
				Channel: &channel,
				Platform: corecharm.Platform{
					OS:           "ubuntu",
					Series:       "focal",
					Architecture: "amd64",
				},
			},
		},
	}
	return c
}

func (s *CharmHubClientSuite) expectRefresh() {
	s.expectRefreshWithRevision(0)
}

func (s *CharmHubClientSuite) expectRefreshWithRevision(rev int) {
	resp := []transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{
				CreatedAt: time.Date(2020, 7, 7, 9, 39, 44, 132000000, time.UTC),
				Download:  transport.Download{HashSHA256: "c97e1efc5367d2fdcfdf29f4a2243b13765cc9cbdfad19627a29ac903c01ae63", Size: 5487460, URL: "https://api.staging.charmhub.io/api/v1/charms/download/jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD_208.charm"},
				ID:        "jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD",
				Name:      "ubuntu",
				Resources: []transport.ResourceRevision{
					{
						Download:    transport.ResourceDownload{HashSHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", HashSHA3384: "0c63a75b845e4f7d01107d852e4c2485c51a50aaaa94fc61995e71bbee983a2ac3713831264adb47fb6bd1e058d5f004", HashSHA384: "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b", HashSHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e", Size: 0, URL: "https://api.staging.charmhub.io/api/v1/resources/download/charm_jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD.wal-e_0"},
						Name:        "wal-e",
						Revision:    rev,
						Type:        "file",
						Filename:    "wal-e.snap",
						Description: "WAL-E Snap Package",
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
