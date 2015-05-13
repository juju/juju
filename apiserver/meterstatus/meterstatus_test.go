// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/meterstatus"
	"github.com/juju/juju/state"
)

type meterStatusSuite struct{}

var _ = gc.Suite(&meterStatusSuite{})

func (s *meterStatusSuite) TestError(c *gc.C) {
	_, err := meterstatus.MeterStatusWrapper(ErrorGetter)
	c.Assert(err, gc.ErrorMatches, "an error")
}

func (s *meterStatusSuite) TestWrapper(c *gc.C) {
	tests := []struct {
		about          string
		input          func() (state.MeterStatus, error)
		expectedOutput state.MeterStatus
	}{{
		about:          "notset in, amber out",
		input:          NotSetGetter,
		expectedOutput: state.MeterStatus{state.MeterAmber, "not set"},
	}, {
		about:          "notavailable in, red out",
		input:          NotAvailableGetter,
		expectedOutput: state.MeterStatus{state.MeterRed, "not available"},
	}, {
		about:          "red in, red out",
		input:          RedGetter,
		expectedOutput: state.MeterStatus{state.MeterRed, "info"},
	}, {
		about:          "green in, green out",
		input:          GreenGetter,
		expectedOutput: state.MeterStatus{state.MeterGreen, "info"},
	}, {
		about:          "amber in, amber out",
		input:          AmberGetter,
		expectedOutput: state.MeterStatus{state.MeterAmber, "info"},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		status, err := meterstatus.MeterStatusWrapper(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Code, gc.Equals, test.expectedOutput.Code)
		c.Assert(status.Info, gc.Equals, test.expectedOutput.Info)
	}
}

func ErrorGetter() (state.MeterStatus, error) {
	return state.MeterStatus{}, errors.New("an error")
}

func NotAvailableGetter() (state.MeterStatus, error) {
	return state.MeterStatus{state.MeterNotAvailable, ""}, nil
}

func NotSetGetter() (state.MeterStatus, error) {
	return state.MeterStatus{state.MeterNotSet, ""}, nil
}

func RedGetter() (state.MeterStatus, error) {
	return state.MeterStatus{state.MeterRed, "info"}, nil
}

func GreenGetter() (state.MeterStatus, error) {
	return state.MeterStatus{state.MeterGreen, "info"}, nil
}
func AmberGetter() (state.MeterStatus, error) {
	return state.MeterStatus{state.MeterAmber, "info"}, nil
}
