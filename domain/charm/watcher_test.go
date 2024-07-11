// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/charm/service"
	"github.com/juju/juju/domain/charm/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchCharm(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewStringsWatcherC(c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(func(c *gc.C) {
		id, err = svc.SetCharm(context.Background(), &stubCharm{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.AssertWatcher) {
		w.AssertChange(id.String())
	})

	// Ensure that we get the charm deleted event.

	harness.AddTest(func(c *gc.C) {
		err := svc.DeleteCharm(context.Background(), id)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.AssertWatcher) {
		w.AssertChange(id.String())
	})

	harness.Run(c)
}

type stubCharm struct{}

func (s *stubCharm) Meta() *internalcharm.Meta {
	return &internalcharm.Meta{
		Name: "test",
	}
}

func (s *stubCharm) Manifest() *internalcharm.Manifest {
	return &internalcharm.Manifest{
		Bases: []internalcharm.Base{
			{
				Name: "ubuntu",
				Channel: internalcharm.Channel{
					Risk: internalcharm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

func (s *stubCharm) Config() *internalcharm.Config {
	return nil
}

func (s *stubCharm) Actions() *internalcharm.Actions {
	return nil
}

func (s *stubCharm) Revision() int {
	return 0
}
