// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modelconfig/service/testing"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *serviceSuite) TestSetModelConfig(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"secret-backend": "auto",
	}

	st := testing.NewState()
	defer st.Close()

	svc := NewWatchableService(defaults, config.ModelValidator(), loggertesting.WrapCheckLog(c), st, st, st)

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
		"agent-version":  jujuversion.Current.String(),
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
		"name", "uuid", "type", "foo", "logging-config",
	})
}

func (s *serviceSuite) TestSetModelConfigSecretBackend(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"secret-backend": "some-backend",
	}

	st := testing.NewState()
	defer st.Close()

	svc := NewWatchableService(defaults, config.ModelValidator(), loggertesting.WrapCheckLog(c), st, st, st)

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
		"agent-version":  jujuversion.Current.String(),
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"secret-backend": "some-backend",
		"logging-config": "<root>=INFO",
	})

	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Check(changes, jc.SameContents, []string{
		"name", "uuid", "type", "foo", "logging-config",
	})
}

func (s *serviceSuite) TestSetModelConfigSecretBackendRollback(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "error",
		"secret-backend": "some-backend",
	}

	st := testing.NewState()
	defer st.Close()

	svc := NewWatchableService(defaults, config.ModelValidator(), loggertesting.WrapCheckLog(c), st, st, st)

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
	c.Assert(err, gc.ErrorMatches, `set config error`)

	cfg, err := svc.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"agent-version":  jujuversion.Current.String(),
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})
}

func (s *serviceSuite) TestUpdateModelConfigSecretBackendRollback(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
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

	st := testing.NewState()
	defer st.Close()

	svc := NewWatchableService(defaults, config.ModelValidator(), loggertesting.WrapCheckLog(c), st, st, st)
	err := svc.SetModelConfig(ctx, attrs)
	c.Assert(err, jc.ErrorIsNil)

	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)
	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-ctx.Done():
		c.Fatal(ctx.Err())
	}
	c.Check(changes, jc.SameContents, []string{
		"name", "uuid", "type", "foo", "logging-config",
	})

	attrs["foo"] = "error"
	attrs["secret-backend"] = "some-backend"
	err = svc.UpdateModelConfig(ctx, attrs, nil)
	c.Assert(err, gc.ErrorMatches, `updating model config: update config error`)

	cfg, err := svc.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	// secret-backend gets rolled back to its original value "auto".
	c.Check(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"agent-version":  jujuversion.Current.String(),
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})

}
