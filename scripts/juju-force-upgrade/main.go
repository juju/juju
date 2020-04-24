// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/version"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	jversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.forceupgrade")

func checkErr(label string, err error) {
	if err != nil {
		logger.Errorf("%s: %s", label, err)
		os.Exit(1)
	}
}

const dataDir = "/var/lib/juju"

func getState() (*state.StatePool, error) {
	tag, err := getCurrentMachineTag(dataDir)
	if err != nil {
		return nil, errors.Annotate(err, "finding machine tag")
	}

	logger.Infof("current machine tag: %s", tag)

	config, err := getConfig(tag)
	if err != nil {
		return nil, errors.Annotate(err, "loading agent config")
	}

	mongoInfo, available := config.MongoInfo()
	if !available {
		return nil, errors.New("mongo info not available from agent config")
	}
	session, err := mongo.DialWithInfo(*mongoInfo, mongo.DefaultDialOpts())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer session.Close()

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		MongoSession:       session,
	})
	if err != nil {
		return nil, errors.Annotate(err, "opening state connection")
	}
	return pool, nil
}

func getCurrentMachineTag(datadir string) (names.MachineTag, error) {
	var empty names.MachineTag
	values, err := filepath.Glob(filepath.Join(datadir, "agents", "machine-*"))
	if err != nil {
		return empty, errors.Annotate(err, "problem globbing")
	}
	switch len(values) {
	case 0:
		return empty, errors.Errorf("no machines found")
	case 1:
		return names.ParseMachineTag(filepath.Base(values[0]))
	default:
		return empty, errors.Errorf("too many options: %v", values)
	}
}

func getConfig(tag names.MachineTag) (agent.ConfigSetterWriter, error) {
	path := agent.ConfigPath("/var/lib/juju", tag)
	return agent.ReadConfig(path)
}

func main() {
	loggo.GetLogger("").SetLogLevel(loggo.TRACE)
	gnuflag.Usage = func() {
		fmt.Printf("Usage: %s <model-uuid> <version>\n", os.Args[0])
		os.Exit(1)
	}

	gnuflag.Parse(true)

	args := gnuflag.Args()
	if len(args) < 2 {
		gnuflag.Usage()
	}

	modelUUID := args[0]
	agentVersion := version.MustParse(args[1])
	if agentVersion.Compare(jversion.Current) < 0 {
		// Force the client to think it is at least as new as the desired version
		jversion.Current = agentVersion
	}

	statePool, err := getState()
	checkErr("getting state connection", err)
	defer statePool.Close()

	modelSt, err := statePool.Get(modelUUID)
	checkErr("open model", err)
	defer func() {
		modelSt.Release()
		statePool.Remove(modelUUID)
	}()

	checkErr("set model agent version", modelSt.SetModelAgentVersion(agentVersion, true))
}
