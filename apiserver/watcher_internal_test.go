// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type allWatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&allWatcherSuite{})

func (s *allWatcherSuite) watcher() *SrvAllWatcher {
	// We explicitly don't have a real watcher here as the tests
	// are for the translation of types.
	return &SrvAllWatcher{}
}

func (s *allWatcherSuite) TestTranslateApplicationWithStatus(c *gc.C) {
	w := s.watcher()
	input := &multiwatcher.ApplicationInfo{
		ModelUUID: testing.ModelTag.Id(),
		Name:      "test-app",
		CharmURL:  "test-app",
		Life:      life.Alive,
		Status: multiwatcher.StatusInfo{
			Current: status.Active,
		},
	}
	output := w.translateApplication(input)
	c.Assert(output, jc.DeepEquals, &params.ApplicationInfo{
		ModelUUID: input.ModelUUID,
		Name:      input.Name,
		CharmURL:  input.CharmURL,
		Life:      input.Life,
		Status: params.StatusInfo{
			Current: status.Active,
		},
	})
}
