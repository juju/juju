// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type sanitiseCharmOriginSuite struct{}

var _ = gc.Suite(&sanitiseCharmOriginSuite{})

func (s *sanitiseCharmOriginSuite) TestSanitise(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithValues(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Channel:      "win8",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithEmptyValues(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValues(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValuesAlt(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValuesOSVersusChannel(c *gc.C) {
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
	got, err := sanitiseCharmOrigin(received, requested)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannel(c *gc.C) {
	ch := corecharm.MustParseChannel("stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannelNop(c *gc.C) {
	ch := corecharm.MustParseChannel("latest/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannelNopOtherTrack(c *gc.C) {
	ch := corecharm.MustParseChannel("5/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*got.Channel, gc.Equals, corecharm.MustParseChannel("5/stable"))
}
