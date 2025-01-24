// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domainresource "github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator     *MockCoordinator
	resourceService *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: Empty model.
	model := description.NewModel(description.ModelArgs{})

	s.resourceService.EXPECT().ImportResources(gomock.Any(), nil)

	// Act:
	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag(appName),
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
	unitResTime := time.Now().Truncate(time.Second).Add(-time.Hour * 2).UTC()
	unit := app.AddUnit(description.UnitArgs{
		Tag: names.NewUnitTag(unitName),
	})
	unit.AddResource(description.UnitResourceArgs{
		Name: res1Name,
		RevisionArgs: description.ResourceRevisionArgs{
			Timestamp: unitResTime,
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
			ResourceName: res1Name,
			UnitName:     unitName,
			Timestamp:    unitResTime,
		}},
	}})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

// TestImportRevisionNotValidOriginUpload checks that an error is thrown when a
// revision is found that is incompatible with a resource with the origin:
// upload.
func (s *importSuite) TestImportRevisionNotValidOriginUpload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag(appName),
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

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

// TestImportRevisionNotValidOriginStore checks that an error is thrown when a
// revision is found that is incompatible with a resource with the origin:
// store.
func (s *importSuite) TestImportRevisionNotValidOriginStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag(appName),
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
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceRevisionNotValid)
}

func (s *importSuite) TestImportOriginNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag(appName),
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
	c.Assert(err, jc.ErrorIs, resourceerrors.OriginNotValid)
}

func (s *importSuite) TestImportResourceNameNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appName := "app-name"
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag(appName),
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
	c.Assert(err, jc.ErrorIs, resourceerrors.ResourceNameNotValid)
}
