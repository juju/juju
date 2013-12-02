// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	_ "launchpad.net/juju-core/provider/all"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.plugins.restore")

const restoreDoc = `
Restore restores a backup created with juju backup
by creating a new juju bootstrap instances and arranging
it so that the existing instances in the environment
talk to it.

It verifies that the existing bootstrap instance is
not running. The given constraints will be used
to choose the new instance.
`

type restoreCommand struct {
	cmd.EnvCommandBase
	Constraints constraints.Value
	backupFile string
}

func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-restore",
		Purpose: "Restore a backup made with juju backup",
		Args: "<backupfile.tar.gz>",
		Doc:     restoreDoc,
	}
}

func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "set environment constraints")
}

func (c *restoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no backup file specified")
	}
	c.backupFile = args[0]
}

var updateBootstrapMachine = template.Must(template.New("").Parse(`
	set -e -x
	tar xzf juju-backup.tgz
	test -d juju-backup
	initctl stop juju-db
	initctl stop jujud-machine-0
	cd /
	rm -r /var/lib/juju /var/log/juju
	tar -C juju-backup/root -x -f - | tar -C / -xpz -f -
	initctl start juju-db
	mongo --ssl -u admin -p newAdminSecret localhost:37017/admin
		db.machines.update({_id: 0}, {$set: {instanceid: {{.NewInstanceId}} } })
		db.instanceData.update({_id: 0}, {$set: {instanceid: {{.NewInstanceId}} } })
	initctl start jujud-machine-0
`))


func (c *restoreCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	cfg, err := environs.ConfigForName(c.EnvName, store)
	if err != nil {
		return err
	}
	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	cfg, err = cfg.Apply(map[string]interface{} {
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return fmt.Errorf("cannot enable provisioner-safe-mode: %v", err)
	}
	env, err := environs.New(cfg)
	if err != nil {
		return err
	}
	state, err := bootstrap.LoadState(env.Storage())
	if err != nil {
		return fmt.Errorf("cannot retrieve environment storage; perhaps the environment was not bootstrapped: %v", err)
	}
	if len(state.StateInstances) == 0 {
		return fmt.Errorf("no instances found on bootstrap state; perhaps the environment was not bootstrapped", err)
	}
	if len(state.StateInstances) > 1 {
		return fmt.Errorf("restore does not support HA juju configurations yet")
	}
	inst, err := env.Instances(state.StateInstances)
	if err == nil {
		return fmt.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
	}
	if err != environs.ErrNoInstances {
		return fmt.Errorf("cannot detect whether old instance is still running: %v", err)
	}
	// Remove the storage so that we can bootstrap without the provider complaining.
	if err := env.Storage.Remove(bootstrap.StateFile); err != nil {
		return fmt.Errorf("cannot remove %q from storage: %v", bootstrap.StateFile, err)
	}
	
	// TODO If we fail beyond here, then we won't have a state file and
	// we won't be able to re-run this script because it fails without it.
	// We could either try to recreate the file if we fail (which is itself
	// error-prone) or we could provide a --no-check flag to make
	// it go ahead anyway without the check.

	if err := bootstrap.Bootstrap(env, c.Constraints); err != nil {
		return fmt.Errorf("cannot bootstrap new instance: %v", err)
	}
	logger.Printf("connecting to new instance")
	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	if err != nil {
		return fmt.Errorf("cannot connect to bootstrap instance: %v")
	}
	addr, err := conn.State.Client().PublicAddress("machine-0")
	if err != nil {
		return fmt.Errorf("cannot get public address of bootstrap machine: %v", err)
	}
	if err := scp(c.backupFile, addr, "~/juju-backup.tgz"); err != nil {
		return fmt.Errorf("cannot copy backup file to bootstrap instance: %v", err)
	}
	if err := ssh(addr, 
	ssh to machine 0
	
	script to run on machine 0, as root:
	
		initctl stop juju-db
		initctl stop jujud-machine-0
		cd /
		rm -r /var/lib/juju /var/log/juju
		tar xpzf /tmp/juju-backup.tgz
		initctl start juju-db
		mongo --ssl -u admin -p newAdminSecret localhost:37017/admin
			db.machines.update({_id: 0}, {$set: {instanceid: newInstanceId}})
			db.instanceData.update({_id: 0}, {$set: {instanceid: newInstanceId}})
			# initctl start jujud-machine-0

	updateAllMachines()
}

func scp(file, host, destFile string) error {
	cmd := exec.Command("scp", "-B", "-q", file, host + ":" + destFile)
	logger.Printf("copying backup file to bootstrap host")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("scp failed: %s", out)
	}
	return err
}


func (c *restoreCommand) updateAllMachines() {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	stateAddr, err := GetStateAddress(conn.Environ)
	if err != nil {
		return err
	}
	fmt.Printf("using state address %v\n", stateAddr)
	return updateAllMachines(conn, stateAddr)
}

// GetStateAddress returns the address of one state server
func GetStateAddress(environ environs.Environ) (string, error) {
	// XXX: Can easily look up state server address using api instead
	stateInfo, _, err := environ.StateInfo()
	if err != nil {
		return "", err
	}
	return strings.Split(stateInfo.Addrs[0], ":")[0], nil
}

var agentAddressTemplate = `
set -exu
cd /var/lib/juju/agents
for agent in *
do
	initctl stop jujud-$agent
	sed -i.old -r "/^(stateaddresses|apiaddresses):/{
		n
		s/- .*(:[0-9]+)/- $ADDR\1/
	}" $agent/agent.conf
	if [[ $agent = unit-* ]]
	then
		sed -i -r 's/change-version: [0-9]+$/change-version: 0/' $agent/state/relations/*/*
	fi
	initctl start jujud-$agent
done
sed -i -r 's/^(:syslogtag, startswith, "juju-" @)(.*)(:[0-9]+.*)$/\1'$ADDR'\3/' /etc/rsyslog.d/*-juju*.conf
`

// renderScriptArg generates an ssh script argument to update state addresses
func renderScriptArg(stateAddr string) string {
	script := strings.Replace(agentAddressTemplate, "$ADDR", stateAddr, -1)
	return "sudo bash -c " + utils.ShQuote(script)
}

// runMachineUpdate connects via ssh to the machine and runs the update script
func runMachineUpdate(m *state.Machine, sshArg string) error {
	logger.Infof("updating machine: %v\n", m)
	addr := instance.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return fmt.Errorf("no appropriate public address found")
	}
	args := []string{
		"-l", "ubuntu",
		"-T",
		"-o", "StrictHostKeyChecking no",
		"-o", "PasswordAuthentication no",
		addr,
		sshArg,
	}
	c := exec.Command("ssh", args...)
	if data, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh command failed: %v (%q)", err, data)
	}
	return nil
}

// updateAllMachines finds all machines resets the stored state address
func updateAllMachines(conn *juju.Conn, stateAddr string) error {
	machines, err := conn.State.AllMachines()
	if err != nil {
		return err
	}
	pendingMachineCount := 0
	done := make(chan error)
	for _, machine := range machines {
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet support by this plugin.
		if machine.IsManager() || machine.Life() != state.Alive {
			continue
		}
		pendingMachineCount += 1
		machine := machine
		go func() {
			err := runMachineUpdate(machine, renderScriptArg(stateAddr))
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

func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	command := restoreCommand{}
	os.Exit(cmd.Main(&command, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
