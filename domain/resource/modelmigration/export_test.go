// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreresouces "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	domainresource "github.com/juju/juju/domain/resource"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

var fingerprint = []byte("123456789012345678901234567890123456789012345678")

type exportSuite struct {
	testhelpers.IsolationSuite

	exportService *MockExportService
}

func TestExportSuite(t *stdtesting.T) { tc.Run(t, &exportSuite{}) }
func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) TestResourceExportEmpty(c *tc.C) {
	model := description.NewModel(description.ModelArgs{})

	exportOp := exportOperation{
		service: s.exportService,
	}

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportSuite) TestResourceExport(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: add an app and unit to the model.
	model := description.NewModel(description.ModelArgs{})
	appName := "app-name"
	app := model.AddApplication(description.ApplicationArgs{
		Name: appName,
	})
	unitName := "app-name/0"
	app.AddUnit(description.UnitArgs{
		Name: unitName,
	})

	fp, err := resource.NewFingerprint(fingerprint)
	c.Assert(err, tc.ErrorIsNil)

	// Arrange: create resource data.
	res1Name := "resource-1"
	res1Revision := 1
	res1Time := time.Now().Truncate(time.Second).UTC()
	res1Origin := resource.OriginStore
	res1Size := int64(21)
	res1RetrievedBy := "retrieved by 1"
	res2Name := "resource-2"
	res2Revision := -1
	res2Origin := resource.OriginUpload
	res2Time := time.Now().Truncate(time.Second).Add(-time.Hour).UTC()
	res2Size := int64(12)
	res2RetrievedBy := "retrieved by 2"
	unitResName := "resource-3"
	unitResRevision := -1
	unitResOrigin := resource.OriginUpload
	unitResTime := time.Now().Truncate(time.Second).Add(-time.Hour).UTC()
	unitResSize := int64(32)
	unitResRetrievedBy := "retrieved by 3"

	// Arrange: expect ExportResources for the app.
	s.exportService.EXPECT().ExportResources(gomock.Any(), appName).Return(domainresource.ExportedResources{
		Resources: []coreresouces.Resource{{
			Resource: resource.Resource{
				Meta: resource.Meta{
					Name: res1Name,
				},
				Origin:      res1Origin,
				Revision:    res1Revision,
				Fingerprint: fp,
				Size:        res1Size,
			},
			Timestamp:   res1Time,
			RetrievedBy: res1RetrievedBy,
		}, {
			Resource: resource.Resource{
				Meta: resource.Meta{
					Name: res2Name,
				},
				Origin:      res2Origin,
				Revision:    res2Revision,
				Fingerprint: fp,
				Size:        res2Size,
			},
			Timestamp:   res2Time,
			RetrievedBy: res2RetrievedBy,
		}},
		UnitResources: []coreresouces.UnitResources{{
			Name: coreunit.Name(unitName),
			Resources: []coreresouces.Resource{{
				Resource: resource.Resource{
					Meta: resource.Meta{
						Name: unitResName,
					},
					Origin:      unitResOrigin,
					Revision:    unitResRevision,
					Fingerprint: fp,
					Size:        unitResSize,
				},
				Timestamp:   unitResTime,
				RetrievedBy: unitResRetrievedBy,
			}},
		}}},
		nil,
	)

	// Act: export the resources
	exportOp := exportOperation{
		service: s.exportService,
	}
	err = exportOp.Execute(c.Context(), model)

	// Assert: check no errors occurred.
	c.Assert(err, tc.ErrorIsNil)

	// Assert the app has resources.
	apps := model.Applications()
	c.Assert(apps, tc.HasLen, 1)
	resources := apps[0].Resources()
	c.Assert(resources, tc.HasLen, 2)
	c.Check(resources[0].Name(), tc.Equals, res1Name)

	// Assert resource 1 was exported correctly.
	res1AppRevision := resources[0].ApplicationRevision()
	c.Check(res1AppRevision.Revision(), tc.Equals, res1Revision)
	c.Check(res1AppRevision.Origin(), tc.Equals, res1Origin.String())
	c.Check(res1AppRevision.RetrievedBy(), tc.Equals, res1RetrievedBy)
	c.Check(res1AppRevision.SHA384(), tc.Equals, fp.String())
	c.Check(res1AppRevision.Size(), tc.Equals, res1Size)
	c.Check(res1AppRevision.Timestamp(), tc.Equals, res1Time)

	// Assert resource 2 was exported correctly.
	res2AppRevision := resources[1].ApplicationRevision()
	c.Check(res2AppRevision.Revision(), tc.Equals, res2Revision)
	c.Check(res2AppRevision.Origin(), tc.Equals, res2Origin.String())
	c.Check(res2AppRevision.RetrievedBy(), tc.Equals, res2RetrievedBy)
	c.Check(res2AppRevision.SHA384(), tc.Equals, fp.String())
	c.Check(res2AppRevision.Size(), tc.Equals, res2Size)
	c.Check(res2AppRevision.Timestamp(), tc.Equals, res2Time)

	// Assert the unit resource was exported correctly.
	units := app.Units()
	c.Assert(units, tc.HasLen, 1)
	unitResources := units[0].Resources()
	c.Assert(unitResources, tc.HasLen, 1)
	c.Check(unitResources[0].Name(), tc.Equals, unitResName)
	unitResourceRevision := unitResources[0].Revision()
	c.Check(unitResourceRevision.Revision(), tc.Equals, unitResRevision)
	c.Check(unitResourceRevision.Origin(), tc.Equals, unitResOrigin.String())
	c.Check(unitResourceRevision.RetrievedBy(), tc.Equals, unitResRetrievedBy)
	c.Check(unitResourceRevision.SHA384(), tc.Equals, fp.String())
	c.Check(unitResourceRevision.Size(), tc.Equals, unitResSize)
	c.Check(unitResourceRevision.Timestamp(), tc.Equals, unitResTime)
}
