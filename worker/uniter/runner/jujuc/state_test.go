// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/errors"
)

type stateSuite struct {
	jujucSuite
}

func (s *stateSuite) expectStateSetOne() {
	s.mockContext.EXPECT().SetCacheValue("one", "two").Return(nil)
}

func (s *stateSuite) expectStateSetOneEmpty() {
	s.mockContext.EXPECT().SetCacheValue("one", "").Return(nil)
}

func (s *stateSuite) expectStateSetTwo() {
	s.expectStateSetOne()
	s.mockContext.EXPECT().SetCacheValue("three", "four").Return(nil)
}

func (s *stateSuite) expectStateDeleteOne() {
	s.mockContext.EXPECT().DeleteCacheValue("five").Return(nil)
}

func (s *stateSuite) expectStateGetTwo() {
	setupCache := map[string]string{
		"one":   "two",
		"three": "four",
	}
	s.mockContext.EXPECT().GetCache().Return(setupCache, nil)
}

func (s *stateSuite) expectStateGetValueOne() {
	s.mockContext.EXPECT().GetSingleCacheValue("one").Return("two", nil)
}

func (s *stateSuite) expectStateGetValueNotFound() {
	s.mockContext.EXPECT().GetSingleCacheValue("five").Return("", errors.NotFoundf("%q", "five"))
}
