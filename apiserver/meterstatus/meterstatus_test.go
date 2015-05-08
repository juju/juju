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

func (s *meterStatusSuite) TestNotAvailable(c *gc.C) {
	status, err := meterstatus.MeterStatusWrapper(NotAvailableGetter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterRed)
	c.Assert(status.Info, gc.Equals, "not available")
}

func (s *meterStatusSuite) TestNotSet(c *gc.C) {
	status, err := meterstatus.MeterStatusWrapper(NotSetGetter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterAmber)
	c.Assert(status.Info, gc.Equals, "not set")
}

func (s *meterStatusSuite) TestColour(c *gc.C) {
	status, err := meterstatus.MeterStatusWrapper(RedGetter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterRed)
	c.Assert(status.Info, gc.Equals, "info")
	status, err = meterstatus.MeterStatusWrapper(GreenGetter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterGreen)
	c.Assert(status.Info, gc.Equals, "info")
	status, err = meterstatus.MeterStatusWrapper(AmberGetter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterAmber)
	c.Assert(status.Info, gc.Equals, "info")
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
