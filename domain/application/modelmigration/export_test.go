// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type exportApplicationSuite struct {
	exportSuite
}

var _ = gc.Suite(&exportApplicationSuite{})

func (s *exportApplicationSuite) TestApplicationExportEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return(nil, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.Applications(), gc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return(nil, errors.Errorf("boom"))

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, ".*boom")
	c.Check(model.Applications(), gc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportNoLocator(c *gc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		ModelType: model.IAAS,
		CharmUUID: charmUUID,
	}}, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, `.*exporting charm URL: unsupported source ""`)
	c.Check(model.Applications(), gc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportMultipleApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		ModelType: model.IAAS,
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}, {
		Name:      "prometheus-k8s",
		ModelType: model.CAAS,
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus-k8s",
			Revision:     42,
			Architecture: architecture.PPC64EL,
		},
	}}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus-k8s").Return(false, nil)

	s.expectCharmOriginFor("prometheus")
	s.expectApplicationConfigFor("prometheus")
	s.expectMinimalCharmFor("prometheus")
	s.expectApplicationConstraintsFor("prometheus", constraints.Value{})

	s.expectCharmOriginFor("prometheus-k8s")
	s.expectApplicationConfigFor("prometheus-k8s")
	s.expectMinimalCharmFor("prometheus-k8s")
	s.expectApplicationConstraintsFor("prometheus-k8s", constraints.Value{})
	s.expectGetApplicationScaleStateFor("prometheus-k8s", application.ScaleState{
		Scaling:     true,
		Scale:       1,
		ScaleTarget: 2,
	})

	s.expectApplicationUnitsFor("prometheus")
	s.expectApplicationUnitsFor("prometheus-k8s")

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 2)

	apps := model.Applications()
	c.Check(apps[0].Name(), gc.Equals, "prometheus")
	c.Check(apps[1].Name(), gc.Equals, "prometheus-k8s")

	c.Check(apps[0].CharmURL(), gc.Equals, "ch:amd64/prometheus-42")
	c.Check(apps[1].CharmURL(), gc.Equals, "ch:ppc64el/prometheus-k8s-42")

	// Check that the scaling state is not set for the first application.
	c.Check(apps[0].ProvisioningState().ScaleTarget(), gc.Equals, 0)
	c.Check(apps[0].ProvisioningState().Scaling(), jc.IsFalse)
	c.Check(apps[0].DesiredScale(), gc.Equals, 0)

	// Check that the scaling state is set for the second application.
	c.Check(apps[1].ProvisioningState().ScaleTarget(), gc.Equals, 2)
	c.Check(apps[1].ProvisioningState().Scaling(), jc.IsTrue)
	c.Check(apps[1].DesiredScale(), gc.Equals, 1)
}

func (s *exportApplicationSuite) TestApplicationExportUnits(c *gc.C) {
	// Arrange:
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		ModelType: model.IAAS,
		CharmLocator: charm.CharmLocator{
			Source: charm.CharmHubSource,
		},
	}}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	s.expectCharmOriginFor("prometheus")
	s.expectApplicationConfigFor("prometheus")
	s.expectMinimalCharmFor("prometheus")
	s.expectApplicationConstraintsFor("prometheus", constraints.Value{})

	s.exportService.EXPECT().GetApplicationUnits(gomock.Any(), "prometheus").Return([]application.ExportUnit{{
		Name:      "prometheus/0",
		Machine:   "0",
		Principal: "principal1/0",
	}, {
		Name:      "prometheus/1",
		Machine:   "1",
		Principal: "principal2/0",
	}}, nil)

	// Act:
	exportOp := s.newExportOperation()
	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	apps := model.Applications()
	c.Check(apps[0].Name(), gc.Equals, "prometheus")

	// Check that the scaling state is set for the second application.
	units := apps[0].Units()
	c.Check(units, gc.HasLen, 2)
	c.Check(units[0].Name(), gc.Equals, "prometheus/0")
	c.Check(units[0].Machine(), gc.Equals, "0")
	c.Check(units[0].Principal(), gc.Equals, "principal1/0")

	c.Check(units[1].Name(), gc.Equals, "prometheus/1")
	c.Check(units[1].Machine(), gc.Equals, "1")
	c.Check(units[1].Principal(), gc.Equals, "principal2/0")
}

