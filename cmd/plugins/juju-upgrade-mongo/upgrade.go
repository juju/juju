// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"launchpad.net/gnuflag"

	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/utils/ssh"
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

const upgradeDoc = ``

var logger = loggo.GetLogger("juju.plugins.upgrademongo")

type upgradeMongoCommand struct {
	envcmd.EnvCommandBase
	Log   cmd.Log
	local bool
}

func (c *upgradeMongoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-upgradei-mongo",
		Purpose: "Upgrade from mongo 2.4 to 3.1",
		Args:    "",
		Doc:     upgradeDoc,
	}
}

func progress(f string, a ...interface{}) {
	fmt.Printf("%s\n", fmt.Sprintf(f, a...))
}

func mustParseTemplate(templ string) *template.Template {
	t := template.New("").Funcs(template.FuncMap{
		"shquote": utils.ShQuote,
	})
	return template.Must(t.Parse(templ))
}

type sshParams struct {
	OldPassword   string
	StatePort     int
	JujuDbPath    string
	JujuAgentPath string
}

// runViaSSH will run arbitrary code in the remote machine.
func runViaSSH(addr, script string, params sshParams, stderr, stdout *bytes.Buffer, verbose bool, local bool) error {
	userAddr := "ubuntu@" + addr
	params.JujuDbPath = jujuDbPath
	params.JujuAgentPath = jujuAgentPath
	if local {
		params.JujuDbPath = jujuDbPathLocal
		params.JujuAgentPath = jujuAgentPathLocal
	}
	functions := systemdfiles + upgradeMongoInAgentConfig + upgradeMongoBinary + mongoEval + replset + noauth
	var callable string
	if verbose {
		callable = `set -u
`
	}
	callable += functions + script
	tmpl := mustParseTemplate(callable)
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, params)
	if err != nil {
		panic(errors.Annotate(err, "template error"))
	}

	if local {
		userCmd := exec.Command("sudo", []string{"-n", "bash", "-c", buf.String()}...)
		userCmd.Stderr = stderr
		userCmd.Stdout = stdout
		err = userCmd.Run()
	} else {
		userCmd := ssh.Command(userAddr, []string{"-o", "UserKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=no", "sudo", "-n", "bash", "-c " + utils.ShQuote(buf.String())}, nil)
		userCmd.Stderr = stderr
		userCmd.Stdout = stdout
		err = userCmd.Run()
	}
	if err != nil {
		fmt.Println("\nErr: ")
		fmt.Printf("%s", fmt.Sprintf("%s", stderr.String()))
		fmt.Println("\nOut: ")
		fmt.Printf("%s", fmt.Sprintf("%s", stdout.String()))
		return errors.Annotatef(err, "ssh command failed: See above")
	}
	//fmt.Printf("%s", fmt.Sprintf("%s", stdout.String()))
	//progress("ssh command succedded: %s", "see above")
	return nil
}

// runViaJujuSSH will run arbitrary code in the remote machine.
func runViaJujuSSH(machine, script string, params sshParams, stdout, stderr *bytes.Buffer) error {
	params.JujuDbPath = jujuDbPath
	params.JujuAgentPath = jujuAgentPath

	functions := upgradeMongoInAgentConfig + upgradeMongoBinary + mongoEval
	script = functions + script
	tmpl := mustParseTemplate(script)
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, params)
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

func addPPA(addr string, local bool, stdout *bytes.Buffer) error {
	var stderrBuf bytes.Buffer
	// beware, juju-mongodb3 only works in vivid.
	addPPACommand := `echo "preparing environment for mongo 3"
apt-add-repository -y ppa:hduran-8/juju-mongodb2.6
apt-add-repository -y ppa:hduran-8/juju-mongodb3
apt-get update
apt-get install juju-mongodb2.6 juju-mongodb3
apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::options::=--force-unsafe-io --assume-yes --quiet install mongodb-clients`
	return runViaSSH(addr, addPPACommand, sshParams{}, &stderrBuf, stdout, true, local)
}

