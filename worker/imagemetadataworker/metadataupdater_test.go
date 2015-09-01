// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/imagemetadataworker"
)

var _ = gc.Suite(&imageMetadataUpdateSuite{})

type imageMetadataUpdateSuite struct {
	baseMetadataSuite
}

func (s *imageMetadataUpdateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}
func (s *imageMetadataUpdateSuite) TestWorker(c *gc.C) {

	var list imagemetadataworker.ListPublishedMetadataFunc = func(env environs.Environ) ([]*imagemetadata.ImageMetadata, error) {
		return []*imagemetadata.ImageMetadata{{Id: "whatever"}}, nil
	}

	done := make(chan struct{})
	client := s.ImageClient(func(m []params.CloudImageMetadata) error {
		close(done)
		return nil
	})

	w := imagemetadataworker.NewWorker(client, list, s.SomeEnviron())

	defer w.Wait()
	defer w.Kill()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for images metadata to update")
	}
}

func (s *imageMetadataUpdateSuite) TestMetadataChanges(c *gc.C) {
	var stored []params.CloudImageMetadata
	client := s.ImageClient(func(m []params.CloudImageMetadata) error {
		stored = append(stored, m...)
		return nil
	})

	imageId := "work"
	var list imagemetadataworker.ListPublishedMetadataFunc = func(env environs.Environ) ([]*imagemetadata.ImageMetadata, error) {
		return []*imagemetadata.ImageMetadata{{Id: imageId}}, nil
	}

	env := s.SomeEnviron()
	oneM := params.CloudImageMetadata{ImageId: imageId, Source: "public"}
	var expected []params.CloudImageMetadata

	for i := 0; i < 2; i++ {
		err := imagemetadataworker.DoWork(client, list, env)
		c.Assert(err, jc.ErrorIsNil)
		expected = append(expected, oneM)
		c.Assert(stored, gc.DeepEquals, expected)

	}
}
