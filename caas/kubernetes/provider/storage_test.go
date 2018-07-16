// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/storage"
)

var _ = gc.Suite(&storageSuite{})

type storageSuite struct {
	BaseSuite
	k8sClient kubernetes.Interface
}

func (s *storageSuite) k8sProvider(c *gc.C, ctrl *gomock.Controller) storage.Provider {
	s.k8sClient = mocks.NewMockInterface(ctrl)

	return provider.StorageProvider(s.k8sClient, testNamespace)
}

func (s *storageSuite) TestValidateConfig(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", provider.K8s_ProviderType, map[string]interface{}{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attrs(), jc.DeepEquals, map[string]interface{}{
		"storage-class":       "my-storage",
		"storage-provisioner": "aws-storage",
		"storage-label":       "storage-fred",
	})
}

func (s *storageSuite) TestValidateConfigError(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	cfg, err := storage.NewConfig("name", provider.K8s_ProviderType, map[string]interface{}{
		"storage-class":       "",
		"storage-provisioner": "aws-storage",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, "storage-class must be specified if storage-provisioner is specified")
}

func (s *storageSuite) TestValidateConfigDefaultStorageClass(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg, err := provider.NewStorageConfig(map[string]interface{}{"storage-provisioner": "ebs"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.StorageClass(cfg), gc.Equals, "juju-unit-storage")
}

func (s *storageSuite) TestNewStorageConfig(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg, err := provider.NewStorageConfig(map[string]interface{}{
		"storage-class":       "juju-ebs",
		"storage-provisioner": "ebs",
		"parameters.type":     "gp2",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.StorageClass(cfg), gc.Equals, "juju-ebs")
	c.Assert(provider.StorageProvisioner(cfg), gc.Equals, "ebs")
	c.Assert(provider.StorageParameters(cfg), jc.DeepEquals, map[string]string{"type": "gp2"})
}

func (s *storageSuite) TestSupports(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsFalse)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsTrue)
}

func (s *storageSuite) TestScope(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	p := s.k8sProvider(c, ctrl)
	c.Assert(p.Scope(), gc.Equals, storage.ScopeMachine)
}