func upgradeTo26(addr, password string, port int, local bool, stdout *bytes.Buffer) error {
	var stderrBuf bytes.Buffer
	upgradeTo26Command := `/usr/lib/juju/bin/mongodump --ssl -u admin -p {{.OldPassword | shquote}} --port {{.StatePort}} --out ~/migrateTo26dump
echo "dumped mongo"

systemctl stop $AGENTSERV
echo "stoped juju"

systemctl stop $DBSERV
echo "stoped mongo"

UpgradeMongoVersion 2.6
echo "upgraded to 2.6 in conf"

UpgradeMongoBinary 2.6 only
echo "upgraded to 2.6 in systemd"

systemctl daemon-reload
echo "realoaded systemd"

/usr/lib/juju/mongo2.6/bin/mongod --dbpath {{.JujuDbPath}} --replSet juju --upgrade
echo "upgraded mongo 2.6"

systemctl start $DBSERV
echo "starting mongodb 2.6"
echo "waiting for mongo to come online"

sleep 120
mongoAdminEval 'db.getSiblingDB("admin").runCommand({authSchemaUpgrade: 1 })'
echo "upgraded auth schema."

systemctl restart $DBSERV
echo "waiting for mongo to come online"
sleep 60

`
	return runViaSSH(addr, upgradeTo26Command, sshParams{OldPassword: password, StatePort: port}, &stderrBuf, stdout, true, local)
}

func upgradeTo3(addr, password string, port int, local bool, stdout *bytes.Buffer) error {
	var stderrBuf bytes.Buffer
	upgradeTo3Command := `attempts=0
until [ $attempts -ge 60 ]
do
/usr/lib/juju/mongo2.6/bin/mongodump --ssl -u admin -p {{.OldPassword | shquote}} --port {{.StatePort}} --out ~/migrateTo3dump&& break
    echo "attempt $attempts"
    attempts=$[$attempts+1]
    sleep 10
done
if [ $attempts -eq 60 ]; then
            exit 1
fi
echo "dumped for migration to 3"

rsConf
echo "rs.config $RSCONF"

systemctl stop $AGENTSERV
echo "stopped juju"

systemctl stop $DBSERV
echo "stopped mongo"

UpgradeMongoVersion 3.1
echo "upgrade version in agent.conf"

UpgradeMongoBinary 3 only
echo "upgrade systemctl call"

systemctl daemon-reload
echo "reload systemctl"

systemctl start $DBSERV
echo "start mongo 3 without wt"

echo "will wait"
attempts=0
until [ $attempts -ge 30 ]
do
    /usr/lib/juju/mongo3/bin/mongodump --ssl -u admin -p {{.OldPassword | shquote}} --port {{.StatePort}} --out ~/migrateToTigerDump  && break
    echo "attempt $attempts"
    attempts=$[$attempts+1]
    sleep 10
done
if [ $attempts -eq 30 ]; then
            exit 1
fi
echo "perform migration dump"

systemctl stop $DBSERV
echo "stopped mongo"

UpgradeMongoBinary 3 storage
NoAuth
NoReplSet
echo "upgrade mongo including storage"

systemctl daemon-reload
echo "reload systemctl"

mv {{.JujuDbPath}} {{.JujuDbPath}}.old
echo "move db"

mkdir {{.JujuDbPath}}
echo "create new db"

systemctl start $DBSERV
echo "start mongo"

rsconfcommand="rs.initiate($RSCONF)"
mongoAnonEval $rsconfcommand

echo "initiated replicaset"
echo "waiting for replicaset to come online"
sleep 60

/usr/lib/juju/mongo3/bin/mongorestore -vvvvv --ssl --sslAllowInvalidCertificates --port {{.StatePort}} --host localhost  --db=juju ~/migrateToTigerDump/juju
/usr/lib/juju/mongo3/bin/mongorestore -vvvvv --ssl --sslAllowInvalidCertificates --port {{.StatePort}} --host localhost  --db=admin ~/migrateToTigerDump/admin
/usr/lib/juju/mongo3/bin/mongorestore -vvvvv --ssl --sslAllowInvalidCertificates --port {{.StatePort}} --host localhost  --db=logs ~/migrateToTigerDump/logs
/usr/lib/juju/mongo3/bin/mongorestore -vvvvv --ssl --sslAllowInvalidCertificates --port {{.StatePort}} --host localhost  --db=presence ~/migrateToTigerDump/presence
#Ignore blobstorage for now, it is causing a bug, most likely corruption?
echo "restored backup to wt"

systemctl stop $DBSERV
echo "stop mongo"

Auth
ReplSet
systemctl daemon-reload
echo "reload systemctl"

systemctl start $DBSERV
echo "start mongo"

systemctl start $AGENTSERV
echo "start juju"

systemctl start $DBSERV
echo "start mongo"
echo "waiting mongo to come online"
`
	return runViaSSH(addr, upgradeTo3Command, sshParams{OldPassword: password, StatePort: port}, &stderrBuf, stdout, true, local)
}

