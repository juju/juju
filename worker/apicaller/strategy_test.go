// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apicaller/mocks"
)

type StrategyConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StrategyConnectSuite{})

func (*StrategyConnectSuite) TestConnect(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Debugf(gomock.Any())

	mockConn := mocks.NewMockConnection(ctrl)

	password := "aaabbbccc"

	fn := func(info *api.Info, _ api.DialOpts) (api.Connection, error) {
		c.Assert(info.Password, gc.Equals, password)
		return mockConn, nil
	}

	strategy := apicaller.DefaultConnectStrategy(fn, mockLogger)
	conn, requiredFallback, err := strategy.Connect(&api.Info{
		Password: password,
	}, "xxxyyyzzz")
	c.Assert(conn, gc.Equals, mockConn)
	c.Assert(requiredFallback, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
}

func (*StrategyConnectSuite) TestConnectWithNoPassword(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Debugf(gomock.Any())

	mockConn := mocks.NewMockConnection(ctrl)

	password := ""
	fallbackPassword := "xxxyyyzzz"

	fn := func(info *api.Info, _ api.DialOpts) (api.Connection, error) {
		c.Assert(info.Password, gc.Equals, fallbackPassword)
		return mockConn, nil
	}

	strategy := apicaller.DefaultConnectStrategy(fn, mockLogger)
	conn, requiredFallback, err := strategy.Connect(&api.Info{
		Password: password,
	}, fallbackPassword)
	c.Assert(conn, gc.Equals, mockConn)
	c.Assert(requiredFallback, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

func (*StrategyConnectSuite) TestConnectWithPasswordWithUnauthorizedError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Debugf(gomock.Any()).Times(2)

	mockConn := mocks.NewMockConnection(ctrl)

	password := "aaabbbccc"
	fallbackPassword := "xxxyyyzzz"

	var (
		first = true
		got   []string
	)
	fn := func(info *api.Info, _ api.DialOpts) (api.Connection, error) {
		defer func() {
			first = false
		}()

		got = append(got, info.Password)

		if first {
			return nil, params.Error{
				Code: params.CodeUnauthorized,
			}
		}
		return mockConn, nil
	}

	strategy := apicaller.DefaultConnectStrategy(fn, mockLogger)
	conn, requiredFallback, err := strategy.Connect(&api.Info{
		Password: password,
	}, fallbackPassword)
	c.Assert(conn, gc.Equals, mockConn)
	c.Assert(requiredFallback, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(got, jc.DeepEquals, []string{password, fallbackPassword})
}
