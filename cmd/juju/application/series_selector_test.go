// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/collections/set"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
			// Simple selectors first, no supported series.

			title: "juju deploy simple   # no default series, no supported series",
			seriesSelector: seriesSelector{
				conf: defaultSeries{},
			},
			err: "series not specified and charm does not define any",
		}, {
			title: "juju deploy simple   # default series set, no supported series",
			seriesSelector: seriesSelector{
				conf:                defaultSeries{"bionic", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple with old series  # default series set, no supported series",
			seriesSelector: seriesSelector{
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: wily not supported",
		},
		{
			title: "juju deploy simple --series=precise   # default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "precise",
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: precise not supported",
		}, {
			title: "juju deploy simple --series=bionic   # default series set, no supported series, no supported juju series",
			seriesSelector: seriesSelector{
				seriesFlag: "bionic",
				conf:       defaultSeries{"wily", true},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy simple --series=bionic   # default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy trusty/simple   # charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "trusty",
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: trusty not supported",
		},
		{
			title: "juju deploy bionic/simple   # charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "bionic",
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy cosmic/simple --series=bionic   # series specified, charm series set, default series set, no supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				charmURLSeries:      "cosmic",
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple --force   # no default series, no supported series, use LTS (bionic)",
			seriesSelector: seriesSelector{
				force: true,
				conf:  defaultSeries{},
			},
			expectedSeries: "bionic",
		},

		// Now charms with supported series.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"precise", "bionic", "cosmic"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid serie  # use charm default, nothing specified, no default series",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"precise"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series not specified and charm does not define any",
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default series doesn't match, nothing specified",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic"},
				conf:                defaultSeries{"wily", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic", "disco"},
				conf:                defaultSeries{"disco", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic", "disco"),
			},
			expectedSeries: "disco",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic", "disco"},
				conf:                defaultSeries{"disco", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: disco not supported",
		},
		{
			title: "juju deploy multiseries with force  # use model series defaults if supported by charm, force",
			seriesSelector: seriesSelector{
				supportedSeries:     []string{"bionic", "cosmic", "disco"},
				conf:                defaultSeries{"disco", true},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
				force:               true,
			},
			expectedSeries: "disco",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"cosmic", "bionic"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # unsupported requested",
			seriesSelector: seriesSelector{
				seriesFlag:          "bionic",
				supportedSeries:     []string{"utopic", "vivid"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series "bionic" not supported by charm, supported series are: utopic,vivid`,
		},
		{
			title: "juju deploy multiseries --series=bionic --force   # unsupported forced",
			seriesSelector: seriesSelector{
				seriesFlag:      "bionic",
				supportedSeries: []string{"utopic", "vivid"},
				force:           true,
				conf:            defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic"},
				conf:                defaultSeries{},
				supportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			seriesSelector: seriesSelector{
				charmURLSeries:  "bionic",
				supportedSeries: []string{"utopic", "vivid", "bionic"},
				conf:            defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # non-default but supported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "cosmic",
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"utopic", "vivid", "bionic", "cosmic"},
				conf:                defaultSeries{},
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
				conf:            defaultSeries{},
			},
			err: `series "cosmic" not supported by charm, supported series are: bionic,utopic,vivid`,
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			seriesSelector: seriesSelector{
				seriesFlag:          "cosmic",
				charmURLSeries:      "bionic",
				supportedSeries:     []string{"bionic", "utopic", "vivid", "cosmic"},
				conf:                defaultSeries{},
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
				conf:            defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
	}

	// Use bionic for LTS for all calls.
	previous := series.SetLatestLtsForTesting("bionic")
	defer series.SetLatestLtsForTesting(previous)

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

type defaultSeries struct {
	series   string
	explicit bool
}

func (d defaultSeries) DefaultSeries() (string, bool) {
	return d.series, d.explicit
}
