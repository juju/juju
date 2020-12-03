// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type dashboardVersionSuite struct {
	ConnSuite
}

var _ = gc.Suite(&dashboardVersionSuite{})

func (s *dashboardVersionSuite) TestDashboardSetVersionNotFoundError(c *gc.C) {
	err := s.State.DashboardSetVersion(version.MustParse("2.0.1"))
	c.Assert(err, gc.ErrorMatches, `cannot find "2.0.1" Dashboard version in the storage: 2.0.1 binary metadata not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *dashboardVersionSuite) TestDashboardVersionNotFoundError(c *gc.C) {
	_, err := s.State.DashboardVersion()
	c.Assert(err, gc.ErrorMatches, "Juju Dashboard version not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *dashboardVersionSuite) TestDashboardSetVersion(c *gc.C) {
	vers := s.addArchive(c, "2.1.0")
	err := s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	obtainedVers, err := s.State.DashboardVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVers, gc.Equals, vers)
}

func (s *dashboardVersionSuite) TestDashboardSwitchVersion(c *gc.C) {
	err := s.State.DashboardSetVersion(s.addArchive(c, "2.47.0"))
	c.Assert(err, jc.ErrorIsNil)

	vers := s.addArchive(c, "2.42.0")
	err = s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	obtainedVers, err := s.State.DashboardVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVers, gc.Equals, vers)

	// The collection still only includes one document.
	s.checkCount(c)
}

// addArchive adds a fake Juju Dashboard archive to the binary storage.
func (s *dashboardVersionSuite) addArchive(c *gc.C, vers string) version.Number {
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	content := "content " + vers
	err = storage.Add(bytes.NewReader([]byte(content)), binarystorage.Metadata{
		SHA256:  "hash",
		Size:    int64(len(content)),
		Version: vers,
	})
	c.Assert(err, jc.ErrorIsNil)
	return version.MustParse(vers)
}

// checkCount ensures that there is only one document in the Dashboard settings
// mongo collection.
func (s *dashboardVersionSuite) checkCount(c *gc.C) {
	settings := s.State.MongoSession().DB("juju").C(state.GUISettingsC)
	count, err := settings.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)
}
