// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// A simple command for dumping out the logs stored in
// MongoDB. Intended to be use in emergency situations to recover logs
// when Juju is broken somehow.

package dumplogs

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	jujucmd "github.com/juju/juju/cmd"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	corenames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

// NewCommand returns a new Command instance which implements the
// "juju-dumplogs" command.
func NewCommand() cmd.Command {
	return &dumpLogsCommand{
		agentConfig: jujudagent.NewAgentConf(""),
	}
}

type dumpLogsCommand struct {
	cmd.CommandBase
	agentConfig jujudagent.AgentConf
	machineId   string
	outDir      string
}

// Info implements cmd.Command.
func (c *dumpLogsCommand) Info() *cmd.Info {
	doc := `
This tool can be used to access Juju's logs when the Juju controller
isn't functioning for some reason. It must be run on a Juju controller
server, connecting to the Juju database instance and generating a log
file for each model that exists in the controller.

Log files are written out to the current working directory by
default. Use -d / --output-directory option to specify an alternate
target directory.

In order to connect to the database, the local machine agent's
configuration is needed. In most circumstances the configuration will
be found automatically. The --data-dir and/or --machine-id options may
be required if the agent configuration can't be found automatically.
`[1:]
	return jujucmd.Info(&cmd.Info{
		Name:    corenames.JujuDumpLogs,
		Purpose: "output the logs that are stored in the local Juju database",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *dumpLogsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.agentConfig.AddFlags(f)
	f.StringVar(&c.outDir, "d", ".", "directory to write logs files to")
	f.StringVar(&c.outDir, "output-directory", ".", "")
	f.StringVar(&c.machineId, "machine-id", "", "id of the machine on this host (optional)")
}

// Init implements cmd.Command.
func (c *dumpLogsCommand) Init(args []string) error {
	err := c.agentConfig.CheckArgs(args)
	if err != nil {
		return errors.Trace(err)
	}

	var agentTag names.Tag
	if c.machineId == "" {
		agentTag, err = c.findAgentTag(c.agentConfig.DataDir())
		if err != nil {
			return errors.Trace(err)
		}
	} else if !names.IsValidMachine(c.machineId) {
		return errors.New("--machine-id option expects a non-negative integer")
	} else {
		agentTag = names.NewMachineTag(c.machineId)
	}

	err = c.agentConfig.ReadConfig(agentTag.String())
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Run implements cmd.Command.
func (c *dumpLogsCommand) Run(ctx *cmd.Context) error {
	config := c.agentConfig.CurrentConfig()
	info, ok := config.MongoInfo()
	if !ok {
		return errors.New("no database connection info available (is this a controller host?)")
	}

	session, err := mongo.DialWithInfo(*info, mongo.DefaultDialOpts())
	if err != nil {
		return errors.Trace(err)
	}
	defer session.Close()

	statePool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		MongoSession:       session,
	})
	if err != nil {
		return errors.Annotate(err, "failed to connect to database")
	}
	defer statePool.Close()
	st0 := statePool.SystemState()
	modelUUIDs, err := st0.AllModelUUIDs()
	if err != nil {
		return errors.Annotate(err, "failed to look up models")
	}
	for _, modelUUID := range modelUUIDs {
		err := c.dumpLogsForEnv(ctx, statePool, names.NewModelTag(modelUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to dump logs for model %s", modelUUID)
		}
	}

	return nil
}

func (c *dumpLogsCommand) findAgentTag(dataDir string) (names.Tag, error) {
	entries, err := ioutil.ReadDir(agent.BaseDir(dataDir))
	if err != nil {
		return nil, errors.Annotate(err, "failed to read agent configuration base directory")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			var tag names.Tag
			tag, err = names.ParseMachineTag(entry.Name())
			if err == nil {
				return tag, nil
			}
			tag, err = names.ParseControllerAgentTag(entry.Name())
			if err == nil {
				return tag, nil
			}
		}
	}
	return nil, errors.New("no machine or controller agent configuration found")
}

func (c *dumpLogsCommand) dumpLogsForEnv(ctx *cmd.Context, statePool *state.StatePool, tag names.ModelTag) error {
	st, err := statePool.Get(tag.Id())
	if err != nil {
		if errors.IsNotFound(err) {
			ctx.Infof("model with uuid %v has been removed", tag.Id())
			return nil
		}
		return errors.Annotate(err, "failed open model")
	}
	defer st.Release()

	logName := ctx.AbsPath(filepath.Join(c.outDir, fmt.Sprintf("%s.log", tag.Id())))
	ctx.Infof("writing to %s", logName)

	file, err := os.Create(logName)
	if err != nil {
		return errors.Annotate(err, "failed to open output file")
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	tailer, err := state.NewLogTailer(st, state.LogTailerParams{NoTail: true})
	if err != nil {
		return errors.Annotate(err, "failed to create a log tailer")
	}
	logs := tailer.Logs()
	for {
		rec, ok := <-logs
		if !ok {
			break
		}
		writer.WriteString(c.format(
			rec.Time,
			rec.Level,
			rec.Entity,
			rec.Module,
			rec.Message,
		) + "\n")
	}

	return nil
}

func (c *dumpLogsCommand) format(timestamp time.Time, level loggo.Level, entity, module, message string) string {
	ts := timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s: %s %s %s %s", entity, ts, level, module, message)
}
