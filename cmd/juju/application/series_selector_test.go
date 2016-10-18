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

		ltsSeries      string
		expectedSeries string
		message        string
		err            string
	}{{
		title: "use charm default, e.g. juju deploy ubuntu",
		seriesSelector: seriesSelector{
			supportedSeries: []string{"trusty", "precise"},
			conf:            defaultSeries{"wily", true},
		},
		ltsSeries:      "precise",
		expectedSeries: "trusty",
		message:        "with the default charm metadata series %q",
	}, {
		title: "use supported requested, e.g. juju deploy ubuntu --series trusty",
		seriesSelector: seriesSelector{
			seriesFlag:      "trusty",
			supportedSeries: []string{"trusty"},
			conf:            defaultSeries{},
		},
		expectedSeries: "trusty",
		message:        "with the user specified series %q",
	}, {
		title: "unsupported requested, e.g. juju deploy ubuntu --series quantal",
		seriesSelector: seriesSelector{
			seriesFlag:      "quantal",
			supportedSeries: []string{"trusty", "precise"},
			conf:            defaultSeries{},
		},
		err: `series "quantal" not supported by charm, supported series are: trusty,precise`,
	}, {
		title: "charm without series specified or requested, with --force",
		seriesSelector: seriesSelector{
			force: true,
			conf:  defaultSeries{},
		},
		ltsSeries:      "quantal",
		expectedSeries: "quantal",
		message:        "with the latest LTS series %q",
	}, {
		title: "charm without series specified or requested, without --force",
		seriesSelector: seriesSelector{
			conf: defaultSeries{},
		},
		ltsSeries:      "quantal",
		expectedSeries: "quantal",
		err:            `series "quantal" not supported by charm, supported series are: <none defined>`,
	}, {
		title: "charm without series specified, series requested, without --force",
		seriesSelector: seriesSelector{
			seriesFlag: "xenial",
			conf:       defaultSeries{},
		},
		ltsSeries:      "quantal",
		expectedSeries: "quantal",
		err:            `series "xenial" not supported by charm, supported series are: <none defined>`,
	}, {
		title: "charm without series specified, series requested, with --force",
		seriesSelector: seriesSelector{
			seriesFlag: "xenial",
			conf:       defaultSeries{},
			force:      true,
		},
		ltsSeries:      "quantal",
		expectedSeries: "xenial",
		message:        "with the user specified series %q",
	}, {
		title: "no requested series, default to model series if supported",
		seriesSelector: seriesSelector{
			conf:            defaultSeries{"xenial", true},
			supportedSeries: []string{"precise", "xenial"},
		},
		expectedSeries: "xenial",
		message:        "with the configured model default series %q",
	}, {
		title: "juju deploy --force --series=wily for unsupported series",
		seriesSelector: seriesSelector{
			seriesFlag:      "wily",
			supportedSeries: []string{"trusty"},
			force:           true,
			conf:            defaultSeries{},
		},
		expectedSeries: "wily",
		message:        "with the user specified series %q",
	}, {
		title: "juju deploy --series=precise for non-default but supported series",
		seriesSelector: seriesSelector{
			seriesFlag:      "precise",
			supportedSeries: []string{"trusty", "precise"},
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
		message:        "with the user specified series %q",
	}, {
		title: "juju deploy precise/ubuntu for non-default but supported series",
		seriesSelector: seriesSelector{
			charmURLSeries:  "precise",
			supportedSeries: []string{"trusty", "precise"},
			conf:            defaultSeries{},
		},
		expectedSeries: "precise",
		message:        "with the user specified series %q",
	}, {
		title: "juju deploy precise/ubuntu --series=wily for non-default but supported series",
		seriesSelector: seriesSelector{
			charmURLSeries:  "precise",
			seriesFlag:      "wily",
			supportedSeries: []string{"trusty", "wily"},
			conf:            defaultSeries{},
		},
		expectedSeries: "wily",
		message:        "with the user specified series %q",
	}, {
		title: "juju deploy precise/ubuntu --series=quantal for usupported series",
		seriesSelector: seriesSelector{
			charmURLSeries:  "precise",
			seriesFlag:      "quantal",
			supportedSeries: []string{"trusty", "precise"},
			conf:            defaultSeries{},
		},
		err: `series "quantal" not supported by charm, supported series are: trusty,precise`,
	}}

	for i, test := range deploySeriesTests {

		func() {
			c.Logf("test %d [%s]", i, test.title)
			if test.ltsSeries != "" {
				previous := series.SetLatestLtsForTesting(test.ltsSeries)
				defer series.SetLatestLtsForTesting(previous)
			}
			series, err := test.seriesSelector.charmSeries()
			if test.err != "" {
				c.Check(err, gc.ErrorMatches, test.err)
				return
			}
			c.Check(err, jc.ErrorIsNil)
			c.Check(series, gc.Equals, test.expectedSeries)
		}()
	}
}

type defaultSeries struct {
	series   string
	explicit bool
}

func (d defaultSeries) DefaultSeries() (string, bool) {
	return d.series, d.explicit
}
