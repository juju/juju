// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	jujutesting "github.com/juju/juju/testing"
)

type modelconfigSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) TestWatch(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults modelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	}

	st := state.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "uuid"),
		jujutesting.NewCheckLogger(c))
	svc := service.NewWatchableService(defaults, st, factory)

	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)
	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Assert(len(changes), gc.Equals, 0)

	err = svc.SetModelConfig(ctx, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := svc.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})

	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Check(changes, jc.SameContents, []string{
		"name", "uuid", "type", "foo", "secret-backend", "logging-config",
	})
}

type modelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

func (f modelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}