func (s *exportApplicationSuite) TestApplicationExportConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	cons := constraints.Value{
		AllocatePublicIP: ptr(true),
		Arch:             ptr("amd64"),
		Container:        ptr(instance.ContainerType("lxd")),
		CpuCores:         ptr(uint64(2)),
		CpuPower:         ptr(uint64(1000)),
		ImageID:          ptr("foo"),
		InstanceRole:     ptr("bar"),
		InstanceType:     ptr("baz"),
		VirtType:         ptr("vm"),
		Mem:              ptr(uint64(1024)),
		RootDisk:         ptr(uint64(1024)),
		RootDiskSource:   ptr("qux"),
		Spaces:           ptr([]string{"space0", "space1"}),
		Tags:             ptr([]string{"tag0", "tag1"}),
		Zones:            ptr([]string{"zone0", "zone1"}),
	}
	s.expectApplicationConstraints(cons)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.Constraints().AllocatePublicIP(), gc.Equals, true)
	c.Check(app.Constraints().Architecture(), gc.Equals, "amd64")
	c.Check(app.Constraints().Container(), gc.Equals, "lxd")
	c.Check(app.Constraints().CpuCores(), gc.Equals, uint64(2))
	c.Check(app.Constraints().CpuPower(), gc.Equals, uint64(1000))
	c.Check(app.Constraints().ImageID(), gc.Equals, "foo")
	c.Check(app.Constraints().InstanceType(), gc.Equals, "baz")
	c.Check(app.Constraints().VirtType(), gc.Equals, "vm")
	c.Check(app.Constraints().Memory(), gc.Equals, uint64(1024))
	c.Check(app.Constraints().RootDisk(), gc.Equals, uint64(1024))
	c.Check(app.Constraints().RootDiskSource(), gc.Equals, "qux")
	c.Check(app.Constraints().Spaces(), gc.DeepEquals, []string{"space0", "space1"})
	c.Check(app.Constraints().Tags(), gc.DeepEquals, []string{"tag0", "tag1"})
	c.Check(app.Constraints().Zones(), gc.DeepEquals, []string{"zone0", "zone1"})
}

func (s *exportApplicationSuite) TestExportScalingState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus-k8s",
		ModelType: model.CAAS,
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus-k8s",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}}, nil)
	s.exportService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "prometheus-k8s").Return(application.CharmOrigin{
		Name:   "prometheus-k8s",
		Source: charm.CharmHubSource,
		Platform: application.Platform{
			OSType:       application.Ubuntu,
			Channel:      "24.04",
			Architecture: architecture.AMD64,
		},
	}, nil)
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus-k8s").Return(false, nil)

	s.expectMinimalCharmFor("prometheus-k8s")
	s.expectApplicationConfigFor("prometheus-k8s")
	s.expectApplicationConstraintsFor("prometheus-k8s", constraints.Value{})
	s.expectGetApplicationScaleStateFor("prometheus-k8s", application.ScaleState{
		Scaling:     true,
		ScaleTarget: 42,
		Scale:       1,
	})
	s.expectApplicationUnitsFor("prometheus-k8s")

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.ProvisioningState().ScaleTarget(), gc.Equals, 42)
	c.Check(app.ProvisioningState().Scaling(), jc.IsTrue)
	c.Check(app.DesiredScale(), gc.Equals, 1)
}

func (s *exportApplicationSuite) TestApplicationExportExposedEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectApplication(c)
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.expectApplicationConstraints(constraints.Value{})
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(true, nil)
	s.exportService.EXPECT().GetExposedEndpoints(gomock.Any(), "prometheus").Return(map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("beta"),
		},
		"foo": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
			ExposeToCIDRs:    set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	}, nil)

	exportOp := s.newExportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.ExposedEndpoints(), gc.HasLen, 2)
	c.Check(app.ExposedEndpoints()[""].ExposeToSpaceIDs(), jc.SameContents, []string{"beta"})
	c.Check(app.ExposedEndpoints()["foo"].ExposeToSpaceIDs(), jc.SameContents, []string{"space0", "space1"})
	c.Check(app.ExposedEndpoints()["foo"].ExposeToCIDRs(), jc.SameContents, []string{"10.0.0.0/24", "10.0.1.0/24"})
}

func (s *exportApplicationSuite) TestApplicationExportEndpointBindings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	charmUUID := charmtesting.GenCharmID(c)
	spaceUUID := uuid.MustNewUUID().String()

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      "prometheus",
		ModelType: model.IAAS,
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         "prometheus",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		EndpointBindings: map[string]string{
			"":         network.AlphaSpaceId,
			"endpoint": spaceUUID,
			"misc":     "",
		},
	}}, nil)
	s.expectCharmOriginFor("prometheus")
	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationUnits()
	s.expectApplicationConstraints(constraints.Value{})
	s.exportService.EXPECT().IsApplicationExposed(gomock.Any(), "prometheus").Return(false, nil)

	// Act:
	exportOp := s.newExportOperation()
	model := description.NewModel(description.ModelArgs{})
	err := exportOp.Execute(context.Background(), model)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	app := model.Applications()[0]
	c.Check(app.EndpointBindings(), gc.HasLen, 3)
	c.Check(app.EndpointBindings()[""], gc.Equals, network.AlphaSpaceId)
	c.Check(app.EndpointBindings()["endpoint"], gc.Equals, spaceUUID)
	c.Check(app.EndpointBindings()["misc"], gc.Equals, "")
}
