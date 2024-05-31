// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/testing"
)

type handlerSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&handlerSuite{})

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
	rb, err := h.OnSave(ctx, rawCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb(ctx), jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), model.UUID("some-uuid")).Return("orig-backend", nil)
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
	_, err := h.OnSave(ctx, rawCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveRollback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), model.UUID("some-uuid")).Return("orig-backend", nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), model.UUID("some-uuid"), "some-backend").Return(nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), model.UUID("some-uuid"), "orig-backend").Return(nil)

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
	rb, err := h.OnSave(ctx, rawCfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb(ctx), jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}
