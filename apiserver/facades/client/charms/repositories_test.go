// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/arch"
)

type charmStoreResolversSuite struct {
	repo CSRepository
}

var _ = gc.Suite(&charmStoreResolversSuite{})

func (s *charmStoreResolversSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.repo = mocks.NewMockCSRepository(ctrl)
	return ctrl
}

func getCharmHubCharmResponse() ([]transport.InfoChannelMap, transport.InfoChannelMap) {
	return []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name: "stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 18,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "candidate",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "candidate",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "edge",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "edge",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "second/stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "second",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 13,
				Version:  "1.0.3",
			},
		}}, transport.InfoChannelMap{
			Channel: transport.Channel{
				Name: "stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}
}

func alternativeDefaultChannelMap() transport.InfoChannelMap {
	return transport.InfoChannelMap{
		Channel: transport.Channel{
			Name: "other",
			Platform: transport.Platform{
				Architecture: arch.DefaultArchitecture,
				OS:           "ubuntu",
				Series:       "bionic",
			},
			Risk:  "edge",
			Track: "1.0",
		},
		Revision: transport.InfoRevision{
			Platforms: []transport.Platform{{
				Architecture: arch.DefaultArchitecture,
				OS:           "ubuntu",
				Series:       "bionic",
			}},
			Revision: 17,
			Version:  "1.0.3",
		},
	}
}

var entityMeta = `
name: myname
version: 1.0.3
subordinate: false
summary: A charm or bundle.
description: |
  This will install and setup services optimized to run in the cloud.
  By default it will place Ngnix configured to scale horizontally
  with Nginx's reverse proxy.
series: [bionic, xenial]
provides:
  source:
    interface: dummy-token
requires:
  sink:
    interface: dummy-token
`

func getCharmHubBundleResponse() ([]transport.InfoChannelMap, transport.InfoChannelMap) {
	return []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name: "stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 18,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "candidate",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "candidate",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "edge",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "edge",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				BundleYAML: entityBundle,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "second/stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "second",
			},
			Revision: transport.InfoRevision{
				BundleYAML: entityBundle,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 13,
				Version:  "1.0.3",
			},
		}}, transport.InfoChannelMap{
			Channel: transport.Channel{
				Name: "stable",
				Platform: transport.Platform{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.InfoRevision{
				BundleYAML: entityBundle,
				Platforms: []transport.Platform{{
					Architecture: arch.DefaultArchitecture,
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}
}

const entityBundle = `
series: bionic
applications:
    wordpress:
        charm: wordpress
        num_units: 1
`

type sanitizeCharmOriginSuite struct{}

var _ = gc.Suite(&sanitizeCharmOriginSuite{})

func (s *sanitizeCharmOriginSuite) TestSanitize(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "all",
		OS:           "all",
		Series:       "all",
	}
	requested := params.CharmOrigin{
		Architecture: arch.DefaultArchitecture,
		OS:           "Ubuntu",
		Series:       "focal",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: arch.DefaultArchitecture,
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithValues(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "arm64",
		OS:           "windows",
		Series:       "win8",
	}
	requested := params.CharmOrigin{
		Architecture: arch.DefaultArchitecture,
		OS:           "Ubuntu",
		Series:       "focal",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: "arm64",
		OS:           "windows",
		Series:       "win8",
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithEmptyValues(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	}
	requested := params.CharmOrigin{
		Architecture: arch.DefaultArchitecture,
		OS:           "Ubuntu",
		Series:       "focal",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValues(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "all",
		OS:           "all",
		Series:       "all",
	}
	requested := params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesAlt(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "all",
		OS:           "all",
		Series:       "focal",
	}
	requested := params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: "",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesOSVersusSeries(c *gc.C) {
	received := params.CharmOrigin{
		Architecture: "all",
		OS:           "ubuntu",
		Series:       "all",
	}
	requested := params.CharmOrigin{
		Architecture: "",
		OS:           "",
		Series:       "",
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, params.CharmOrigin{
		Architecture: "",
		OS:           "ubuntu",
		Series:       "",
	})
}
