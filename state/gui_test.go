// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type guiVersionSuite struct {
	ConnSuite
}

var _ = gc.Suite(&guiVersionSuite{})

func (s *guiVersionSuite) TestGUISetVersionNotFoundError(c *gc.C) {
	err := s.State.GUISetVersion(version.MustParse("2.0.1"))
	c.Assert(err, gc.ErrorMatches, `cannot find "2.0.1" GUI version in the storage: 2.0.1 binary metadata not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *guiVersionSuite) TestGUIVersionNotFoundError(c *gc.C) {
	_, err := s.State.GUIVersion()
	c.Assert(err, gc.ErrorMatches, "Juju GUI version not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *guiVersionSuite) TestGUISetVersion(c *gc.C) {
	vers := s.addArchive(c, "2.1.0")
	err := s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	obtainedVers, err := s.State.GUIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVers, gc.Equals, vers)
}

func (s *guiVersionSuite) TestGUISwitchVersion(c *gc.C) {
	err := s.State.GUISetVersion(s.addArchive(c, "2.47.0"))
	c.Assert(err, jc.ErrorIsNil)

	vers := s.addArchive(c, "2.42.0")
	err = s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	obtainedVers, err := s.State.GUIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVers, gc.Equals, vers)

	// The collection still only includes one document.
	s.checkCount(c)
}

// addArchive adds a fake Juju GUI archive to the binary storage.
func (s *guiVersionSuite) addArchive(c *gc.C, vers string) version.Number {
	storage, err := s.State.GUIStorage()
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

// checkCount ensures that there is only one document in the GUI settings
// mongo collection.
func (s *guiVersionSuite) checkCount(c *gc.C) {
	settings := s.State.MongoSession().DB("juju").C(state.GUISettingsC)
	count, err := settings.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 1)
}
