// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package statemetrics_test

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/statemetrics"
	"github.com/juju/juju/status"
)

type collectorSuite struct {
	testing.IsolationSuite
	pool      *mockStatePool
	collector *statemetrics.Collector
}

var _ = gc.Suite(&collectorSuite{})

func (s *collectorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	users := []*mockUser{{
		tag:              names.NewUserTag("alice"),
		controllerAccess: permission.NoAccess,
	}, {
		tag:              names.NewUserTag("bob"),
		controllerAccess: permission.NoAccess,
	}, {
		tag:              names.NewUserTag("cayley@cambridge"),
		deleted:          true,
		controllerAccess: permission.AddModelAccess,
	}, {
		tag:              names.NewUserTag("dominique"),
		disabled:         true,
		controllerAccess: permission.ReadAccess,
	}}
	connectedModel := mockModel{
		tag:    names.NewModelTag("b266dff7-eee8-4297-b03a-4692796ec193"),
		life:   state.Alive,
		status: status.StatusInfo{Status: status.Available},
		machines: []*mockMachine{{
			life:           state.Alive,
			agentStatus:    status.StatusInfo{Status: status.Started},
			instanceStatus: status.StatusInfo{Status: status.Running},
		}},
		users: users,
	}
	s.pool = &mockStatePool{
		models: []*mockModel{
			&connectedModel,
			{
				tag:    names.NewModelTag("1ab5799e-e72d-4de7-b70d-499edfab0e5c"),
				life:   state.Dying,
				status: status.StatusInfo{Status: status.Destroying},
				machines: []*mockMachine{{
					life:           state.Alive,
					agentStatus:    status.StatusInfo{Status: status.Error},
					instanceStatus: status.StatusInfo{Status: status.ProvisioningError},
				}},
				users: users,
			}},
	}
	s.pool.system = &mockState{
		model:      &connectedModel,
		users:      users,
		modelUUIDs: s.pool.modelUUIDs(),
	}
	s.collector = statemetrics.New(s.pool)
}

func (s *collectorSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descStrings []string
	for desc := range ch {
		descStrings = append(descStrings, desc.String())
	}
	expect := []string{
		`.*fqName: "juju_state_machines".*`,
		`.*fqName: "juju_state_models".*`,
		`.*fqName: "juju_state_users".*`,
		`.*fqName: "juju_state_scrape_errors".*`,
		`.*fqName: "juju_state_scrape_duration_seconds".*`,
	}
	c.Assert(descStrings, gc.HasLen, len(expect))
	for i, expect := range expect {
		c.Assert(descStrings[i], gc.Matches, expect)
	}
}

func (s *collectorSuite) collect(c *gc.C) ([]prometheus.Metric, []dto.Metric) {
	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()
	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	dtoMetrics := make([]dto.Metric, len(metrics))
	for i, metric := range metrics {
		err := metric.Write(&dtoMetrics[i])
		c.Assert(err, jc.ErrorIsNil)
	}
	return metrics, dtoMetrics
}

func (s *collectorSuite) checkExpected(c *gc.C, actual, expected []dto.Metric) {
	c.Assert(actual, gc.HasLen, len(expected))
	for i, dm := range actual {
		fmt.Println("actual metric #%d: %+v", i, dm)
		var found bool
		for i, m := range expected {
			if !reflect.DeepEqual(dm, m) {
				continue
			}
			expected = append(expected[:i], expected[i+1:]...)
			found = true
			break
		}
		if !found {
			c.Errorf("metric #%d %+v not expected", i, dm)
		}
	}
}

func float64ptr(v float64) *float64 {
	return &v
}

func (s *collectorSuite) TestCollect(c *gc.C) {
	_, dtoMetrics := s.collect(c)

	// The scrape time metric has a non-deterministic value,
	// so we just check that it is non-zero.
	c.Assert(dtoMetrics, gc.Not(gc.HasLen), 0)
	scrapeDurationMetric := dtoMetrics[len(dtoMetrics)-1]
	c.Assert(scrapeDurationMetric.Gauge.GetValue(), gc.Not(gc.Equals), 0)

	labelpair := func(n, v string) *dto.LabelPair {
		return &dto.LabelPair{Name: &n, Value: &v}
	}
	s.checkExpected(c, dtoMetrics, []dto.Metric{
		// juju_state_machines
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("agent_status", "started"),
				labelpair("life", "alive"),
				labelpair("machine_status", "running"),
			},
		},
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("agent_status", "error"),
				labelpair("life", "alive"),
				labelpair("machine_status", "provisioning error"),
			},
		},

		// juju_state_models
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("life", "alive"),
				labelpair("status", "available"),
			},
		},
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("life", "dying"),
				labelpair("status", "destroying"),
			},
		},

		// juju_state_users
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("controller_access", "add-model"),
				labelpair("deleted", "true"),
				labelpair("disabled", ""),
				labelpair("domain", "cambridge"),
			},
		},
		{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("controller_access", "read"),
				labelpair("deleted", ""),
				labelpair("disabled", "true"),
				labelpair("domain", ""),
			},
		},
		{
			Gauge: &dto.Gauge{Value: float64ptr(2)},
			Label: []*dto.LabelPair{
				labelpair("controller_access", ""),
				labelpair("deleted", ""),
				labelpair("disabled", ""),
				labelpair("domain", ""),
			},
		},

		// juju_state_scrape_errors
		{
			Gauge: &dto.Gauge{Value: float64ptr(0)},
		},

		// juju_state_scrape_interval_seconds
		{
			Gauge: &dto.Gauge{Value: scrapeDurationMetric.Gauge.Value},
		},
	})
}

func (s *collectorSuite) TestCollectErrors(c *gc.C) {
	s.pool.system.SetErrors(
		errors.New("no models for you"),
		errors.New("no users for you"),
	)
	_, dtoMetrics := s.collect(c)

	// The scrape time metric has a non-deterministic value,
	// so we just check that it is non-zero.
	c.Assert(dtoMetrics, gc.Not(gc.HasLen), 0)
	scrapeDurationMetric := dtoMetrics[len(dtoMetrics)-1]
	c.Assert(scrapeDurationMetric.Gauge.GetValue(), gc.Not(gc.Equals), 0)

	s.checkExpected(c, dtoMetrics, []dto.Metric{
		// juju_state_scrape_errors
		{
			Gauge: &dto.Gauge{Value: float64ptr(2)},
		},

		// juju_state_scrape_interval_seconds
		{
			Gauge: &dto.Gauge{Value: scrapeDurationMetric.Gauge.Value},
		},
	})
}
