// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/testing"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type handlerSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&handlerSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *handlerSuite) TestSecretBackendOnLoad(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), model.UUID("some-uuid")).Return("some-backend", nil)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    "some-uuid",
	}
	got, err := h.OnLoad(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]string{
		"secret-backend": "some-backend",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveNoUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	rawCfg := map[string]any{
		"name": "some-model",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    "some-uuid",
	}
	err := h.OnSave(ctx, rawCfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), model.UUID("some-uuid"), "some-backend").Return(nil)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    "some-uuid",
	}
	err := h.OnSave(ctx, rawCfg, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveResetWithDefault(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), model.UUID("some-uuid"), "myvault").Return(nil)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	defaults := modeldefaults.Defaults{
		"secret-backend": modeldefaults.DefaultAttributeValue{
			Source: config.JujuControllerSource,
			Value:  "myvault",
		},
	}
	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		Defaults:     defaults,
		BackendState: st,
		ModelUUID:    "some-uuid",
	}
	err := h.OnSave(ctx, rawCfg, []string{"secret-backend"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveResetNoDefault(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), model.UUID("some-uuid"), "auto").Return(nil)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		Defaults:     modeldefaults.Defaults{},
		BackendState: st,
		ModelUUID:    "some-uuid",
	}
	err := h.OnSave(ctx, rawCfg, []string{"secret-backend"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}
