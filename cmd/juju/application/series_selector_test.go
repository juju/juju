// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
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
	}{{
		// Simple selectors first, no supported series.

		title: "juju deploy simple   # no default series, no supported series",
		seriesSelector: seriesSelector{
			conf: defaultSeries{},
		},
		err: "series not specified and charm does not define any",
	}, {
		title: "juju deploy simple   # default series set, no supported series",
		seriesSelector: seriesSelector{
			conf: defaultSeries{"wily", true},
		},
		expectedSeries: "wily",
	}, {
		title: "juju deploy simple --series=precise   # default series set, no supported series",
		seriesSelector: seriesSelector{
			seriesFlag: "precise",
			conf:       defaultSeries{"wily", true},
		},
		expectedSeries: "precise",
	}, {
		title: "juju deploy trusty/simple   # charm series set, default series set, no supported series",
		seriesSelector: seriesSelector{
			charmURLSeries: "trusty",
			conf:           defaultSeries{"wily", true},
		},
		expectedSeries: "trusty",
	}, {
		title: "juju deploy trusty/simple --series=precise   # series specified, charm series set, default series set, no supported series",
		seriesSelector: seriesSelector{
			seriesFlag:     "precise",
			charmURLSeries: "trusty",
			conf:           defaultSeries{"wily", true},
		},
		expectedSeries: "precise",
	}, {
		title: "juju deploy simple --force   # no default series, no supported series, use LTS (xenial)",
		seriesSelector: seriesSelector{
			force: true,
			conf:  defaultSeries{},
		},
		expectedSeries: "xenial",
	}, {
		// Now charms with supported series.

		title: "juju deploy multiseries   # use charm default, nothing specified, no default series",
		seriesSelector: seriesSelector{
			supportedSeries: []string{"utopic", "vivid"},
			conf:            defaultSeries{},
		},
		expectedSeries: "utopic",
	}, {
		title: "juju deploy multiseries   # use charm defaults used if default series doesn't match, nothing specified",
		seriesSelector: seriesSelector{
			supportedSeries: []string{"utopic", "vivid"},
			conf:            defaultSeries{"wily", true},
		},
		expectedSeries: "utopic",
	}, {
		title: "juju deploy multiseries   # use model series defaults if supported by charm",
		seriesSelector: seriesSelector{
			supportedSeries: []string{"utopic", "vivid", "wily"},
			conf:            defaultSeries{"wily", true},
		},
		expectedSeries: "wily",
	}, {
		title: "juju deploy multiseries --series=precise   # use supported requested",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			supportedSeries: []string{"utopic", "vivid", "precise"},
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
	}, {
		title: "juju deploy multiseries --series=precise   # unsupported requested",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			supportedSeries: []string{"utopic", "vivid"},
			conf:            defaultSeries{},
		},
		err: `series "precise" not supported by charm, supported series are: utopic,vivid`,
	}, {
		title: "juju deploy multiseries --series=precise --force   # unsupported forced",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			supportedSeries: []string{"utopic", "vivid"},
			force:           true,
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
	}, {
		title: "juju deploy trusty/multiseries  # non-default but supported series",
		seriesSelector: seriesSelector{
			charmURLSeries:  "trusty",
			supportedSeries: []string{"utopic", "vivid", "trusty"},
			conf:            defaultSeries{},
		},
		expectedSeries: "trusty",
	}, {
		title: "juju deploy trusty/multiseries --series=precise  # non-default but supported series",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			charmURLSeries:  "trusty",
			supportedSeries: []string{"utopic", "vivid", "trusty", "precise"},
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
	}, {
		title: "juju deploy trusty/multiseries --series=precise  # unsupported series",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			charmURLSeries:  "trusty",
			supportedSeries: []string{"trusty", "utopic", "vivid"},
			conf:            defaultSeries{},
		},
		err: `series "precise" not supported by charm, supported series are: trusty,utopic,vivid`,
	}, {
		title: "juju deploy trusty/multiseries --series=precise --force  # unsupported series forced",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			charmURLSeries:  "trusty",
			supportedSeries: []string{"trusty", "utopic", "vivid"},
			force:           true,
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
	}}

	// Use xenial for LTS for all calls.
	previous := series.SetLatestLtsForTesting("xenial")
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
