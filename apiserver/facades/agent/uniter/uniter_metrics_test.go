// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type unitMetricBatchesSuite struct {
	uniterSuiteBase

	*commontesting.ModelWatcherTest
	uniter *uniter.UniterAPI

	meteredApplication *state.Application
	meteredCharm       *state.Charm
	meteredUnit        *state.Unit
}

var _ = gc.Suite(&unitMetricBatchesSuite{})

func (s *unitMetricBatchesSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.SetUpTest(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	s.meteredCharm = f.MakeCharm(c, &factory.CharmParams{
		Name: "metered",
		URL:  "ch:amd64/quantal/metered",
	})
	s.meteredApplication = f.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.meteredCharm,
	})
	s.meteredUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.meteredApplication,
		SetCharmURL: true,
	})

	meteredAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: s.meteredUnit.Tag(),
	}
	st := s.ControllerModel(c).State()
	s.uniter = s.newUniterAPI(c, st, meteredAuthorizer)

	s.ModelWatcherTest = commontesting.NewModelWatcherTest(
		s.uniter,
		st,
		s.resources,
	)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatch(c *gc.C) {
	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.uniter.AddMetricBatches(context.Background(), params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.meteredUnit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.meteredCharm.String(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}},
	)

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.ControllerModel(c).State().MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.meteredCharm.String())
	c.Assert(batch.Unit(), gc.Equals, s.meteredUnit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatchNoCharmURL(c *gc.C) {
	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.uniter.AddMetricBatches(context.Background(), params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.meteredUnit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.meteredCharm.String(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}})

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.ControllerModel(c).State().MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.meteredCharm.String())
	c.Assert(batch.Unit(), gc.Equals, s.meteredUnit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatchDiffTag(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	unit2 := f.MakeUnit(c, &factory.UnitParams{Application: s.meteredApplication, SetCharmURL: true})

	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	tests := []struct {
		about  string
		tag    string
		expect string
	}{{
		about:  "different unit",
		tag:    unit2.Tag().String(),
		expect: "permission denied",
	}, {
		about:  "user tag",
		tag:    names.NewLocalUserTag("admin").String(),
		expect: `"user-admin" is not a valid unit tag`,
	}, {
		about:  "machine tag",
		tag:    names.NewMachineTag("0").String(),
		expect: `"machine-0" is not a valid unit tag`,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		result, err := s.uniter.AddMetricBatches(context.Background(), params.MetricBatchParams{
			Batches: []params.MetricBatchParam{{
				Tag: test.tag,
				Batch: params.MetricBatch{
					UUID:     uuid,
					CharmURL: "",
					Created:  time.Now(),
					Metrics:  metrics,
				}}}})

		if test.expect == "" {
			c.Assert(result.OneError(), jc.ErrorIsNil)
		} else {
			c.Assert(result.OneError(), gc.ErrorMatches, test.expect)
		}
		c.Assert(err, jc.ErrorIsNil)

		_, err = s.ControllerModel(c).State().MetricBatch(uuid)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}
