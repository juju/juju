// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"text/template"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	_ "launchpad.net/juju-core/provider/all"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

func main() {
	Main(os.Args)
}

func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(&restoreCommand{}, cmd.DefaultContext(), args[1:]))
}

var logger = loggo.GetLogger("juju.plugins.restore")

const restoreDoc = `
Restore restores a backup created with juju backup
by creating a new juju bootstrap instance and arranging
it so that the existing instances in the environment
talk to it.

It verifies that the existing bootstrap instance is
not running. The given constraints will be used
to choose the new instance.
`

type restoreCommand struct {
	cmd.EnvCommandBase
	Log         cmd.Log
	Constraints constraints.Value
	backupFile  string
}

func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-restore",
		Purpose: "Restore a backup made with juju backup",
		Args:    "<backupfile.tar.gz>",
		Doc:     restoreDoc,
	}
}

func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set environment constraints")
	c.Log.AddFlags(f)
}

func (c *restoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no backup file specified")
	}
	c.backupFile = args[0]
	return cmd.CheckEmpty(args[1:])
}

var updateBootstrapMachineTemplate = mustParseTemplate(`
	set -e -x
	tar xzf juju-backup.tgz
	test -d juju-backup

	initctl stop jujud-machine-0

	initctl stop juju-db
	rm -r /var/lib/juju /var/log/juju
	tar -C / -xvp -f juju-backup/root.tar
	mkdir -p /var/lib/juju/db
	export LC_ALL=C
	mongorestore --drop --dbpath /var/lib/juju/db juju-backup/dump
	initctl start juju-db

	mongoEval() {
		mongo --ssl -u admin -p {{.AdminSecret | shquote}} localhost:37017/admin --eval "$1"
	}
	# wait for mongo to come up after starting the juju-db upstart service.
	for i in $(seq 1 60)
	do
		mongoEval ' ' && break
		sleep 2
	done
	mongoEval '
		db = db.getSiblingDB("juju")
		db.machines.update({_id: "0"}, {$set: {instanceid: '{{.NewInstanceId | printf "%q" | shquote}}' } })
		db.instanceData.update({_id: "0"}, {$set: {instanceid: '{{.NewInstanceId | printf "%q"| shquote}}' } })
	'
	initctl start jujud-machine-0
`)

func updateBootstrapMachineScript(instanceId instance.Id, adminSecret string) string {
	return execTemplate(updateBootstrapMachineTemplate, struct {
		NewInstanceId instance.Id
		AdminSecret   string
	}{instanceId, adminSecret})
}

func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if err := c.Log.Start(ctx); err != nil {
		return err
	}
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	cfg, _, err := environs.ConfigForName(c.EnvName, store)
	if err != nil {
		return err
	}
	env, err := rebootstrap(cfg, c.Constraints)
	if err != nil {
		return fmt.Errorf("cannot re-bootstrap environment: %v", err)
	}
	logger.Infof("connecting to newly bootstrapped instance")
	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	if err != nil {
		return fmt.Errorf("cannot connect to bootstrap instance: %v", err)
	}
	logger.Infof("restoring bootstrap machine")
	newInstId, machine0Addr, err := restoreBootstrapMachine(conn, c.backupFile)
	if err != nil {
		return fmt.Errorf("cannot restore bootstrap machine: %v", err)
	}
	logger.Infof("restored bootstrap machine")
	// Update the environ state to point to the new instance.
	if err := bootstrap.SaveState(env.Storage(), &bootstrap.BootstrapState{
		StateInstances: []instance.Id{newInstId},
	}); err != nil {
		return fmt.Errorf("cannot update environ bootstrap state storage: %v", err)
	}
	// Construct our own state info rather than using juju.NewConn so
	// that we can avoid storage eventual-consistency issues
	// (and it's faster too).
	caCert, ok := cfg.CACert()
	if !ok {
		return fmt.Errorf("configuration has no CA certificate")
	}
	logger.Infof("opening state")
	st, err := state.Open(&state.Info{
		Addrs:    []string{fmt.Sprintf("%s:%d", machine0Addr, cfg.StatePort())},
		CACert:   caCert,
		Tag:      "",
		Password: env.Config().AdminSecret(),
	}, state.DefaultDialOpts())
	if err != nil {
		return fmt.Errorf("cannot open state: %v", err)
	}
	logger.Infof("updating all machines")
	if err := updateAllMachines(st, machine0Addr); err != nil {
		return fmt.Errorf("cannot update machines: %v", err)
	}
	return nil
}

func rebootstrap(cfg *config.Config, cons constraints.Value) (environs.Environ, error) {
	logger.Infof("re-bootstrapping environment")
	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	cfg, err := cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot enable provisioner-safe-mode: %v", err)
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	state, err := bootstrap.LoadState(env.Storage())
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve environment storage; perhaps the environment was not bootstrapped: %v", err)
	}
	if len(state.StateInstances) == 0 {
		return nil, fmt.Errorf("no instances found on bootstrap state; perhaps the environment was not bootstrapped")
	}
	if len(state.StateInstances) > 1 {
		return nil, fmt.Errorf("restore does not support HA juju configurations yet")
	}
	inst, err := env.Instances(state.StateInstances)
	if err == nil {
		return nil, fmt.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
	}
	if err != environs.ErrNoInstances {
		return nil, fmt.Errorf("cannot detect whether old instance is still running: %v", err)
	}
	// Remove the storage so that we can bootstrap without the provider complaining.
	if err := env.Storage().Remove(bootstrap.StateFile); err != nil {
		return nil, fmt.Errorf("cannot remove %q from storage: %v", bootstrap.StateFile, err)
	}

	// TODO If we fail beyond here, then we won't have a state file and
	// we won't be able to re-run this script because it fails without it.
	// We could either try to recreate the file if we fail (which is itself
	// error-prone) or we could provide a --no-check flag to make
	// it go ahead anyway without the check.

	if err := bootstrap.Bootstrap(bootstrapContext{}, env, cons); err != nil {
		return nil, fmt.Errorf("cannot bootstrap new instance: %v", err)
	}
	return env, nil
}

