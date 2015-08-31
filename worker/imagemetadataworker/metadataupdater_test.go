// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/worker/imagemetadataworker"
)

type imageMetadataUpdateSuite struct {
	testing.JujuConnSuite

	metadataUpdater *imagemetadataworker.MetadataUpdateWorker
}

var _ = gc.Suite(&imageMetadataUpdateSuite{})

func (s *imageMetadataUpdateSuite) SetUpSuite(c *gc.C) {
	c.Assert(*imagemetadataworker.Interval, gc.Equals, 24*time.Hour)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *imageMetadataUpdateSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *imageMetadataUpdateSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

}

func (s *imageMetadataUpdateSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *imageMetadataUpdateSuite) runUpdater(c *gc.C, updateInterval time.Duration) {
	s.PatchValue(imagemetadataworker.Interval, updateInterval)

	updaterClient := s.APIState.MetadataUpdater()
	c.Assert(updaterClient, gc.NotNil)

	// TODO (anastasiamac 2015-08-31) where am I getting environs from?
	s.metadataUpdater = imagemetadataworker.NewMetadataUpdateWorker(s.Environ, updaterClient)
	s.AddCleanup(func(c *gc.C) { s.metadataUpdater.Stop() })
}

func (s *imageMetadataUpdateSuite) getMetadata(c *gc.C) ([]params.CloudImageMetadata, error) {
	client := s.APIState.MetadataUpdater()
	return client.List("", "", nil, nil, "", "")
}

func (s *imageMetadataUpdateSuite) noStoredMetadata(c *gc.C) []params.CloudImageMetadata {
	all, err := s.getMetadata(c)
	c.Assert(err, gc.ErrorMatches, ".*matching cloud image metadata not found.*")
	return all
}

func (s *imageMetadataUpdateSuite) allStoredMetadata(c *gc.C) []params.CloudImageMetadata {
	all, err := s.getMetadata(c)
	c.Assert(err, jc.ErrorIsNil)
	return all
}

func (s *imageMetadataUpdateSuite) TestMetadataUpdateRunsInitially(c *gc.C) {
	// Make sure that there is nothing before worker runs
	s.noStoredMetadata(c)

	// Run the updater with a long update interval to ensure only the initial
	// update on startup is run.
	s.runUpdater(c, time.Hour)

	// Make sure that there is some metadata
	c.Assert(len(s.allStoredMetadata(c)) > 0, jc.IsTrue)
}

func (s *imageMetadataUpdateSuite) TestMetadataUpdateRunsPeriodically(c *gc.C) {
	// Start the updater
	s.runUpdater(c, 5*time.Millisecond)
	originalMs := s.allStoredMetadata(c)
	c.Assert(len(originalMs) > 0, jc.IsTrue)

	// Make some changes
	imagePrefix := "test-"
	for i, m := range originalMs {
		m.ImageId = fmt.Sprintf("%v%d", imagePrefix, i)
	}
	client := s.APIState.MetadataUpdater()
	errs, err := client.Save(originalMs)
	c.Assert(err, jc.ErrorIsNil)
	for _, e := range errs {
		noErr := e.Error == nil || e.Error.Message == ""
		c.Assert(noErr, jc.IsTrue)
	}

	// Once updater runs, image ids should be correct
	updatedMs := s.allStoredMetadata(c)
	c.Assert(len(updatedMs) > 0, jc.IsTrue)
	c.Assert(len(updatedMs), gc.Equals, len(originalMs))

	for _, m := range updatedMs {
		c.Assert(strings.HasPrefix(m.ImageId, imagePrefix), jc.IsFalse)
	}
}

func (s *imageMetadataUpdateSuite) TestDiesOnError(c *gc.C) {
	msg := "worker err"
	mockUpdate := func(muw *imagemetadataworker.MetadataUpdateWorker) error {
		return errors.New(msg)
	}
	s.PatchValue(&imagemetadataworker.UpdateMetadata, mockUpdate)

	client := s.APIState.MetadataUpdater()
	c.Assert(client, gc.NotNil)

	// TODO (anastasiamac 2015-08-31) need environ here
	metadataUpdater := imagemetadataworker.NewMetadataUpdateWorker(s.Environ, client)
	err := metadataUpdater.Stop()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}
