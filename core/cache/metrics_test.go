// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	"bytes"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
)

// The metrics hook into the ControllerSuite as it has
// the base methods we need to enable this cleanly.

func (s *ControllerSuite) TestCollect(c *gc.C) {
	loggo.GetLogger("juju.core.cache").SetLogLevel(loggo.TRACE)
	controller, events := s.New(c)

	// Note that the model change is processed last.
	s.ProcessChange(c, charmChange, events)
	s.ProcessChange(c, appChange, events)
	s.ProcessChange(c, machineChange, events)
	s.ProcessChange(c, unitChange, events)
	s.ProcessChange(c, modelChange, events)

	collector := cache.NewMetricsCollector(controller)

	expected := bytes.NewBuffer([]byte(`
# HELP juju_cache_applications Number of applications managed by the controller.
# TYPE juju_cache_applications gauge
juju_cache_applications{life="alive"} 1
# HELP juju_cache_machines Number of machines managed by the controller.
# TYPE juju_cache_machines gauge
juju_cache_machines{agent_status="active",instance_status="active",life="alive"} 1
# HELP juju_cache_models Number of models in the controller.
# TYPE juju_cache_models gauge
juju_cache_models{life="alive",status="active"} 1
# HELP juju_cache_units Number of units managed by the controller.
# TYPE juju_cache_units gauge
juju_cache_units{agent_status="active",life="alive",workload_status="active"} 1
		`[1:]))

	err := testutil.CollectAndCompare(
		collector, expected,
		"juju_cache_models",
		"juju_cache_machines",
		"juju_cache_applications",
		"juju_cache_units")
	if !c.Check(err, jc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}

	workertest.CleanKill(c, controller)
}
