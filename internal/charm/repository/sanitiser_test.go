// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type sanitiseCharmOriginSuite struct{}

func TestSanitiseCharmOriginSuite(t *stdtesting.T) {
	tc.Run(t, &sanitiseCharmOriginSuite{})
}
func (s *sanitiseCharmOriginSuite) TestSanitise(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithValues(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "windows",
			Channel:      "win8",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithEmptyValues(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValues(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValuesAlt(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseWithRequestedEmptyValuesOSVersusChannel(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "",
			OS:           "ubuntu",
			Channel:      "",
		},
	})
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannel(c *tc.C) {
	ch := corecharm.MustParseChannel("stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*got.Channel, tc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannelNop(c *tc.C) {
	ch := corecharm.MustParseChannel("latest/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*got.Channel, tc.Equals, corecharm.MustParseChannel("latest/stable"))
}

func (s *sanitiseCharmOriginSuite) TestSanitiseChannelNopOtherTrack(c *tc.C) {
	ch := corecharm.MustParseChannel("5/stable")
	received := corecharm.Origin{
		Channel: &ch,
	}
	got, err := sanitiseCharmOrigin(received, corecharm.Origin{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*got.Channel, tc.Equals, corecharm.MustParseChannel("5/stable"))
}
