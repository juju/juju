// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"fmt"
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller/controller"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// legacySuite has the tests for the controller client-side facade
// which use ApiServerSuite. The plan is to gradually move these tests
// to Suite in controller_test.go.
type legacySuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper
}

var _ = gc.Suite(&legacySuite{})

func (s *legacySuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithMultiWatcher = true
	s.ApiServerSuite.SetUpTest(c)
}

func (s *legacySuite) OpenAPI(c *gc.C) *controller.Client {
	return controller.NewClient(s.OpenControllerAPI(c))
}

func (s *legacySuite) TestAllModels(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	owner := names.NewUserTag("user@remote")

	f.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner,
	}).Close()
	f.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: owner,
	}).Close()

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	models, err := sysManager.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 3)

	var obtained []string
	for _, m := range models {
		c.Assert(m.Type, gc.Equals, model.IAAS)
		obtained = append(obtained, fmt.Sprintf("%s/%s", m.Owner, m.Name))
	}
	expected := []string{
		"admin/controller",
		"user@remote/first",
		"user@remote/second",
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *legacySuite) TestModelConfig(c *gc.C) {
	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	cfg, err := sysManager.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["name"], gc.Equals, "controller")
}

func (s *legacySuite) TestControllerConfig(c *gc.C) {
	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	cfg, err := sysManager.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	cfgFromDB, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(int(cfg["state-port"].(float64)), gc.Equals, cfgFromDB.StatePort())
	c.Assert(int(cfg["api-port"].(float64)), gc.Equals, cfgFromDB.APIPort())
}

func (s *legacySuite) TestDestroyController(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	model := f.MakeModel(c, &factory.ModelParams{Name: "foo"})
	f, release = s.NewFactory(c, model.ModelUUID())
	defer release()
	f.MakeMachine(c, nil) // make it non-empty
	model.Close()

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	err := sysManager.DestroyController(controller.DestroyControllerParams{})
	c.Assert(err, gc.ErrorMatches, `failed to destroy model: hosting 1 other model \(controller has hosted models\)`)
}

func (s *legacySuite) TestListBlockedModels(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.SwitchBlockOn(state.ChangeBlock, "change block for controller")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SwitchBlockOn(state.DestroyBlock, "destroy block for controller")
	c.Assert(err, jc.ErrorIsNil)

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	results, err := sysManager.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []params.ModelBlockInfo{
		{
			Name:     "controller",
			UUID:     st.ModelUUID(),
			OwnerTag: jujutesting.AdminUser.String(),
			Blocks: []string{
				"BlockChange",
				"BlockDestroy",
			},
		},
	})
}

func (s *legacySuite) TestRemoveBlocks(c *gc.C) {
	st := s.ControllerModel(c).State()
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	sysManager := s.OpenAPI(c)
	defer sysManager.Close()
	err := sysManager.RemoveBlocks()
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := st.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *legacySuite) TestWatchAllModels(c *gc.C) {
	// The WatchAllModels infrastructure is comprehensively tested
	// else. This test just ensure that the API calls work end-to-end.
	sysManager := s.OpenAPI(c)
	defer sysManager.Close()

	w, err := sysManager.WatchAllModels()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := w.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	deltasC := make(chan []params.Delta)
	go func() {
		deltas, err := w.Next()
		c.Assert(err, jc.ErrorIsNil)
		deltasC <- deltas
	}()

	select {
	case deltas := <-deltasC:
		c.Assert(deltas, gc.HasLen, 1)
		modelInfo := deltas[0].Entity.(*params.ModelUpdate)

		model := s.ControllerModel(c)
		cfg, err := model.Config()
		c.Assert(err, jc.ErrorIsNil)
		status, err := model.Status()
		c.Assert(err, jc.ErrorIsNil)

		// Resource tags are unmarshalled as map[string]interface{}
		// Convert to map[string]string for comparison.
		switch tags := modelInfo.Config[config.ResourceTagsKey].(type) {
		case map[string]interface{}:
			tagStrings := make(map[string]string)
			for tag, val := range tags {
				tagStrings[tag] = fmt.Sprintf("%v", val)
			}
			modelInfo.Config[config.ResourceTagsKey] = tagStrings
		}

		// Integer values are unmarshalled as float64 across
		// the (JSON) api boundary. Explicitly convert to int
		// for the following attributes.
		switch val := modelInfo.Config[config.NetBondReconfigureDelayKey].(type) {
		case float64:
			modelInfo.Config[config.NetBondReconfigureDelayKey] = int(val)
		}
		switch val := modelInfo.Config[config.NumProvisionWorkersKey].(type) {
		case float64:
			modelInfo.Config[config.NumProvisionWorkersKey] = int(val)
		}
		switch val := modelInfo.Config[config.NumContainerProvisionWorkersKey].(type) {
		case float64:
			modelInfo.Config[config.NumContainerProvisionWorkersKey] = int(val)
		}

		expectedStatus := params.StatusInfo{
			Current: status.Status,
			Message: status.Message,
		}
		modelInfo.Status.Since = nil
		c.Assert(modelInfo.ModelUUID, gc.Equals, model.UUID())
		c.Assert(modelInfo.Name, gc.Equals, model.Name())
		c.Assert(modelInfo.Life, gc.Equals, life.Alive)
		c.Assert(modelInfo.Owner, gc.Equals, model.Owner().Id())
		c.Assert(modelInfo.ControllerUUID, gc.Equals, model.ControllerUUID())
		c.Assert(modelInfo.Config, jc.DeepEquals, cfg.AllAttrs())
		c.Assert(modelInfo.Status, jc.DeepEquals, expectedStatus)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *legacySuite) TestAPIServerCanShutdownWithOutstandingNext(c *gc.C) {
	conn := s.OpenControllerAPI(c)
	sysManager := controller.NewClient(conn)
	defer sysManager.Close()

	w, err := sysManager.WatchAllModels()
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()

	deltasC := make(chan struct{}, 2)
	go func() {
		defer close(deltasC)
		for {
			_, err := w.Next()
			if err != nil {
				return
			}
			deltasC <- struct{}{}
		}
	}()
	// Read the first event.
	select {
	case <-deltasC:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
	// Wait a little while for the Next call to actually arrive.
	time.Sleep(testing.ShortWait)

	// We should be able to close the server instance
	// even when there's an outstanding Next call.
	srvStopped := make(chan struct{})
	go func() {
		start := time.Now()
		err = s.Server.Stop()
		if err != nil {
			return
		}
		c.Logf("stop took %v", time.Since(start))
		close(srvStopped)
	}()

	select {
	case <-srvStopped:
	case <-time.After(time.Minute): // LongWait (10s) didn't seem quite long enough, see LP 1900931
		c.Fatal("timed out waiting for server to stop")
	}

	// Check that the Next call has returned too.
	select {
	case _, ok := <-deltasC:
		if ok {
			c.Fatalf("got unexpected event from deltasC")
		}
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *legacySuite) TestGetControllerAccess(c *gc.C) {
	controller := s.OpenAPI(c)
	defer controller.Close()
	err := controller.GrantController("fred@external", "superuser")
	c.Assert(err, jc.ErrorIsNil)
	access, err := controller.GetControllerAccess("fred@external")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.Access("superuser"))
}