func ensureRunningServices(addr string, local bool, stdout *bytes.Buffer) error {
	var stderrBuf bytes.Buffer
	command := `
sleep 60
systemctl start $DBSERV
echo "mongo started"
`
	return runViaSSH(addr, command, sshParams{}, &stderrBuf, stdout, true, local)
}

func (c *upgradeMongoCommand) agentConfig(addr string, local bool) (agent.Config, error) {

	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	var err error
	if local {
		err = runViaSSH(addr, "cat ~/.juju/local/agents/machine-0/agent.conf", sshParams{}, &stderrBuf, &stdoutBuf, false, true)
	} else {
		err = runViaJujuSSH(addr, "cat /var/lib/juju/agents/machine-0/agent.conf", sshParams{}, &stdoutBuf, &stderrBuf)
		if err != nil {
			return nil, errors.Annotate(err, "cannot obtain agent config")
		}
	}
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, errors.Annotate(err, "cannot write temporary file for agent config")
	}
	defer os.Remove(f.Name())
	rawAgent := stdoutBuf.String()
	index := strings.Index(rawAgent, "# format")
	if index > 0 {
		rawAgent = rawAgent[index:]
	}

	_, err = f.Write([]byte(rawAgent))
	if err != nil {
		return nil, errors.Annotate(err, "cannot write config in temporary file")
	}
	return agent.ReadConfig(f.Name())
}

func externalIPFromStatus() (string, error) {
	var stderr, stdout bytes.Buffer
	cmd := exec.Command("juju", "status")
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return "", errors.Annotate(err, "cannot get juju status")
	}

	var status map[string]interface{}
	err = goyaml.Unmarshal(stdout.Bytes(), &status)
	if err != nil {
		return "", errors.Annotate(err, "cannot unmarshall status")
	}
	machines := status["machines"].(map[interface{}]interface{})
	machine0 := machines["0"].(map[interface{}]interface{})
	dnsname := machine0["dns-name"].(string)
	return dnsname, nil
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

func (c *upgradeMongoCommand) Run(ctx *cmd.Context) error {
	if err := c.Log.Start(ctx); err != nil {
		return err
	}
	if c.local && !checkIfRoot() {
		fmt.Println("will be called as root")
		fullpath, err := exec.LookPath("juju-upgrade-mongo")
		if err != nil {
			return err
		}

		sudoArgs := []string{"--preserve-env", fullpath}
		sudoArgs = append(sudoArgs, "--local")

		command := exec.Command("sudo", sudoArgs...)
		// Now hook up stdin, stdout, stderr
		command.Stdin = ctx.Stdin
		command.Stdout = ctx.Stdout
		command.Stderr = ctx.Stderr
		// And run it!
		return command.Run()
	}
	var stdout bytes.Buffer
	var closer chan int
	closer = make(chan int, 1)
	defer func() { closer <- 1 }()
	go bufferPrinter(&stdout, closer, false)
	dnsname, err := externalIPFromStatus()
	if err != nil {
		return errors.Annotate(err, "cannot determine api addresses")
	}
	addr := dnsname
	config, err := c.agentConfig("0", c.local)
	if err != nil {
		return errors.Annotate(err, "cannot determine agent config")
	}

	err = addPPA(addr, c.local, &stdout)
	if err != nil {
		return errors.Annotate(err, "cannot add mongo 2.6 and 3 ppas")
	}
	info, _ := config.StateServingInfo()
	err = upgradeTo26(addr, config.OldPassword(), info.StatePort, c.local, &stdout)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade to 2.6")
	}

	err = upgradeTo3(addr, config.OldPassword(), info.StatePort, c.local, &stdout)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade to 3")
	}
	err = ensureRunningServices(addr, c.local, &stdout)
	if err != nil {
		return errors.Annotate(err, "cannot ensure services")
	}

	return nil
}

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}
