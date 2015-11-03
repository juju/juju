// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"text/template"
	"time"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/loggo"
	"github.com/juju/utils"
)

const jujuDbPath = "/var/lib/juju/db"
const jujuDbPathLocal = "~/.juju/local/db"
const jujuAgentPath = "/var/lib/juju/agents/machine-0/agent.conf"
const jujuAgentPathLocal = "~/.juju/local/agents/machine-0/agent.conf"

const systemdfiles = `
DBSERV=$(ls /etc/systemd/system/juju-db*.service | cut -d "/" -f5)
AGENTSERV=$(ls /etc/systemd/system/juju-agent*.service 2>/dev/null | cut -d "/" -f5)
if [ -z $AGENTSERV ]; then
AGENTSERV=$(ls /etc/systemd/system/jujud*.service 2>/dev/null | cut -d "/" -f5)
fi
`

const noauth = `
NoAuth() {
    sed -i "s/--auth/--noauth/" /etc/systemd/system/juju-db*.service;
    sed -i "s,--keyFile '/var/lib/juju/shared-secret',," /etc/systemd/system/juju-db*.service;
    sed -i "s,--keyFile '$HOME/.juju/local/shared-secret',," /etc/systemd/system/juju-db*.service;
}

Auth() {
if [ -f /var/lib/juju/shared-secret ]; then
    sed -i "s,--noauth,--auth --keyFile '/var/lib/juju/shared-secret'," /etc/systemd/system/juju-db*.service;
else
 sed -i "s,--noauth,--auth --keyFile '$HOME/.juju/local/shared-secret'," /etc/systemd/system/juju-db*.service;
fi
}
`

const replset = `
NoReplSet() {
    sed -i "s/--replSet juju//" /etc/systemd/system/juju-db*.service;
}

ReplSet() {
    sed -i "s/mongod /mongod --replSet juju /" /etc/systemd/system/juju-db*.service;
}
`

const upgradeMongoInAgentConfig = `
ReplaceVersion() {
    	sed -i "s/mongoversion:.*/mongoversion: \"${1}\"/" {{.JujuAgentPath}}
}

AddVersion () {
    echo "mongoversion: \"${1}\"" >> {{.JujuAgentPath}}
}

UpgradeMongoVersion() {
    VERSIONKEY=$(grep mongoversion {{.JujuAgentPath}})
    if [ -n "$VERSIONKEY" ]; then
	ReplaceVersion $1;
    else
	AddVersion $1;
    fi
}
`

const upgradeMongoBinary = `
UpgradeMongoBinary() {
    sed -i "s/juju\/bin/juju\/mongo${1}\/bin/" /etc/systemd/system/juju-db*.service;
    sed -i "s/juju\/mongo.*\/bin/juju\/mongo${1}\/bin/" /etc/systemd/system/juju-db*.service;
    if [ "$2" == "storage" ]; then
	sed -i "s/--smallfiles//" /etc/systemd/system/juju-db*.service;
	sed -i "s/--noprealloc/--storageEngine wiredTiger/" /etc/systemd/system/juju-db*.service;
    fi
}
`

const mongoEval = `
mongoAdminEval() {
        echo "will run as admin: $@"
        attempts=0
        until [ $attempts -ge 5 ]
        do
            echo "printjson($@)" > /tmp/mongoAdminEval.js
    	    mongo --ssl -u admin -p {{.OldPassword | shquote}} localhost:{{.StatePort}}/admin /tmp/mongoAdminEval.js && break
            echo "attempt $attempts"
            attempts=$[$attempts+1]
            sleep 10
        done
        rm /tmp/mongoAdminEval.js
        if [ $attempts -eq 5 ]; then
            exit 1
        fi
}

mongoAnonEval() {
        echo "will run as anon: $@"
        attempts=0
        until [ $attempts -ge 5 ]
        do
            echo "printjson($@)" > /tmp/mongoAnonEval.js
    	    mongo --ssl  localhost:{{.StatePort}}/admin /tmp/mongoAnonEval.js  && break
            echo "attempt $attempts"
            attempts=$[$attempts+1]
            sleep 10
        done
        rm /tmp/mongoAnonEval.js
        if [ $attempts -eq 5 ]; then
            exit 1
        fi
}

rsConf() {
RSCONF=$(mongo --ssl --quiet -u admin -p {{.OldPassword | shquote}} localhost:{{.StatePort}}/admin --eval "printjson(rs.conf())"|tr -d '\n')
}
`

