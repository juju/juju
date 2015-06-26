// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/resumer"
	coretesting "github.com/juju/juju/testing"
)

type ResumerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ResumerSuite{})

func (s *ResumerSuite) TestResumeTransactionsSuccess(c *gc.C) {
	var callCount int
	apiCaller := apitesting.APICallerFunc(
		func(objType string, version int, id, request string, args, results interface{}) error {
			c.Check(objType, gc.Equals, "Resumer")
			// Since we're not logging in and getting the supported
			// facades and their versions, the client will always send
			// version 0.
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ResumeTransactions")
			c.Check(args, gc.IsNil)
			c.Check(results, gc.IsNil)
			callCount++
			return nil
		},
	)

	st := resumer.NewAPI(apiCaller)
	err := st.ResumeTransactions()
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 1)
}

func (s *ResumerSuite) TestResumeTransactionsFailure(c *gc.C) {
	var callCount int
	apiCaller := apitesting.APICallerFunc(
		func(_ string, _ int, _, _ string, _, _ interface{}) error {
			callCount++
			return errors.New("boom!")
		},
	)

	st := resumer.NewAPI(apiCaller)
	err := st.ResumeTransactions()
	c.Check(err, gc.ErrorMatches, "boom!")
	c.Check(callCount, gc.Equals, 1)
}
