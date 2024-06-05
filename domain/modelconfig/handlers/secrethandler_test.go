// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type secretBackendHandlerSuite struct {
	jtesting.IsolationSuite
}

var _ = gc.Suite(&secretBackendHandlerSuite{})

func (s *secretBackendHandlerSuite) TestRegisteredKeys(c *gc.C) {
	h := SecretBackendHandler{}
	c.Assert(h.RegisteredKeys(), jc.SameContents, []string{"secret-backend"})
}

func (s *secretBackendHandlerSuite) TestOnLoad(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().GetModelSecretBackendName(gomock.Any(), modelUUID).Return("some-backend", nil)

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.IAAS,
	}
	got, err := h.OnLoad(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, map[string]string{
		"secret-backend": "some-backend",
	})
}

func (s *secretBackendHandlerSuite) TestOnSaveNoUpdate(c *gc.C) {
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
		ModelType:    model.IAAS,
	}
	ctx := context.Background()
	err := h.OnSave(ctx, rawCfg)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(rawCfg, jc.DeepEquals, map[string]any{
		"name": "some-model",
	})
}

func (s *secretBackendHandlerSuite) TestOnSave(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "some-backend").Return(nil)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.IAAS,
	}
	err := h.OnSave(context.Background(), rawCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretBackendHandlerSuite) TestOnSaveAutoIAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "internal").Return(nil)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "auto",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.IAAS,
	}
	err := h.OnSave(context.Background(), rawCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretBackendHandlerSuite) TestOnSaveAutoCAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(nil)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "auto",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.CAAS,
	}
	err := h.OnSave(context.Background(), rawCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretBackendHandlerSuite) TestOnSaveNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)
	st.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "some-backend").Return(backenderrors.NotFound)

	rawCfg := map[string]any{
		"name":           "some-model",
		"secret-backend": "some-backend",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
	}
	err := h.OnSave(context.Background(), rawCfg)
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
}

func (*secretBackendHandlerSuite) TestSecretsBackendChecker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)

	oldCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	}

	newCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "vault",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.IAAS,
	}
	err := h.Validate(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*secretBackendHandlerSuite) TestSecretsBackendCheckerIAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)

	oldCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	}

	newCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "kubernetes",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.IAAS,
	}
	err := h.Validate(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"secret-backend"})
}

func (*secretBackendHandlerSuite) TestSecretsBackendCheckerCAAS(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	st := NewMockSecretBackendState(ctrl)

	oldCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	}

	newCfg := map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "internal",
	}

	h := SecretBackendHandler{
		BackendState: st,
		ModelUUID:    modelUUID,
		ModelType:    model.CAAS,
	}
	err := h.Validate(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"secret-backend"})
}
