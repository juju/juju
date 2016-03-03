// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
)

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
		fmt.Fprintf(os.Stderr, "could not obtain context for command: %v\n", err)
		os.Exit(2)
	}
	if err := juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(modelcmd.Wrap(&upgradeMongoCommand{}), ctx, args[1:]))
}

const upgradeDoc = `This command upgrades the version of mongo used to store the Juju model from 2.4 to 3.x`

var logger = loggo.GetLogger("juju.plugins.upgrademongo")

// MongoUpgradeClient defines the methods
// on the client api that mongo upgrade will call.
type MongoUpgradeClient interface {
	Close() error
	MongoUpgradeMode(mongo.Version) (params.MongoUpgradeResults, error)
	ResumeHAReplicationAfterUpgrade([]replicaset.Member) error
}

type upgradeMongoCommand struct {
	modelcmd.ModelCommandBase
	Log      cmd.Log
	local    bool
	haClient MongoUpgradeClient
}

func (c *upgradeMongoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-upgrade-database",
		Purpose: "Upgrade from mongo 2.4 to 3.x",
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
	cmd := exec.Command("ssh", []string{"-o StrictHostKeyChecking=no", fmt.Sprintf("ubuntu@%s", machine), "sudo -n bash -c " + utils.ShQuote(script)}...)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	err := cmd.Run()
	if err != nil {
		return errors.Annotatef(err, "ssh command failed: (%q)", stderr.String())
	}
	return nil
}

// bufferPrinter is intended to print the output of a remote script
// in real time.
// the intention behind this is to provide the user with continuous
// feedback while waiting a remote process that might take some time.
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

const (
	jujuUpgradeScript = `
/var/lib/juju/tools/machine-{{.MachineNumber}}/jujud upgrade-mongo --series {{.Series}} --machinetag 'machine-{{.MachineNumber}}'
`
	jujuUpgradeScriptMembers = `
/var/lib/juju/tools/machine-{{.MachineNumber}}/jujud upgrade-mongo --series {{.Series}} --machinetag 'machine-{{.MachineNumber}}' --members '{{.Members}}'
`
	jujuSlaveUpgradeScript = `
/var/lib/juju/tools/machine-{{.MachineNumber}}/jujud upgrade-mongo --series {{.Series}} --machinetag 'machine-{{.MachineNumber}}' --slave
`
)

type upgradeScriptParams struct {
	MachineNumber string
	Series        string
	Members       string
}

func (c *upgradeMongoCommand) Run(ctx *cmd.Context) error {
	if err := c.Log.Start(ctx); err != nil {
		return err
	}

	migratables, err := c.migratableMachines()
	if err != nil {
		return errors.Annotate(err, "cannot determine status servers")
	}

	addrs := make([]string, len(migratables.rsMembers))
	for i, rsm := range migratables.rsMembers {
		addrs[i] = rsm.Address
	}
	var members string
	if len(addrs) > 0 {
		members = strings.Join(addrs, ",")
	}

	var stdout, stderr bytes.Buffer
	var closer chan int
	closer = make(chan int, 1)
	defer func() { closer <- 1 }()
	go bufferPrinter(&stdout, closer, false)

	t := template.New("").Funcs(template.FuncMap{
		"shquote": utils.ShQuote,
	})
	var tmpl *template.Template
	if members == "" {
		tmpl = template.Must(t.Parse(jujuUpgradeScript))
	} else {
		tmpl = template.Must(t.Parse(jujuUpgradeScriptMembers))
	}
	var buf bytes.Buffer
	upgradeParams := upgradeScriptParams{
		migratables.master.machine.Id(),
		migratables.master.series,
		members,
	}
	if err = tmpl.Execute(&buf, upgradeParams); err != nil {
		return errors.Annotate(err, "cannot build a script to perform the remote upgrade")
	}

	if err := runViaJujuSSH(migratables.master.ip.Value, buf.String(), &stdout, &stderr); err != nil {
		return errors.Annotate(err, "migration to mongo 3 unsuccesful, your database is left in the same state.")
	}
	ts := template.New("")
	tmpl = template.Must(ts.Parse(jujuSlaveUpgradeScript))
	for _, m := range migratables.machines {
		if m.ip.Value == migratables.master.ip.Value {
			continue
		}
		var buf bytes.Buffer
		upgradeParams := upgradeScriptParams{
			m.machine.Id(),
			m.series,
			"",
		}
		if err := tmpl.Execute(&buf, upgradeParams); err != nil {
			return errors.Annotate(err, "cannot build a script to perform the remote upgrade")
		}
		if err := runViaJujuSSH(m.ip.Value, buf.String(), &stdout, &stderr); err != nil {
			return errors.Annotatef(err, "cannot migrate slave machine on %q", m.ip.Value)
		}
	}
	return nil
}

type migratable struct {
	machine names.MachineTag
	ip      network.Address
	result  int
	series  string
}

type upgradeMongoParams struct {
	master    migratable
	machines  []migratable
	rsMembers []replicaset.Member
}

func (c *upgradeMongoCommand) getHAClient() (MongoUpgradeClient, error) {
	if c.haClient != nil {
		return c.haClient, nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get API connection")
	}

	// NewClient does not return an error, so we'll return nil
	return highavailability.NewClient(root), nil
}

func (c *upgradeMongoCommand) migratableMachines() (upgradeMongoParams, error) {
	haClient, err := c.getHAClient()
	if err != nil {
		return upgradeMongoParams{}, err
	}

	defer haClient.Close()
	results, err := haClient.MongoUpgradeMode(mongo.Mongo30wt)
	if err != nil {
		return upgradeMongoParams{}, errors.Annotate(err, "cannot enter mongo upgrade mode")
	}
	result := upgradeMongoParams{}

	result.master = migratable{
		ip:      results.Master.PublicAddress,
		machine: names.NewMachineTag(results.Master.Tag),
		series:  results.Master.Series,
	}
	result.machines = make([]migratable, len(results.Members))
	for i, member := range results.Members {
		result.machines[i] = migratable{
			ip:      member.PublicAddress,
			machine: names.NewMachineTag(member.Tag),
			series:  member.Series,
		}
	}
	result.rsMembers = make([]replicaset.Member, len(results.RsMembers))
	for i, rsMember := range results.RsMembers {
		result.rsMembers[i] = rsMember
	}

	return result, nil
}

// waitForNotified will wait for all ha members to be notified
// of the impending migration or timeout.
func waitForNotified(addrs []string) error {
	return nil
}

// stopAllMongos stops all the mongo slaves to prevent them
// from falling back when we upgrade the master.
func stopAllMongos(addrs []string) error {
	return nil
}

// recreateReplicas creates replica slaves again from the
// upgraded mongo master.
func recreateReplicas(master string, addrs []string) error {
	return nil
}