func (c *upgradeMongoCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.local, "local", false, "this is a local provider")
	c.Log.AddFlags(f)
}

func main() {
	Main(os.Args)
}

// Main is the entry point for this plugins.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(envcmd.Wrap(&upgradeMongoCommand{}), ctx, args[1:]))
}

const upgradeDoc = `This command upgrades the state server
mongo db from 2.4 to 3.`

var logger = loggo.GetLogger("juju.plugins.upgrademongo")

type upgradeMongoCommand struct {
	envcmd.EnvCommandBase
	Log   cmd.Log
	local bool
}

func (c *upgradeMongoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-upgrade-mongo",
		Purpose: "Upgrade from mongo 2.4 to 3.1",
		Args:    "",
		Doc:     upgradeDoc,
	}
}

func mustParseTemplate(templ string) *template.Template {
	t := template.New("").Funcs(template.FuncMap{
		"shquote": utils.ShQuote,
	})
	return template.Must(t.Parse(templ))
}

// runViaJujuSSH will run arbitrary code in the remote machine.
func runViaJujuSSH(machine, script string, stdout, stderr *bytes.Buffer) error {
	functions := upgradeMongoInAgentConfig + upgradeMongoBinary + mongoEval
	script = functions + script
	tmpl := mustParseTemplate(script)
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, nil)
	if err != nil {
		panic(errors.Annotate(err, "template error"))
	}
	cmd := exec.Command("juju", []string{"ssh", machine, "sudo -n bash -c " + utils.ShQuote(buf.String())}...)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	err = cmd.Run()
	if err != nil {
		fmt.Println("\nErr:")
		fmt.Println(fmt.Sprintf("%s", stderr.String()))
		fmt.Println("\nOut:")
		fmt.Println(fmt.Sprintf("%s", stdout.String()))
		return errors.Annotatef(err, "ssh command failed: (%q)", stderr.String())
	}
	//progress("ssh command succedded: %q", fmt.Sprintf("%s", stdout.String()))
	return nil
}

func bufferPrinter(stdout *bytes.Buffer, closer chan int, verbose bool) {
	for {
		select {
		case <-closer:
			return
		case <-time.After(500 * time.Millisecond):

		}
		line, err := stdout.ReadString(byte('\n'))
		if err == nil || err == io.EOF {
			fmt.Print(line)
		}
		if err != nil && err != io.EOF {
			return
		}

	}
}

// We try to stop/start juju with both systems, safer than
// try a convoluted discovery in bash.
const jujuUpgradeScript = `
sudo initctl stop jujud-machine-{{.MachineNumber}}
sudo systemctl stop jujud-machine-{{.MachineNumber}}
/var/lib/juju/tools/machine-{{.MachineNumber}}/jujud --series {{.Series}} --machinetag {{.MachineNumber}} --configfile /var/lib/juju/agents/machine-{{.MachineNumber}}/agent.conf
sudo initctl start jujud-machine-{{.MachineNumber}}
sudo systemctl start jujud-machine-{{.MachineNumber}}
`

type upgradeScriptParams struct {
	MachineNumber int
	Series        string
}

func (c *upgradeMongoCommand) Run(ctx *cmd.Context) error {
	if err := c.Log.Start(ctx); err != nil {
		return err
	}

	var stdout bytes.Buffer
	var closer chan int
	closer = make(chan int, 1)
	defer func() { closer <- 1 }()
	go bufferPrinter(&stdout, closer, false)

	return nil
}

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}
