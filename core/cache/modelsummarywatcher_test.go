// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"sort"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/testing"
)

type modelSummaryWatcherSuite struct {
	cache.BaseSuite
	controller *cache.Controller
	events     <-chan interface{}
}

var _ = gc.Suite(&modelSummaryWatcherSuite{})

func (s *modelSummaryWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.controller, s.events = s.New(c)
	s.baseScenario(c)
	loggo.GetLogger("").SetLogLevel(loggo.TRACE)
}

func (s *modelSummaryWatcherSuite) TestInitialModelsAll(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	initial := s.next(c, changes)
	c.Assert(initial, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:         "controller-uuid",
			Namespace:    "test-admin",
			Name:         "controller",
			Admins:       []string{"test-admin"},
			Status:       cache.StatusGreen,
			MachineCount: 1,
		}, {
			UUID:             "model-1-uuid",
			Namespace:        "test-admin",
			Name:             "model-1",
			Admins:           []string{"test-admin"},
			Status:           cache.StatusGreen,
			MachineCount:     1,
			ApplicationCount: 1,
			UnitCount:        1,
		}, {
			UUID:      "model-2-uuid",
			Namespace: "bob",
			Admins:    []string{"bob"},
			Name:      "model-2",
			Status:    cache.StatusGreen,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestInitialModelsBob(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("bob")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	initial := s.next(c, changes)
	c.Assert(initial, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:             "model-1-uuid",
			Namespace:        "test-admin",
			Name:             "model-1",
			Admins:           []string{"test-admin"},
			Status:           cache.StatusGreen,
			MachineCount:     1,
			ApplicationCount: 1,
			UnitCount:        1,
		}, {
			UUID:      "model-2-uuid",
			Namespace: "bob",
			Name:      "model-2",
			Admins:    []string{"bob"},
			Status:    cache.StatusGreen,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestInitialModelsCharlie(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("charlie")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	initial := s.next(c, changes)
	c.Assert(initial, gc.HasLen, 0)
}

func (s *modelSummaryWatcherSuite) TestAddPermissionShowsModel(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("charlie")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the initial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-2-uuid",
		Name:      "model-2",
		Life:      life.Alive,
		Owner:     "bob",
		UserPermissions: map[string]permission.Access{
			"albert":  permission.AdminAccess,
			"bob":     permission.AdminAccess,
			"mary":    permission.ReadAccess,
			"charlie": permission.ReadAccess,
		},
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-2-uuid",
			Namespace: "bob",
			Name:      "model-2",
			Admins:    []string{"albert", "bob"},
			Status:    cache.StatusGreen,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestRemovingPermissionRemovesModel(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("bob")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the initial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-2-uuid",
		Name:      "model-2",
		Life:      life.Alive,
		Owner:     "bob",
		UserPermissions: map[string]permission.Access{
			"mary":    permission.ReadAccess,
			"charlie": permission.ReadAccess,
		},
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:    "model-2-uuid",
			Removed: true,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestAddModelShowsModel(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("bob")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the initial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-3-uuid",
		Name:      "model-3",
		Life:      life.Alive,
		Owner:     "mary",
		UserPermissions: map[string]permission.Access{
			"bob":  permission.ReadAccess,
			"mary": permission.AdminAccess,
		},
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-3-uuid",
			Namespace: "mary",
			Name:      "model-3",
			Admins:    []string{"mary"},
			Status:    cache.StatusGreen,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestRemoveModelRemovesModel(c *gc.C) {
	watcher := s.controller.WatchModelsAsUser("bob")
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the initial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.RemoveModel{
		ModelUUID: "model-2-uuid",
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:    "model-2-uuid",
			Removed: true,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestModelAnnotationsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the initial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-2-uuid",
		Name:      "model-2",
		Life:      life.Alive,
		Owner:     "bob",
		UserPermissions: map[string]permission.Access{
			"bob":  permission.AdminAccess,
			"mary": permission.ReadAccess,
		},
		Annotations: map[string]string{
			"muted": "true",
		},
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-2-uuid",
			Namespace: "bob",
			Name:      "model-2",
			Admins:    []string{"bob"},
			Status:    cache.StatusGreen,
			Annotations: map[string]string{
				"muted": "true",
			},
		},
	})
}

func (s *modelSummaryWatcherSuite) TestAddingMachineIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.MachineChange{
		ModelUUID: "model-2-uuid",
		Id:        "0",
		Life:      life.Alive,
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:         "model-2-uuid",
			Namespace:    "bob",
			Name:         "model-2",
			Admins:       []string{"bob"},
			Status:       cache.StatusGreen,
			MachineCount: 1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestRemovingMachineIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.RemoveMachine{
		ModelUUID: "model-1-uuid",
		Id:        "0",
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-1-uuid",
			Namespace: "test-admin",
			Name:      "model-1",
			Admins:    []string{"test-admin"},
			Status:    cache.StatusGreen,
			// We didn't actually remove the application, or unit yet.
			ApplicationCount: 1,
			UnitCount:        1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestAddingApplicationIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.ApplicationChange{
		ModelUUID: "model-2-uuid",
		Name:      "foo",
		Life:      life.Alive,
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:             "model-2-uuid",
			Namespace:        "bob",
			Name:             "model-2",
			Admins:           []string{"bob"},
			Status:           cache.StatusGreen,
			ApplicationCount: 1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestRemovingApplicationIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.RemoveApplication{
		ModelUUID: "model-1-uuid",
		Name:      "magic",
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-1-uuid",
			Namespace: "test-admin",
			Name:      "model-1",
			Admins:    []string{"test-admin"},
			Status:    cache.StatusGreen,
			// We didn't actually remove the machine, or unit yet.
			// Yes I know in theory this can't happen, but hey, this is a test.
			MachineCount: 1,
			UnitCount:    1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestAddingUnitIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.UnitChange{
		ModelUUID: "model-1-uuid",
		Name:      "magic/1",
		Life:      life.Alive,
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:             "model-1-uuid",
			Namespace:        "test-admin",
			Name:             "model-1",
			Admins:           []string{"test-admin"},
			Status:           cache.StatusGreen,
			MachineCount:     1,
			ApplicationCount: 1,
			UnitCount:        2,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestRemovingUnitIsChange(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.RemoveUnit{
		ModelUUID: "model-1-uuid",
		Name:      "magic/0",
	}, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-1-uuid",
			Namespace: "test-admin",
			Name:      "model-1",
			Admins:    []string{"test-admin"},
			Status:    cache.StatusGreen,
			// We didn't actually remove the machine, or application yet.
			MachineCount:     1,
			ApplicationCount: 1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestChangesToOneModelCoalesced(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	s.ProcessChange(c, cache.RemoveUnit{
		ModelUUID: "model-1-uuid",
		Name:      "magic/0",
	}, s.events)
	s.ProcessChange(c, cache.RemoveApplication{
		ModelUUID: "model-1-uuid",
		Name:      "magic",
	}, s.events)
	s.ProcessChange(c, cache.RemoveMachine{
		ModelUUID: "model-1-uuid",
		Id:        "0",
	}, s.events)
	s.ProcessChange(c, cache.ApplicationChange{
		ModelUUID: "model-2-uuid",
		Name:      "foo",
		Life:      life.Alive,
	}, s.events)

	update := s.next(c, changes, "model-2-uuid")
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "model-1-uuid",
			Namespace: "test-admin",
			Name:      "model-1",
			Admins:    []string{"test-admin"},
			Status:    cache.StatusGreen,
		}, {
			UUID:             "model-2-uuid",
			Namespace:        "bob",
			Name:             "model-2",
			Admins:           []string{"bob"},
			Status:           cache.StatusGreen,
			ApplicationCount: 1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestUpdatesThatDontChangeSummary(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	modelUpdate := cache.ModelChange{
		ModelUUID: "new-model-uuid",
		Name:      "new-model",
		Life:      life.Alive,
		Owner:     "mary",
		UserPermissions: map[string]permission.Access{
			"bob":  permission.ReadAccess,
			"mary": permission.AdminAccess,
		},
	}
	s.ProcessChange(c, modelUpdate, s.events)

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "new-model-uuid",
			Namespace: "mary",
			Name:      "new-model",
			Admins:    []string{"mary"},
			Status:    cache.StatusGreen,
		},
	})

	// Now we send the same model update, the hash shouldn't change
	// so it shouldn't generate a new event.
	s.ProcessChange(c, modelUpdate, s.events)
	// We send another event after it so we aren't waiting for something
	// to not happen, which makes tests slower. Here we add an application
	// to an existing model to force an update.
	s.ProcessChange(c, cache.ApplicationChange{
		ModelUUID: "model-2-uuid",
		Name:      "foo",
		Life:      life.Alive,
	}, s.events)

	update = s.next(c, changes, "model-2-uuid")

	// No update for the new model, but we do see the second model
	// application change.
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:             "model-2-uuid",
			Namespace:        "bob",
			Name:             "model-2",
			Admins:           []string{"bob"},
			Status:           cache.StatusGreen,
			ApplicationCount: 1,
		},
	})
}

func (s *modelSummaryWatcherSuite) TestNoUpdatesDuringInitialization(c *gc.C) {
	watcher := s.controller.WatchAllModels()
	defer workertest.CleanKill(c, watcher)

	changes := watcher.Changes()
	// discard the intial event
	_ = s.next(c, changes)

	// Simulate a watcher reset.
	s.controller.Mark()

	// Resend initial state.
	s.baseScenario(c)
	// And another change.
	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "new-model-uuid",
		Name:      "new-model",
		Life:      life.Alive,
		Owner:     "mary",
		UserPermissions: map[string]permission.Access{
			"bob":  permission.ReadAccess,
			"mary": permission.AdminAccess,
		},
	}, s.events)

	s.noUpdates(c, changes)
	// Sweep triggers model summary updates for all models.

	s.controller.Sweep()

	update := s.next(c, changes)
	c.Assert(update, jc.DeepEquals, []cache.ModelSummary{
		{
			UUID:      "new-model-uuid",
			Namespace: "mary",
			Name:      "new-model",
			Admins:    []string{"mary"},
			Status:    cache.StatusGreen,
		},
	})
}

func (s *modelSummaryWatcherSuite) next(c *gc.C, changes <-chan []cache.ModelSummary, uuids ...string) []cache.ModelSummary {
	// If we are passed the optional uuid in, there should only be one.
	if len(uuids) > 0 {
		if len(uuids) > 1 {
			c.Fatalf("only one uuid should be passed into next, got %d", len(uuids))
		}
		// Make sure all the published summary events have been handled.
		// We know that the events are consumed in the order that they are
		// published. The model uuid for the last model change is what should
		// be passed in here if there are multiple events.
		cache.WaitForModelSummaryHandled(c, s.controller, uuids[0])
	}

	select {
	case <-time.After(testing.LongWait):
		c.Fatal("no changes sent")
	case summaries := <-changes:
		sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].UUID < summaries[j].UUID })
		return summaries
	}
	// Unreachable due to fatal.
	return nil
}

func (s *modelSummaryWatcherSuite) noUpdates(c *gc.C, changes <-chan []cache.ModelSummary) {
	select {
	case <-time.After(testing.ShortWait):
	// Good, didn't expect any.
	case summaries := <-changes:
		c.Fatalf("received %d changes", len(summaries))
	}
}

func (s *modelSummaryWatcherSuite) baseScenario(c *gc.C) {
	// The values here a minimal, and only set values that are really necessary.
	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "controller-uuid",
		Name:      "controller",
		Life:      life.Alive,
		Owner:     "test-admin",
		UserPermissions: map[string]permission.Access{
			"test-admin": permission.AdminAccess,
		},
	}, s.events)
	s.ProcessChange(c, cache.MachineChange{
		ModelUUID: "controller-uuid",
		Id:        "0",
		Life:      life.Alive,
	}, s.events)
	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-1-uuid",
		Name:      "model-1",
		Life:      life.Alive,
		Owner:     "test-admin",
		UserPermissions: map[string]permission.Access{
			"test-admin": permission.AdminAccess,
			"bob":        permission.ReadAccess,
		},
	}, s.events)
	s.ProcessChange(c, cache.MachineChange{
		ModelUUID: "model-1-uuid",
		Id:        "0",
		Life:      life.Alive,
	}, s.events)
	s.ProcessChange(c, cache.CharmChange{
		ModelUUID: "model-1-uuid",
		CharmURL:  "magic-42",
	}, s.events)
	s.ProcessChange(c, cache.ApplicationChange{
		ModelUUID: "model-1-uuid",
		Name:      "magic",
		Life:      life.Alive,
		CharmURL:  "magic-42",
	}, s.events)
	s.ProcessChange(c, cache.UnitChange{
		ModelUUID:   "model-1-uuid",
		Name:        "magic/0",
		Application: "magic",
		CharmURL:    "magic-42",
		Life:        life.Alive,
	}, s.events)
	s.ProcessChange(c, cache.ModelChange{
		ModelUUID: "model-2-uuid",
		Name:      "model-2",
		Life:      life.Alive,
		Owner:     "bob",
		UserPermissions: map[string]permission.Access{
			"bob":  permission.AdminAccess,
			"mary": permission.ReadAccess,
		},
	}, s.events)
}
