// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	jujutesting "github.com/juju/juju/testing"
)

type handlerSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&handlerSuite{})

func (s *handlerSuite) TestSecretBackendOnLoad(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), modelUUID).Return("some-backend", nil)

	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	got, err := h.OnLoad(ctx)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]string{
		"secret-backend": "some-backend",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveNoUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)

	rawCfg := map[string]any{
		"name": "some-model",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	ctx := context.Background()
	rb, err := h.OnSave(ctx, rawCfg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rb(ctx), jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), modelUUID).Return("orig-backend", nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "some-backend").Return(nil)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	_, err := h.OnSave(context.Background(), rawCfg)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *handlerSuite) TestSecretBackendOnSaveNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), modelUUID).Return("orig-backend", nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "some-backend").Return(backenderrors.NotFound)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	_, err := h.OnSave(context.Background(), rawCfg)
	c.Check(err, jc.ErrorIs, backenderrors.NotFound)
}

func (s *handlerSuite) TestSecretBackendOnSaveRollback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), modelUUID).Return("orig-backend", nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "some-backend").Return(nil)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "orig-backend").Return(nil)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	ctx := context.Background()
	rb, err := h.OnSave(ctx, rawCfg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rb(ctx), jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}
