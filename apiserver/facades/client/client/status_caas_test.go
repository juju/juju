// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"

	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/version"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type CAASStatusSuite struct {
	baseSuite

	model *state.Model
	app   *state.Application
}

var _ = gc.Suite(&CAASStatusSuite{})

func (s *CAASStatusSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.baseSuite.SeedCAASCloud(c)
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// Set up a CAAS model to replace the IAAS one.

	// For the test to run properly with part of the model in mongo and
	// part in a service domain, a model with the same uuid is required
	// in both places for the test to work. Necessary after model config
	// was move to the domain services.
	modelUUID, err := uuid.UUIDFromString(s.DefaultModelUUID.String())
	c.Assert(err, jc.ErrorIsNil)
	st := f.MakeCAASModel(c, &factory.ModelParams{UUID: &modelUUID})
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.model = m

	ch := f2.MakeCharm(c, &factory.CharmParams{
		Name:   "mysql-k8s",
		Series: "focal",
	})
	s.app = f2.MakeApplication(c, &factory.ApplicationParams{
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				OS:           "ubuntu",
				Channel:      "20.04/stable",
				Architecture: "amd64",
			},
		},
		Charm: ch,
	})
	f2.MakeUnit(c, &factory.UnitParams{Application: s.app})
}

func (s *CAASStatusSuite) TestStatusOperatorNotReady(c *gc.C) {
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))

	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "waiting", "installing agent")
}

func (s *CAASStatusSuite) TestStatusCloudContainerSet(c *gc.C) {
	loggo.GetLogger("juju.state.status").SetLogLevel(loggo.TRACE)
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	u, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		u[0].UpdateOperation(state.UnitUpdateProperties{
			CloudContainerStatus: &status.StatusInfo{Status: status.Blocked, Message: "blocked"},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "blocked", "blocked")
}

func (s *CAASStatusSuite) assertUnitStatus(c *gc.C, appStatus params.ApplicationStatus, status, info string) {
	curl, _ := s.app.CharmURL()
	workloadVersion := ""
	if info != "installing agent" && info != "blocked" {
		workloadVersion = "gitlab/latest"
	}
	c.Assert(appStatus, jc.DeepEquals, params.ApplicationStatus{
		Charm:           *curl,
		Base:            params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		WorkloadVersion: workloadVersion,
		Relations:       map[string][]string{},
		SubordinateTo:   []string{},
		Units: map[string]params.UnitStatus{
			s.app.Name() + "/0": {
				AgentStatus: params.DetailedStatus{
					Version: version.Current.String(),
					Status:  "allocating",
				},
				WorkloadStatus: params.DetailedStatus{
					Status: status,
					Info:   info,
				},
			},
		},
		Status: params.DetailedStatus{
			Status: status,
			Info:   info,
		},
		EndpointBindings: map[string]string{
			"":             network.AlphaSpaceName,
			"server":       network.AlphaSpaceName,
			"server-admin": network.AlphaSpaceName,
		},
	})
}

func (s *CAASStatusSuite) TestStatusWorkloadVersionSetByCharm(c *gc.C) {
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.SetScale(1, 1, true)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u, gc.HasLen, 1)
	err = u[0].SetWorkloadVersion("666")
	c.Assert(err, jc.ErrorIsNil)
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	app := status.Applications[s.app.Name()]
	c.Assert(app.WorkloadVersion, gc.Equals, "666")
	c.Assert(app.Scale, gc.Equals, 1)
}
