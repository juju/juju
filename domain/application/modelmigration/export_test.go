// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type exportSuite struct {
	testing.IsolationSuite

	exportService *MockExportService
}

type exportApplicationSuite struct {
	exportSuite
}

var _ = gc.Suite(&exportApplicationSuite{})

func (s *exportApplicationSuite) TestApplicationExportEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.Applications(), gc.HasLen, 0)
}

func (s *exportApplicationSuite) TestApplicationExportConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})

	s.expectMinimalCharm()
	s.expectApplicationConfig()
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
	s.expectGetApplicationScaleState(application.ScaleState{})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)

	app = model.Applications()[0]
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

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})

	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationConstraints(constraints.Value{})
	s.expectGetApplicationScaleState(application.ScaleState{
		Scaling:     true,
		ScaleTarget: 42,
		Scale:       1,
	})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Applications(), gc.HasLen, 1)
	app = model.Applications()[0]
	c.Check(app.ProvisioningState().ScaleTarget(), gc.Equals, 42)
	c.Check(app.ProvisioningState().Scaling(), jc.IsTrue)
	c.Check(app.DesiredScale(), gc.Equals, 1)
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) expectMinimalCharm() {
	meta := &internalcharm.Meta{
		Name: "prometheus",
	}
	cfg := &internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}
	ch := internalcharm.NewCharmBase(meta, nil, cfg, nil, nil)
	locator := charm.CharmLocator{
		Revision: 1,
	}
	s.exportService.EXPECT().GetCharmByApplicationName(gomock.Any(), "prometheus").Return(ch, locator, nil)
}

func (s *exportSuite) expectApplicationConfig() {
	config := config.ConfigAttributes{
		"foo": "bar",
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}
	s.exportService.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), "prometheus").Return(config, settings, nil)
}

func (s *exportSuite) expectGetApplicationScaleState(scaleState application.ScaleState) {
	exp := s.exportService.EXPECT()
	exp.GetApplicationScaleState(gomock.Any(), "prometheus").Return(scaleState, nil)
}

func (s *exportSuite) expectApplicationConstraints(cons constraints.Value) {
	s.exportService.EXPECT().GetApplicationConstraints(gomock.Any(), "prometheus").Return(cons, nil)
}
