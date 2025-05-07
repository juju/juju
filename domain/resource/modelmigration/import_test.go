// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator     *MockCoordinator
	resourceService *MockImportService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.resourceService = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		resourceService: s.resourceService,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, nil, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Empty model.
	model := description.NewModel(description.ModelArgs{})

	s.resourceService.EXPECT().ImportResources(gomock.Any(), nil)

	// Act:
	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	res1Name := "resource-1"
	res1Revision := 1
	res1Origin := resource.OriginStore
	res1Time := time.Now().Truncate(time.Second).UTC()
	res1 := app.AddResource(description.ResourceArgs{
		Name: res1Name,
	})
	res1.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:  res1Revision,
		Origin:    res1Origin.String(),
		Timestamp: res1Time,
	})
	res2Name := "resource-2"
	res2Revision := -1
	res2Origin := resource.OriginUpload
	res2Time := time.Now().Truncate(time.Second).Add(-time.Hour).UTC()
	res2 := app.AddResource(description.ResourceArgs{
		Name: res2Name,
	})
	res2.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:  res2Revision,
		Origin:    res2Origin.String(),
		Timestamp: res2Time,
	})
	unitName := "app-name/0"
	unitRes1Time := time.Now().Truncate(time.Second).Add(-time.Hour * 2).UTC()
	unit := app.AddUnit(description.UnitArgs{
		Name: unitName,
	})
	unit.AddResource(description.UnitResourceArgs{
		Name: res1Name,
		RevisionArgs: description.ResourceRevisionArgs{
			Timestamp: unitRes1Time,
			Revision:  res1Revision,
			Origin:    res1Origin.String(),
		},
	})
	// Arrange: Give the unit a version of the second resource with a different
	// origin and revision to its application resource.
	unitRes2Revision := 4
	unitRes2Origin := resource.OriginStore
	unitRes2Time := time.Now().Truncate(time.Second).Add(-time.Hour).UTC()
	unit.AddResource(description.UnitResourceArgs{
		Name: res2Name,
		RevisionArgs: description.ResourceRevisionArgs{
			Timestamp: unitRes2Time,
			Revision:  unitRes2Revision,
			Origin:    unitRes2Origin.String(),
		},
	})

	s.resourceService.EXPECT().ImportResources(gomock.Any(), []domainresource.ImportResourcesArg{{
		ApplicationName: appName,
		Resources: []domainresource.ImportResourceInfo{{
			Name:      res1Name,
			Origin:    res1Origin,
			Revision:  res1Revision,
			Timestamp: res1Time,
		}, {
			Name:      res2Name,
			Origin:    res2Origin,
			Revision:  res2Revision,
			Timestamp: res2Time,
		}},
		UnitResources: []domainresource.ImportUnitResourceInfo{{
			UnitName: unitName,
			ImportResourceInfo: domainresource.ImportResourceInfo{
				Name:      res1Name,
				Origin:    res1Origin,
				Revision:  res1Revision,
				Timestamp: unitRes1Time,
			},
		}, {
			UnitName: unitName,
			ImportResourceInfo: domainresource.ImportResourceInfo{
				Name:      res2Name,
				Origin:    unitRes2Origin,
				Revision:  unitRes2Revision,
				Timestamp: unitRes2Time,
			},
		}},
	}})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportRevisionOriginUpload checks that when a resource with origin upload
// is imported, the revision is set to -1.
func (s *importSuite) TestImportRevisionOriginUpload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	resName := "resource-1"
	resRevision := 1
	resOrigin := resource.OriginUpload
	resTime := time.Now().Truncate(time.Second).UTC()
	res := app.AddResource(description.ResourceArgs{
		Name: resName,
	})
	res.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:  resRevision,
		Origin:    resOrigin.String(),
		Timestamp: resTime,
	})
	s.resourceService.EXPECT().ImportResources(gomock.Any(), []domainresource.ImportResourcesArg{{
		ApplicationName: appName,
		Resources: []domainresource.ImportResourceInfo{{
			Name:      resName,
			Origin:    resOrigin,
			Revision:  -1,
			Timestamp: resTime,
		}},
	}})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)

	c.Assert(err, tc.ErrorIsNil)
}

// TestImportRevisionNotValidOriginStore checks that an error is thrown when a
// revision is found that is incompatible with a resource with the origin:
// store.
func (s *importSuite) TestImportRevisionNotValidOriginStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	resName := "resource-1"
	resRevision := -1
	resOrigin := resource.OriginStore
	resTime := time.Now().Truncate(time.Second).UTC()
	res := app.AddResource(description.ResourceArgs{
		Name: resName,
	})
	res.SetApplicationRevision(description.ResourceRevisionArgs{
		Revision:  resRevision,
		Origin:    resOrigin.String(),
		Timestamp: resTime,
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *importSuite) TestImportOriginNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	resName := "resource-1"
	resTime := time.Now().Truncate(time.Second).UTC()
	res := app.AddResource(description.ResourceArgs{
		Name: resName,
	})
	res.SetApplicationRevision(description.ResourceRevisionArgs{
		Origin:    "bad-origin",
		Timestamp: resTime,
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, resourceerrors.OriginNotValid)
}

func (s *importSuite) TestImportResourceNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	resTime := time.Now().Truncate(time.Second).UTC()
	res := app.AddResource(description.ResourceArgs{
		Name: "",
	})
	res.SetApplicationRevision(description.ResourceRevisionArgs{
		Timestamp: resTime,
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, resourceerrors.ResourceNameNotValid)
}
