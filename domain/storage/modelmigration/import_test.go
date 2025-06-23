// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return provider.CommonStorageProviders()
	}), loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().CreateStoragePool(gomock.All(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "ebs-fast",
		Provider:   "ebs",
		Attributes: map[string]any{"foo": "bar"},
	})
	s.service.EXPECT().CreateStoragePool(gomock.Any(), "ebs-fast", storage.ProviderType("ebs"), map[string]any{"foo": "bar"}).Times(1)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
