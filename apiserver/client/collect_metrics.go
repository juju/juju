// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/metrics/spool"
)

// CollectMetrics triggers the CollectMetrics command on specified units/services.
func (c *Client) CollectMetrics(collect params.CollectMetricsParams) (results params.CollectMetricsResults, err error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.CollectMetricsResults{}, errors.Trace(err)
	}
	units, err := getAllUnitNames(c.api.state(), collect.Units, collect.Services)
	if err != nil {
		return params.CollectMetricsResults{}, errors.Trace(err)
	}
	collectResults := []params.CollectMetricsResult{}
	var execParams []*RemoteExec
	for _, unit := range units {
		err := checkUnitAndService(unit)
		if err != nil {
			collectResults = append(collectResults, params.CollectMetricsResult{
				UnitId: unit.Name(),
				Error:  err.Error(),
			})
			continue
		}
		// We know that the unit is both a principal unit, and that it has an
		// assigned machine.
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return results, err
		}
		machine, err := c.api.stateAccessor.Machine(machineId)
		if err != nil {
			return results, err
		}
		command := fmt.Sprintf("juju-collect-metrics %s", unit.Name())
		execParam := remoteParamsForMachine(machine, command, collect.Timeout)
		execParam.UnitId = unit.Name()
		execParams = append(execParams, execParam)
	}
	runResults := ParallelExecute(c.getDataDir(), execParams)
	for _, runResult := range runResults.Results {
		result := params.CollectMetricsResult{
			UnitId: runResult.UnitId,
			Error:  runResult.Error,
		}
		if runResult.Error == "" {
			err := addMetricBatches(c.api.state(), runResult.Stdout)
			if err != nil {
				result.Error = err.Error()
			}
		}
		collectResults = append(collectResults, result)
	}
	results.Results = collectResults
	return results, nil
}

func checkUnitAndService(unit *state.Unit) error {
	chURL, _ := unit.CharmURL()
	if chURL == nil || chURL.Schema != "local" {
		return errors.New("not a local charm")
	}
	service, err := unit.Service()
	if err != nil {
		return errors.Trace(err)
	}
	charm, _, err := service.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	metrics := charm.Metrics()
	if metrics == nil || len(metrics.Metrics) == 0 {
		return errors.New("not a metered charm")
	}
	return nil
}

func addMetricBatches(st *state.State, cmdStdout []byte) error {
	var batches []spool.MetricBatch
	err := json.Unmarshal(cmdStdout, &batches)
	if err != nil {
		return errors.Trace(err)
	}
	for _, batch := range batches {
		unitTag, err := names.ParseUnitTag(batch.UnitTag)
		if err != nil {
			return errors.Trace(err)
		}
		metrics := make([]state.Metric, len(batch.Metrics))
		for j, metric := range batch.Metrics {
			metrics[j] = state.Metric{
				Key:   metric.Key,
				Value: metric.Value,
				Time:  metric.Time,
			}
		}
		_, err = st.AddMetrics(state.BatchParam{
			UUID:     batch.UUID,
			CharmURL: batch.CharmURL,
			Created:  batch.Created,
			Unit:     unitTag,
			Metrics:  metrics,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
