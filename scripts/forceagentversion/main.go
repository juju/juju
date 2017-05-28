package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/cmd/jujud/agent"
)

var logger = loggo.GetLogger("forceagentversion")

func setupLogging(logLevel loggo.Level) error {
	writer := loggo.NewSimpleWriter(os.Stderr, logFormatter)
	loggo.ReplaceDefaultWriter(writer)
	return loggo.ConfigureLoggers(fmt.Sprintf("<root>=%s", logLevel.String()))
}

func logFormatter(entry loggo.Entry) string {
	ts := entry.Timestamp.In(time.UTC).Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s %s", ts, entry.Message)

}

type commandLineArgs struct {
	port			int
	modelUUID      string
	rawVersion     string
	version        version.Number
	rawLogLevel    string
	logLevel       loggo.Level
}

func commandLine() commandLineArgs {
	flags := flag.NewFlagSet("mgopurge", flag.ExitOnError)
	var a commandLineArgs
	flags.StringVar(&a.modelUUID, "uuid", "",
		"UUID of the Model to update")
	flags.StringVar(&a.rawVersion, "version", "",
		"version to upgrade to")
	flags.IntVar(&a.port, "port", 37017,
		"mongo port to connect to")
	flags.StringVar(&a.rawLogLevel, "log-level", "TRACE",
		"log level to use (TRACE/DEBUG/INFO/etc)")

	flags.Parse(os.Args[1:])

	targetVersion := version.MustParse(a.rawVersion)
	a.version = targetVersion
	a.logLevel, _ = loggo.ParseLevel(a.rawLogLevel)

	return a
}

func checkErr(label string, err error) {
	if err != nil {
		logger.Errorf("%s: %s", label, err)
		os.Exit(1)
	}
}

const jujuDataDir = "/var/lib/juju"
const jujuAgentDir = jujuDataDir + "/agents/"

func main() {
	args := commandLine()
	checkErr("setup logging", setupLogging(args.logLevel))
	// Check to see who we might be
	matches, err := filepath.Glob(jujuAgentDir + "machine-*")
	checkErr("finding machine agent", err)
	if len(matches) == 0 {
		logger.Errorf("no machines in %q", jujuAgentDir)
		os.Exit(1)
	} else if len(matches) > 1 {
		logger.Errorf("too many machines in %q: %v", jujuAgentDir, matches)
		os.Exit(1)
	}
	agentTag := matches[0][len(jujuAgentDir):]
	agentConf := agent.NewAgentConf(jujuDataDir)
	checkErr("read config", agentConf.ReadConfig(agentTag))
	conf := agentConf.CurrentConfig()
	mongoInfo, available := conf.MongoInfo()
	if !available {
		logger.Errorf("must be run from a controller machine, no mongo info available")
		os.Exit(1)
	}

	openParams := state.OpenParams{
		Clock: clock.WallClock,
		MongoInfo: mongoInfo,
		MongoDialOpts: mongo.DialOpts{
			Timeout:       time.Second,
			SocketTimeout: time.Second,
			Direct:        false,
		},
		ControllerTag:      conf.Controller(),
		ControllerModelTag: conf.Model(),
	}
	st, err := state.Open(openParams)
	checkErr("open state", err)
	modelSt, err := st.ForModel(names.NewModelTag(args.modelUUID))
	checkErr("open model", err)
	checkErr("set model agent version", modelSt.SetModelAgentVersion(args.version))
}
