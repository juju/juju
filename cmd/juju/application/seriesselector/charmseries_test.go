// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package seriesselector_test

import (
	"testing"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/seriesselector"
	"github.com/juju/juju/core/series"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type Suite struct{}

var _ = gc.Suite(&Suite{})

func (s *Suite) TestCharmSeries(c *gc.C) {
	deploySeriesTests := []struct {
		title    string
		args     seriesselector.CharmSeriesArgs
		expected string
		err      string
	}{
		{
			// Simple selectors first, no supported series.

			title: "juju deploy simple   # no default series, no supported series",
			args: seriesselector.CharmSeriesArgs{
				Config: defaultSeries{},
			},
			err: "series not specified and charm does not define any",
		}, {
			title: "juju deploy simple   # default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				Config:              defaultSeries{"bionic", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy simple with old series  # default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: wily not supported",
		},
		{
			title: "juju deploy simple --series=precise   # default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "precise",
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: precise not supported",
		}, {
			title: "juju deploy simple --series=bionic   # default series set, no supported series, no supported juju series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag: "bionic",
				Config:     defaultSeries{"wily", true},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy simple --series=bionic   # default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "bionic",
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy trusty/simple   # charm series set, default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				CharmURLSeries:      "trusty",
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: trusty not supported",
		},
		{
			title: "juju deploy bionic/simple   # charm series set, default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				CharmURLSeries:      "bionic",
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy cosmic/simple --series=bionic   # series specified, charm series set, default series set, no supported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "bionic",
				CharmURLSeries:      "cosmic",
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy simple --force   # no default series, no supported series, use LTS (focal)",
			args: seriesselector.CharmSeriesArgs{
				Force:  true,
				Config: defaultSeries{},
			},
			expected: "focal",
		},

		// Now charms with supported series.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default series",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"bionic", "cosmic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default series",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"precise", "bionic", "cosmic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid serie  # use charm default, nothing specified, no default series",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"precise"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series not specified and charm does not define any",
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default series doesn't match, nothing specified",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"bionic", "cosmic"},
				Config:              defaultSeries{"wily", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"bionic", "cosmic", "disco"},
				Config:              defaultSeries{"disco", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic", "disco"),
			},
			expected: "disco",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"bionic", "cosmic", "disco"},
				Config:              defaultSeries{"disco", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: disco not supported",
		},
		{
			title: "juju deploy multiseries with force  # use model series defaults if supported by charm, force",
			args: seriesselector.CharmSeriesArgs{
				SupportedSeries:     []string{"bionic", "cosmic", "disco"},
				Config:              defaultSeries{"disco", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
				Force:               true,
			},
			expected: "disco",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"cosmic", "bionic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # unsupported requested",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"utopic", "vivid"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series "bionic" not supported by charm, supported series are: utopic,vivid`,
		},
		{
			title: "juju deploy multiseries --series=bionic --force   # unsupported forced",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:      "bionic",
				SupportedSeries: []string{"utopic", "vivid"},
				Force:           true,
				Config:          defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			args: seriesselector.CharmSeriesArgs{
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "bionic",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			args: seriesselector.CharmSeriesArgs{
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"utopic", "vivid", "bionic"},
				Config:          defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # non-default but supported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "cosmic",
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic", "cosmic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:      "cosmic",
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"bionic", "utopic", "vivid"},
				Config:          defaultSeries{},
			},
			err: `series "cosmic" not supported by charm, supported series are: bionic,utopic,vivid`,
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:          "cosmic",
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"bionic", "utopic", "vivid", "cosmic"},
				Config:              defaultSeries{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expected: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=precise --force  # unsupported series forced",
			args: seriesselector.CharmSeriesArgs{
				SeriesFlag:      "precise",
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"bionic", "utopic", "vivid"},
				Force:           true,
				Config:          defaultSeries{},
			},
			err: "expected supported juju series to exist",
		},
	}

	// Use bionic for LTS for all calls.
	previous := series.SetLatestLtsForTesting("bionic")
	defer series.SetLatestLtsForTesting(previous)

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.args.Logger = noopLogger{}
		series, err := seriesselector.CharmSeries(test.args)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(series, gc.Equals, test.expected)
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

type noopLogger struct{}

func (_ noopLogger) Infof(format string, params ...interface{}) {}
