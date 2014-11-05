// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wireformat_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/metricsender/wireformat"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type WireFormatSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&WireFormatSuite{})

func (s *WireFormatSuite) TestToWire(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	now := time.Now().Round(time.Second)
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: false, Time: &now})
	result := wireformat.ToWire(metric)
	m := metric.Metrics()[0]
	metrics := []wireformat.Metric{
		{
			Key:         m.Key,
			Value:       m.Value,
			Time:        m.Time.UTC(),
			Credentials: m.Credentials,
		},
	}
	expected := &wireformat.MetricBatch{
		UUID:     metric.UUID(),
		EnvUUID:  metric.EnvUUID(),
		Unit:     metric.Unit(),
		CharmUrl: metric.CharmURL(),
		Created:  metric.Created().UTC(),
		Metrics:  metrics,
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *WireFormatSuite) TestAck(c *gc.C) {
	resp := wireformat.EnvironmentResponses{}
	c.Assert(resp, gc.HasLen, 0)

	envUUID := "env-uuid"
	envUUID2 := "env-uuid2"
	batchUUID := "batch-uuid"
	batchUUID2 := "batch-uuid2"

	resp.Ack(envUUID, batchUUID)
	resp.Ack(envUUID, batchUUID2)
	resp.Ack(envUUID2, batchUUID)
	c.Assert(resp, gc.HasLen, 2)

	c.Assert(resp[envUUID].AcknowledgedBatches, jc.SameContents, []string{batchUUID, batchUUID2})
	c.Assert(resp[envUUID2].AcknowledgedBatches, jc.SameContents, []string{batchUUID})
}
