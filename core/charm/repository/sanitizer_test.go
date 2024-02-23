// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type sanitizeCharmOriginSuite struct{}

var _ = gc.Suite(&sanitizeCharmOriginSuite{})

func (s *sanitizeCharmOriginSuite) TestSanitize(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "all",
			Channel:      "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Channel:      "20.04",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Channel:      "win8",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Channel:      "20.04",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Channel:      "win8",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithEmptyValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "Ubuntu",
			Channel:      "20.04",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValues(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "all",
			Channel:      "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesAlt(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeWithRequestedEmptyValuesOSVersusChannel(c *gc.C) {
	received := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "all",
			OS:           "ubuntu",
			Channel:      "all",
		},
	}
	requested := corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	}
	got, err := sanitizeCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "",
		},
	})
}

func (s *sanitizeCharmOriginSuite) TestSanitizeChannel(c *gc.C) {
	ch := corecharm.MustParseChannel("stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitizeCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitizeCharmOriginSuite) TestSanitizeChannelNop(c *gc.C) {
	ch := corecharm.MustParseChannel("latest/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitizeCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitizeCharmOriginSuite) TestSanitizeChannelNopOtherTrack(c *gc.C) {
	ch := corecharm.MustParseChannel("5/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitizeCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("5/stable"))
}
