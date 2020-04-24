// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"time"

	"github.com/juju/worker/v2/workertest"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type modelWatcherSuite struct {
	EntitySuite
}

var _ = gc.Suite(&modelWatcherSuite{})

var modelChange = ModelChange{
	ModelUUID: "some-uuid",
}

func (s *modelWatcherSuite) TestWorkerNoModel(c *gc.C) {
	w := newModelWatcher("some-uuid", s.Hub, nil)
	workertest.CleanKill(c, w)
}

func (s *modelWatcherSuite) TestWorkerWithModel(c *gc.C) {
	w := newModelWatcher("some-uuid", s.Hub, s.NewModel(modelChange))
	workertest.CleanKill(c, w)
}

func (s *modelWatcherSuite) TestChangesWithModel(c *gc.C) {
	model := s.NewModel(modelChange)
	w := newModelWatcher("some-uuid", s.Hub, model)
	defer workertest.CleanKill(c, w)

	select {
	case m := <-w.Changes():
		c.Assert(m.UUID(), gc.Equals, "some-uuid")
	case <-time.After(testing.ShortWait):
		// There should be no time needed to wait as the model should be immediately
		// available on the changes channel.
		c.Errorf("No change after %s", testing.ShortWait)
	}

	// Send another event and make sure we can read one off.
	s.Hub.Publish(modelUpdatedTopic, model)

	select {
	case m := <-w.Changes():
		c.Assert(m.UUID(), gc.Equals, "some-uuid")
	case <-time.After(testing.ShortWait):
		// There should be no time needed to wait as the model should be immediately
		// available on the changes channel.
		c.Errorf("No change after %s", testing.ShortWait)
	}
}

func (s *modelWatcherSuite) TestChangesNoModel(c *gc.C) {
	w := newModelWatcher("some-uuid", s.Hub, nil)
	defer workertest.CleanKill(c, w)

	select {
	case m := <-w.Changes():
		c.Errorf("unexpected change %s", pretty.Sprint(m))
	case <-time.After(testing.ShortWait):
	}

	model := s.NewModel(modelChange)
	s.Hub.Publish(modelUpdatedTopic, model)

	select {
	case m := <-w.Changes():
		c.Assert(m.UUID(), gc.Equals, "some-uuid")
	case <-time.After(testing.ShortWait):
		// There should be no time needed to wait as the model should be immediately
		// available on the changes channel.
		c.Errorf("No change after %s", testing.ShortWait)
	}
}

func (s *modelWatcherSuite) TestChangesDifferentModel(c *gc.C) {
	w := newModelWatcher("some-uuid", s.Hub, nil)
	defer workertest.CleanKill(c, w)

	model := s.NewModel(ModelChange{
		ModelUUID: "other-uuid",
	})
	s.Hub.Publish(modelUpdatedTopic, model)

	select {
	case m := <-w.Changes():
		c.Errorf("unexpected change %s", pretty.Sprint(m))
	case <-time.After(testing.ShortWait):
	}
}

func (s *modelWatcherSuite) TestMultipleUpdates(c *gc.C) {
	w := newModelWatcher("some-uuid", s.Hub, nil)
	defer workertest.CleanKill(c, w)

	model := s.NewModel(modelChange)
	s.Hub.Publish(modelUpdatedTopic, model)
	s.Hub.Publish(modelUpdatedTopic, model)
	handled := s.Hub.Publish(modelUpdatedTopic, model)

	// We know the events are handled in order, so we only need to wait for the
	// last of the three events to have been processed.
	select {
	case <-handled:
	case <-time.After(testing.LongWait):
		c.Errorf("publish event not handled")
	}

	select {
	case m := <-w.Changes():
		c.Assert(m.UUID(), gc.Equals, "some-uuid")
	case <-time.After(testing.ShortWait):
		// There should be no time needed to wait as the model should be immediately
		// available on the changes channel.
		c.Errorf("No change after %s", testing.ShortWait)
	}
}
