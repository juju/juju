// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/series"
)

type SeriesSelectorSuite struct{}

var _ = gc.Suite(&SeriesSelectorSuite{})

func (s *SeriesSelectorSuite) TestCharmSeries(c *gc.C) {
	deploySeriesTests := []struct {
		title string

		SeriesSelector

		expectedSeries string
		err            string
	}{
		{
			// Simple selectors first, no supported series.

			title: "juju deploy simple   # no default series, no supported series",
			SeriesSelector: SeriesSelector{
				Conf: defaultBase{},
			},
			err: "series not specified and charm does not define any",
		}, {
			title: "juju deploy simple   # default series set, no supported series",
			SeriesSelector: SeriesSelector{
				Conf:                defaultBase{"ubuntu@18.04", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple with old series  # default series set, no supported series",
			SeriesSelector: SeriesSelector{
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: wily not supported",
		},
		{
			title: "juju deploy simple --series=precise   # default series set, no supported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "precise",
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: precise not supported",
		}, {
			title: "juju deploy simple --series=bionic   # default series set, no supported series, no supported juju series",
			SeriesSelector: SeriesSelector{
				SeriesFlag: "bionic",
				Conf:       defaultBase{"ubuntu@15.10", true},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy simple --series=bionic   # default series set, no supported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "bionic",
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy trusty/simple   # charm series set, default series set, no supported series",
			SeriesSelector: SeriesSelector{
				CharmURLSeries:      "trusty",
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: trusty not supported",
		},
		{
			title: "juju deploy bionic/simple   # charm series set, default series set, no supported series",
			SeriesSelector: SeriesSelector{
				CharmURLSeries:      "bionic",
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy cosmic/simple --series=bionic   # series specified, charm series set, default series set, no supported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "bionic",
				CharmURLSeries:      "cosmic",
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy simple --Force   # no default series, no supported series, use LTS (jammy)",
			SeriesSelector: SeriesSelector{
				Force: true,
				Conf:  defaultBase{},
			},
			expectedSeries: "jammy",
		},

		// Now charms with supported series.

		{
			title: "juju deploy multiseries   # use charm default, nothing specified, no default series",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"bionic", "cosmic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid series  # use charm default, nothing specified, no default series",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"precise", "bionic", "cosmic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries with invalid serie  # use charm default, nothing specified, no default series",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"precise"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `the charm defined series "precise" not supported`,
		},
		{
			title: "juju deploy multiseries   # use charm defaults used if default series doesn't match, nothing specified",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"bionic", "cosmic"},
				Conf:                defaultBase{"ubuntu@15.10", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series "wily" is not supported, supported series are: bionic,cosmic`,
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"bionic", "cosmic", "disco"},
				Conf:                defaultBase{"ubuntu@19.04", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic", "disco"),
			},
			expectedSeries: "disco",
		},
		{
			title: "juju deploy multiseries   # use model series defaults if supported by charm",
			SeriesSelector: SeriesSelector{
				SupportedSeries:     []string{"bionic", "cosmic", "disco"},
				Conf:                defaultBase{"ubuntu@19.04", true},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: "series: disco not supported",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # use supported requested",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"cosmic", "bionic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy multiseries --series=bionic   # unsupported requested",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "bionic",
				SupportedSeries:     []string{"utopic", "vivid"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			err: `series: bionic`,
		},
		{
			title: "juju deploy multiseries --series=bionic --Force   # unsupported forced",
			SeriesSelector: SeriesSelector{
				SeriesFlag:      "bionic",
				SupportedSeries: []string{"utopic", "vivid"},
				Force:           true,
				Conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			SeriesSelector: SeriesSelector{
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "bionic",
		},
		{
			title: "juju deploy bionic/multiseries  # non-default but supported series",
			SeriesSelector: SeriesSelector{
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"utopic", "vivid", "bionic"},
				Conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # non-default but supported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "cosmic",
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"utopic", "vivid", "bionic", "cosmic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:      "cosmic",
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"bionic", "utopic", "vivid"},
				Conf:            defaultBase{},
			},
			err: `series: cosmic`,
		},
		{
			title: "juju deploy bionic/multiseries --series=cosmic  # unsupported series",
			SeriesSelector: SeriesSelector{
				SeriesFlag:          "cosmic",
				CharmURLSeries:      "bionic",
				SupportedSeries:     []string{"bionic", "utopic", "vivid", "cosmic"},
				Conf:                defaultBase{},
				SupportedJujuSeries: set.NewStrings("bionic", "cosmic"),
			},
			expectedSeries: "cosmic",
		},
		{
			title: "juju deploy bionic/multiseries --series=precise --Force  # unsupported series forced",
			SeriesSelector: SeriesSelector{
				SeriesFlag:      "precise",
				CharmURLSeries:  "bionic",
				SupportedSeries: []string{"bionic", "utopic", "vivid"},
				Force:           true,
				Conf:            defaultBase{},
			},
			err: "expected supported juju series to exist",
		},
	}

	// Use bionic for LTS for all calls.
	previous := series.SetLatestLtsForTesting("bionic")
	defer series.SetLatestLtsForTesting(previous)

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.SeriesSelector.Logger = &noOpLogger{}
		series, err := test.SeriesSelector.CharmSeries()
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(series, gc.Equals, test.expectedSeries)
		}
	}
}

func (s *SeriesSelectorSuite) TestValidate(c *gc.C) {
	deploySeriesTests := []struct {
		title    string
		selector SeriesSelector
		err      string
	}{
		{
			title: "should fail when image-id constraint is used and no base is explicitly set",
			selector: SeriesSelector{
				Conf: defaultBase{
					explicit: false,
				},
				UsingImageID: true,
			},
			err: "base must be explicitly provided when image-id constraint is used",
		},
		{
			title: "should return no errors when using image-id and series flag",
			selector: SeriesSelector{
				Conf: defaultBase{
					explicit: false,
				},
				SeriesFlag:   "jammy",
				UsingImageID: true,
			},
		},
		{
			title: "should return no errors when using image-id and charms url series is set",
			selector: SeriesSelector{
				Conf: defaultBase{
					explicit: false,
				},
				CharmURLSeries: "jammy",
				UsingImageID:   true,
			},
		},
		{
			title: "should return no errors when using image-id and explicit base from conf",
			selector: SeriesSelector{
				Conf: defaultBase{
					explicit: true,
				},
				UsingImageID: true,
			},
		},
	}

	for i, test := range deploySeriesTests {
		c.Logf("test %d [%s]", i, test.title)
		test.selector.Logger = &noOpLogger{}
		err := test.selector.Validate()
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
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

type noOpLogger struct{}

func (noOpLogger) Infof(string, ...interface{})  {}
func (noOpLogger) Tracef(string, ...interface{}) {}
