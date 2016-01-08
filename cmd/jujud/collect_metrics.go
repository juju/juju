// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	unitercharm "github.com/juju/juju/worker/uniter/charm"
)

// CollectMetricsCommand runs the collect-metrics hook of the specified unit,
// and prints out the collected metrics.
type CollectMetricsCommand struct {
	cmd.CommandBase
	unit     names.UnitTag
	showHelp bool
}

const collectMetricsCommandDoc = `
Run the collect metrics hook for the unit and send collected metrics to
the controller.

unit-name can be either the unit tag:
 i.e.  unit-ubuntu-0
or the unit id:
 i.e.  ubuntu/0
`

// Info returns usage information for the command.
func (c *CollectMetricsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-collect-metrics",
		Args:    "<unit-name>",
		Purpose: "run the collect metrics hook and send collected metrics to the controller",
		Doc:     collectMetricsCommandDoc,
	}
}

// Init implements the cmd.Command interface.
func (c *CollectMetricsCommand) Init(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing arguments")
	}
	var unitName string
	unitName, args = args[0], args[1:]
	// If the command line param is a unit id (like service/2) we need to
	// change it to the unit tag as that is the format of the agent directory
	// on disk (unit-service-2).
	if names.IsValidUnit(unitName) {
		c.unit = names.NewUnitTag(unitName)
	} else {
		var err error
		c.unit, err = names.ParseUnitTag(unitName)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return cmd.CheckEmpty(args)
}

// Run implements the cmd.Command interface.
func (c *CollectMetricsCommand) Run(ctx *cmd.Context) error {
	paths := uniter.NewWorkerPaths(cmdutil.DataDir, c.unit, "metrics-collect")
	chURL, err := unitercharm.ReadCharmURL(path.Join(paths.GetCharmDir(), unitercharm.CharmURLPath))
	if err != nil {
		return errors.Trace(err)
	}
	if chURL.Schema != "local" {
		return errors.New("not a local charm")
	}

	ch, err := charm.ReadCharm(paths.GetCharmDir())
	if err != nil {
		return errors.Trace(err)
	}
	if ch.Metrics() == nil || len(ch.Metrics().Metrics) == 0 {
		return errors.New("not a metered charm")
	}

	tmpDir, err := ioutil.TempDir("", "collect-metrics")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			logger.Errorf("failed to remove the temporary directory %s: %v", tmpDir, err)
		}
	}()
	metricsFactory := spool.NewFactory(tmpDir)
	err = collect.CollectMetrics(c.unit, cmdutil.DataDir, metricsFactory)
	if err != nil {
		return errors.Trace(err)
	}
	reader, err := metricsFactory.Reader()
	if err != nil {
		return errors.Trace(err)
	}

	batches, err := reader.Read()
	if err != nil {
		return errors.Trace(err)
	}
	data, err := json.Marshal(batches)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctx.Stdout, string(data))
	return nil
}
