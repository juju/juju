// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
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

type sanitizeCharmOriginSuite struct{}

var _ = gc.Suite(&sanitizeCharmOriginSuite{})

func (s *sanitizeCharmOriginSuite) TestSanitize(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "all",
			Series:       "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Series:       "focal",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Series:       "focal",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Series:       "win8",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Series:       "focal",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Series:       "win8",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithEmptyValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Series:       "focal",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "all",
			Series:       "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesAlt(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "all",
			Series:       "focal",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Series:       "focal",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesOSVersusSeries(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "ubuntu",
			Series:       "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Series:       "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Series:       "",
		},
	})
}
