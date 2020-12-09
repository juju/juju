// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/ecs"
	"github.com/juju/juju/caas/ecs/constants"
	"github.com/juju/juju/storage"
)

type storageSuite struct {
	baseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) ecsStorageProvider(c *gc.C, ctrl *gomock.Controller) storage.Provider {
	return ecs.StorageProvider(s.environ)
}

func (s *storageSuite) TestValidateConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.ecsStorageProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", constants.StorageProviderType, map[string]interface{}{
		"volume-type": "gp2",
		"driver":      "rexray/ebs",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attrs(), jc.DeepEquals, storage.Attrs{
		"volume-type": "gp2",
		"driver":      "rexray/ebs",
	})
}

func (s *storageSuite) TestSupports(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.ecsStorageProvider(c, ctrl)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *storageSuite) TestScope(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	p := s.ecsStorageProvider(c, ctrl)
	c.Assert(p.Scope(), gc.Equals, storage.ScopeEnviron)
}
