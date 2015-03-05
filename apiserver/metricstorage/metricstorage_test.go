// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricstorage_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricstorage"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricStorageSuite struct {
	jujutesting.JujuConnSuite

	metricstorage *metricstorage.MetricStorageAPI
	authorizer    apiservertesting.FakeAuthorizer
	unit          *state.Unit
	charm         *state.Charm
	service       *state.Service
}

var _ = gc.Suite(&metricStorageSuite{})

func (s *metricStorageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	s.service = s.Factory.MakeService(c, &factory.ServiceParams{Charm: s.charm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.Tag(),
	}
	storage, err := metricstorage.NewMetricStorageAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.metricstorage = storage
}

func (s *metricStorageSuite) TestNewMetricsManagerAPIRefusesNonUnit(c *gc.C) {
	tests := []struct {
		tag           names.Tag
		expectedError string
	}{
		{names.NewMachineTag("0"), "permission denied"},
		{names.NewLocalUserTag("admin"), "permission denied"},
		{s.unit.Tag(), ""},
	}
	for i, test := range tests {
		c.Logf("test %d", i)

		anAuthoriser := s.authorizer
		anAuthoriser.Tag = test.tag
		endPoint, err := metricstorage.NewMetricStorageAPI(s.State, nil, anAuthoriser)
		if test.expectedError == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(endPoint, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedError)
			c.Assert(endPoint, gc.IsNil)
		}
	}
}

func (s *metricStorageSuite) TestAddMetricsBatch(c *gc.C) {
	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.metricstorage.AddMetricBatches(params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.unit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.charm.URL().String(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}})

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.State.MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.charm.URL().String())
	c.Assert(batch.Unit(), gc.Equals, s.unit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *metricStorageSuite) TestAddMetricsBatchNoCharmURL(c *gc.C) {
	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.metricstorage.AddMetricBatches(params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.unit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: "",
				Created:  time.Now(),
				Metrics:  metrics,
			}}}})

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.State.MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.charm.URL().String())
	c.Assert(batch.Unit(), gc.Equals, s.unit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *metricStorageSuite) TestAddMetricsBatchDiffTag(c *gc.C) {
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})

	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
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
		expect: `"user-admin@local" is not a valid unit tag`,
	}, {
		about:  "machine tag",
		tag:    names.NewMachineTag("0").String(),
		expect: `"machine-0" is not a valid unit tag`,
	}}

	for i, test := range tests {
		c.Log("%d: %s", i, test.about)
		result, err := s.metricstorage.AddMetricBatches(params.MetricBatchParams{
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

		_, err = s.State.MetricBatch(uuid)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}