func restoreBootstrapMachine(conn *juju.APIConn, backupFile string) (newInstId instance.Id, addr string, err error) {
	addr, err = conn.State.Client().PublicAddress("0")
	if err != nil {
		return "", "", fmt.Errorf("cannot get public address of bootstrap machine: %v", err)
	}
	status, err := conn.State.Client().Status(nil)
	if err != nil {
		return "", "", fmt.Errorf("cannot get environment status: %v", err)
	}
	info, ok := status.Machines["0"]
	if !ok {
		return "", "", fmt.Errorf("cannot find bootstrap machine in status")
	}
	newInstId = instance.Id(info.InstanceId)

	if err := scp(backupFile, addr, "~/juju-backup.tgz"); err != nil {
		return "", "", fmt.Errorf("cannot copy backup file to bootstrap instance: %v", err)
	}

	adminSecret := conn.Environ.Config().AdminSecret()
	if err := ssh(addr, updateBootstrapMachineScript(newInstId, adminSecret)); err != nil {
		return "", "", fmt.Errorf("update script failed: %v", err)
	}
	return newInstId, addr, nil
}

var agentAddressTemplate = mustParseTemplate(`
set -exu
cd /var/lib/juju/agents
for agent in *
do
	initctl stop jujud-$agent
	sed -i.old -r "/^(stateaddresses|apiaddresses):/{
		n
		s/- .*(:[0-9]+)/- {{.Address}}\1/
	}" $agent/agent.conf
	if [[ $agent = unit-* ]]
	then
 		sed -i -r 's/change-version: [0-9]+$/change-version: 0/' $agent/state/relations/*/* || true
	fi
	initctl start jujud-$agent
done
sed -i -r 's/^(:syslogtag, startswith, "juju-" @)(.*)(:[0-9]+.*)$/\1{{.Address}}\3/' /etc/rsyslog.d/*-juju*.conf
`)

// setAgentAddressScript generates an ssh script argument to update state addresses
func setAgentAddressScript(stateAddr string) string {
	return execTemplate(agentAddressTemplate, struct {
		Address string
	}{stateAddr})
}

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
func updateAllMachines(st *state.State, stateAddr string) error {
	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	pendingMachineCount := 0
	done := make(chan error)
	for _, machine := range machines {
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet support by this plugin.
		if machine.IsManager() || machine.Life() == state.Dead {
			continue
		}
		pendingMachineCount++
		machine := machine
		go func() {
			err := runMachineUpdate(machine, setAgentAddressScript(stateAddr))
			if err != nil {
				logger.Errorf("failed to update machine %s: %v", machine, err)
			} else {
				logger.Infof("updated machine %s", machine)
			}
			done <- err
		}()
	}
	err = nil
	for ; pendingMachineCount > 0; pendingMachineCount-- {
		if updateErr := <-done; updateErr != nil && err == nil {
			err = fmt.Errorf("machine update failed")
		}
	}
	return err
}

// runMachineUpdate connects via ssh to the machine and runs the update script
func runMachineUpdate(m *state.Machine, sshArg string) error {
	logger.Infof("updating machine: %v\n", m)
	addr := instance.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return fmt.Errorf("no appropriate public address found")
	}
	return ssh(addr, sshArg)
}

func ssh(addr string, script string) error {
	args := []string{
		"-l", "ubuntu",
		"-T",
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
		addr,
		"sudo -n bash -c " + utils.ShQuote(script),
	}
	cmd := exec.Command("ssh", args...)
	logger.Debugf("ssh command: %s %q", cmd.Path, cmd.Args)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh command failed: %v (%q)", err, data)
	}
	logger.Infof("ssh command succeeded: %q", data)
	return nil
}

func scp(file, host, destFile string) error {
	cmd := exec.Command(
		"scp",
		"-B",
		"-q",
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
		file,
		"ubuntu@"+host+":"+destFile)
	logger.Infof("copying backup file to bootstrap host")
	logger.Debugf("scp command: %s %q", cmd.Path, cmd.Args)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("scp failed: %s", out)
	}
	return err
}

func mustParseTemplate(templ string) *template.Template {
	t := template.New("").Funcs(template.FuncMap{
		"shquote": utils.ShQuote,
	})
	return template.Must(t.Parse(templ))
}

func execTemplate(tmpl *template.Template, data interface{}) string {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, data)
	if err != nil {
		panic(fmt.Errorf("template error: %v", err))
	}
	return buf.String()
}

type bootstrapContext struct{}

func (bootstrapContext) Stdin() io.Reader {
	return os.Stdin
}

func (bootstrapContext) Stdout() io.Writer {
	return os.Stdout
}

func (bootstrapContext) Stderr() io.Writer {
	return os.Stderr
}

func (bootstrapContext) InterruptNotify(c chan<- os.Signal) {
	signal.Notify(c, os.Interrupt)
}

func (bootstrapContext) StopInterruptNotify(c chan<- os.Signal) {
	signal.Stop(c)
}
