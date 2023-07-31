// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
)

type SeriesSelectorSuite struct{}

var _ = gc.Suite(&SeriesSelectorSuite{})

func (s *SeriesSelectorSuite) TestCharmSeries(c *gc.C) {
	deploySeriesTests := []struct {
		title string

		seriesSelector

		expectedSeries string
		err            string
	}{
		{
			// Simple selectors first, no supported corebase.

			title: "juju deploy simple   # no default series, no supported series",
			seriesSelector: seriesSelector{
				conf: defaultBase{},
			},
			err: "series not specified and charm does not define any",
		}, {
			title: "juju deploy simple   # default series set, no supported series",
			seriesSelector: seriesSelector{
				conf:                defaultBase{"ubuntu@18.04", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple with old series  # default series set, no supported series",
			seriesSelector: seriesSelector{
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: wily not supported",
		},
		{
			title: "juju deploy simple --series=precise   # default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "precise",
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: precise not supported",
		}, {
			title: "juju deploy simple --series=bionic   # default series set, no supported series, no supported juju series",
			seriesSelector: seriesSelector{
				seriesFlag: "bionic",
				conf:       defaultBase{"ubuntu@15.10", true},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy simple --series=bionic   # default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy trusty/simple   # charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "trusty",
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: trusty not supported",
		},
		{
			title: "juju deploy bionic/simple   # charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "bionic",
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy cosmic/simple --series=bionic   # series specified, charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				charmURLSeries:      "cosmic",
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple --force   # no default series, no supported series, use LTS (jammy)",
			seriesSelector: seriesSelector{
				force: true,
				conf:  defaultBase{},
			},
			expectedSeries: "jammy",
		},

		// Now charms with supported corebase.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"precise", "bionic", "cosmic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid serie  # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"precise"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `the charm defined series "precise" not supported`,
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default series doesn't match, nothing specified",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic"},
				conf:                defaultBase{"ubuntu@15.10", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series "wily" is not supported, supported series are: bionic,cosmic`,
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic", "disco"},
				conf:                defaultBase{"ubuntu@19.04", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic", "disco"),
			},
			expectedSeries: "disco",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic", "disco"},
				conf:                defaultBase{"ubuntu@19.04", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: disco not supported",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"cosmic", "bionic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # unsupported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"utopic", "vivid"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series: bionic`,
		},
		{
			title: "juju deploy multiseries --series=bionic --force   # unsupported forced",
			seriesSelector: seriesSelector{
				seriesFlag:      "bionic",
				supportedSeries: []string{"utopic", "vivid"},
				force:           true,
				conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:  "bionic",
				supportedSeries: []string{"utopic", "vivid", "bionic"},
				conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # non-default but supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "cosmic",
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic", "cosmic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			seriesSelector: seriesSelector{
				seriesFlag:      "cosmic",
				charmURLSeries:  "bionic",
				supportedSeries: []string{"bionic", "utopic", "vivid"},
				conf:            defaultBase{},
			},
			err: `series: cosmic`,
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "cosmic",
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"bionic", "utopic", "vivid", "cosmic"},
				conf:                defaultBase{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=precise --force  # unsupported series forced",
			seriesSelector: seriesSelector{
				seriesFlag:      "precise",
				charmURLSeries:  "bionic",
				supportedSeries: []string{"bionic", "utopic", "vivid"},
				force:           true,
				conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
	}

	// Use bionic for LTS for all calls.
	previous := corebase.SetLatestLtsForTesting("bionic")
	defer corebase.SetLatestLtsForTesting(previous)

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		series, err := test.seriesSelector.charmSeries()
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(series, gc.Equals, test.expectedSeries)
		}
	}
}

type defaultBase struct {
	base     string
	explicit bool
}

func (d defaultBase) DefaultBase() (string, bool) {
	return d.base, d.explicit
}
