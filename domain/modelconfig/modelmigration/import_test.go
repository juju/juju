// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"maps"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator           *MockCoordinator
	service               *MockImportService
	modelDefaultsProvider *MockModelDefaultsProvider
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.modelDefaultsProvider = NewMockModelDefaultsProvider(ctrl)

	c.Cleanup(func() {
		s.coordinator = nil
		s.service = nil
		s.modelDefaultsProvider = nil
	})

	return ctrl
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, s.modelDefaultsProvider, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config, err := config.New(config.NoDefaults, map[string]any{
		"name":             "foo",
		"uuid":             "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":             "sometype",
		"workload-storage": "mystorage",
		"operator-storage": "otherstorage",
	})
	c.Assert(err, tc.ErrorIsNil)
	importedConfig := map[string]any{
		"logging-config":   "<root>=INFO",
		"workload-storage": "mystorage",
	}

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "sometype").Return(configschema.Fields{
		"logging-config":   configschema.Attr{},
		"workload-storage": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), importedConfig).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: config.AllAttrs(),
	})

	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestKubernetesWorkloadStorageBecomesDefaultSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"name":             "foo",
			"uuid":             "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type":             "kubernetes",
			"workload-storage": "ceph-rbd",
		},
	})
	originalConfig := maps.Clone(model.Config())

	// The 4.0 Kubernetes model config schema has no workload-storage key,
	// and the 3.x nomination becomes the default filesystem source.
	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "kubernetes").Return(configschema.Fields{
		"name":                              configschema.Attr{},
		"uuid":                              configschema.Attr{},
		"type":                              configschema.Attr{},
		"storage-default-filesystem-source": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"name":                              "foo",
		"uuid":                              "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":                              "kubernetes",
		"storage-default-filesystem-source": "ceph-rbd",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	// The description's own config is not mutated: the storage pool
	// import reads the workload-storage key from it.
	c.Check(model.Config(), tc.DeepEquals, originalConfig)
}

func (s *importSuite) TestKubernetesWorkloadAndOperatorStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// A 3.x model carrying both storage keys: workload-storage is
	// translated while operator-storage (removed with podspec charms) is
	// dropped by the schema filter with no side effect.
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"type":             "kubernetes",
			"workload-storage": "ceph-rbd",
			"operator-storage": "otherstorage",
		},
	})

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "kubernetes").Return(configschema.Fields{
		"type":                              configschema.Attr{},
		"storage-default-filesystem-source": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"type":                              "kubernetes",
		"storage-default-filesystem-source": "ceph-rbd",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestKubernetesWorkloadStorageDottedClassBecomesPoolSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// The derived default source must name the pool the storage pool
	// import derives from the same key: here the class contains dots, so
	// the derived pool name differs from the class name.
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"type":             "kubernetes",
			"workload-storage": "ebs.csi.aws.com",
		},
	})

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "kubernetes").Return(configschema.Fields{
		"type":                              configschema.Attr{},
		"storage-default-filesystem-source": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"type":                              "kubernetes",
		"storage-default-filesystem-source": "ebs-csi-aws-com",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestKubernetesWorkloadStorageForeignPoolNotRedirected(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// A pool of the derived name that provisions from another class is
	// left alone: the nomination gets no default source, mirroring 3.6,
	// where the existing pool won regardless of its attributes.
	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"type":             "kubernetes",
			"workload-storage": "fast.ssd",
		},
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "fast-ssd",
		Provider:   "kubernetes",
		Attributes: map[string]any{"storage-class": "other-sc"},
	})

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "kubernetes").Return(configschema.Fields{
		"type": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"type": "kubernetes",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestKubernetesWorkloadStorageRespectsDefaultSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"type":                              "kubernetes",
			"workload-storage":                  "ceph-rbd",
			"storage-default-filesystem-source": "my-pool",
		},
	})

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "kubernetes").Return(configschema.Fields{
		"type":                              configschema.Attr{},
		"storage-default-filesystem-source": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"type":                              "kubernetes",
		"storage-default-filesystem-source": "my-pool",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestNonKubernetesWorkloadStorageNotTranslated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"type":             "ec2",
			"workload-storage": "ceph-rbd",
		},
	})

	s.service.EXPECT().GetModelConfigSchemaForCloudType(gomock.Any(), "ec2").Return(configschema.Fields{
		"type": configschema.Attr{},
	}, nil)
	s.service.EXPECT().SetModelConfig(gomock.Any(), map[string]any{
		"type": "ec2",
	}).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service:          s.service,
		defaultsProvider: s.modelDefaultsProvider,
		logger:           loggertesting.WrapCheckLog(c),
	}
}
