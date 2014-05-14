// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"text/template"

	"github.com/juju/loggo"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
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
	"launchpad.net/juju-core/utils/ssh"
)

func main() {
	Main(os.Args)
}

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
	os.Exit(cmd.Main(envcmd.Wrap(&restoreCommand{}), ctx, args[1:]))
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
	envcmd.EnvCommandBase
	Log             cmd.Log
	Constraints     constraints.Value
	backupFile      string
	showDescription bool
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
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set environment constraints")
	f.BoolVar(&c.showDescription, "description", false, "show the purpose of this plugin")
	c.Log.AddFlags(f)
}

func (c *restoreCommand) Init(args []string) error {
	if c.showDescription {
		return cmd.CheckEmpty(args)
	}
	if len(args) == 0 {
		return fmt.Errorf("no backup file specified")
	}
	c.backupFile = args[0]
	return cmd.CheckEmpty(args[1:])
}

var updateBootstrapMachineTemplate = mustParseTemplate(`
	set -exu

	export LC_ALL=C
	tar xzf juju-backup.tgz
	test -d juju-backup
	apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::options::=--force-unsafe-io --assume-yes --quiet install mongodb-clients
	
	initctl stop jujud-machine-0

	initctl stop juju-db
	rm -r /var/lib/juju
	rm -r /var/log/juju

	tar -C / -xvp -f juju-backup/root.tar
	mkdir -p /var/lib/juju/db

	# Prefer jujud-mongodb binaries if available 
	export MONGORESTORE=mongorestore
	if [ -f /usr/lib/juju/bin/mongorestore ]; then
		export MONGORESTORE=/usr/lib/juju/bin/mongorestore;
	fi	
	$MONGORESTORE --drop --dbpath /var/lib/juju/db juju-backup/dump

	initctl start juju-db

	mongoAdminEval() {
		mongo --ssl -u admin -p {{.Creds.OldPassword | shquote}} localhost:37017/admin --eval "$1"
	}


	mongoEval() {
		mongo --ssl -u {{.Creds.Tag}} -p {{.Creds.Password | shquote}} localhost:37017/juju --eval "$1"
	}

	# wait for mongo to come up after starting the juju-db upstart service.
	for i in $(seq 1 100)
	do
		mongoEval ' ' && break
		sleep 5
	done

	mongoEval '
		db = db.getSiblingDB("juju")
		db.machines.update({_id: "0"}, {$set: {instanceid: {{.NewInstanceId | printf "%q" }} } })
		db.instanceData.update({_id: "0"}, {$set: {instanceid: {{.NewInstanceId | printf "%q" }} } })
	'
	
	# Create a new replicaSet conf and re initiate it
	mongoAdminEval '
		conf = { "_id" : "juju", "version" : 1, "members" : [ { "_id" : 1, "host" : "{{ .PrivateAddress | printf "%s:37017" }}" , "tags" : { "juju-machine-id" : "0" } }]}
		rs.initiate(conf)
	'

	# Give time to replset to initiate
	for i in $(seq 1 20)
	do
		mongoEval ' ' && break
		sleep 5
	done

	initctl stop juju-db

	# Update the agent.conf for machine-0 with the new addresses
	cd /var/lib/juju/agents
	sed -i.old -r -e "/^(stateaddresses):/{
		n
		s/- .*(:[0-9]+)/- {{.Address}}\1/
	}" -e "/^(apiaddresses):/{
		n
		s/- .*(:[0-9]+)/- {{.PrivateAddress}}\1/
	}"  machine-0/agent.conf

	initctl start juju-db
	initctl start jujud-machine-0
`)

func updateBootstrapMachineScript(instanceId instance.Id, creds credentials, addr, paddr string) string {
	return execTemplate(updateBootstrapMachineTemplate, struct {
		NewInstanceId  instance.Id
		Creds          credentials
		Address        string
		PrivateAddress string
	}{instanceId, creds, addr, paddr})
}

func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.showDescription {
		fmt.Fprintf(ctx.Stdout, "%s\n", c.Info().Purpose)
		return nil
	}
	if err := c.Log.Start(ctx); err != nil {
		return err
	}
	creds, err := extractCreds(c.backupFile)
	if err != nil {
		return fmt.Errorf("cannot extract credentials from backup file: %v", err)
	}
	progress("extracted credentials from backup file")
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	cfg, _, err := environs.ConfigForName(c.EnvName, store)
	if err != nil {
		return err
	}
	env, err := rebootstrap(cfg, ctx, c.Constraints)
	if err != nil {
		return fmt.Errorf("cannot re-bootstrap environment: %v", err)
	}
	progress("connecting to newly bootstrapped instance")
	conn, err := juju.NewAPIConn(env, api.DefaultDialOpts())
	if err != nil {
		return fmt.Errorf("cannot connect to bootstrap instance: %v", err)
	}
	progress("restoring bootstrap machine")
	newInstId, machine0Addr, err := restoreBootstrapMachine(conn, c.backupFile, creds)
	if err != nil {
		return fmt.Errorf("cannot restore bootstrap machine: %v", err)
	}
	progress("restored bootstrap machine")
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
	progress("opening state")
	st, err := state.Open(&state.Info{
		Addrs:    []string{fmt.Sprintf("%s:%d", machine0Addr, cfg.StatePort())},
		CACert:   caCert,
		Tag:      creds.Tag,
		Password: creds.Password,
	}, state.DefaultDialOpts(), environs.NewStatePolicy())
	if err != nil {
		return fmt.Errorf("cannot open state: %v", err)
	}
	progress("updating all machines")
	if err := updateAllMachines(st, machine0Addr); err != nil {
		return fmt.Errorf("cannot update machines: %v", err)
	}
	return nil
}

func progress(f string, a ...interface{}) {
	fmt.Printf("%s\n", fmt.Sprintf(f, a...))
}

func rebootstrap(cfg *config.Config, ctx *cmd.Context, cons constraints.Value) (environs.Environ, error) {
	progress("re-bootstrapping environment")
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

	args := environs.BootstrapParams{Constraints: cons}
	if err := bootstrap.Bootstrap(ctx, env, args); err != nil {
		return nil, fmt.Errorf("cannot bootstrap new instance: %v", err)
	}
	return env, nil
}

func restoreBootstrapMachine(conn *juju.APIConn, backupFile string, creds credentials) (newInstId instance.Id, addr string, err error) {
	client := conn.State.Client()
	addr, err = client.PublicAddress("0")
	if err != nil {
		return "", "", fmt.Errorf("cannot get public address of bootstrap machine: %v", err)
	}
	paddr, err := client.PrivateAddress("0")
	if err != nil {
		return "", "", fmt.Errorf("cannot get private address of bootstrap machine: %v", err)
	}
	status, err := client.Status(nil)
	if err != nil {
		return "", "", fmt.Errorf("cannot get environment status: %v", err)
	}
	info, ok := status.Machines["0"]
	if !ok {
		return "", "", fmt.Errorf("cannot find bootstrap machine in status")
	}
	newInstId = instance.Id(info.InstanceId)

	progress("copying backup file to bootstrap host")
	if err := sendViaScp(backupFile, addr, "~/juju-backup.tgz"); err != nil {
		return "", "", fmt.Errorf("cannot copy backup file to bootstrap instance: %v", err)
	}
	progress("updating bootstrap machine")
	if err := runViaSsh(addr, updateBootstrapMachineScript(newInstId, creds, addr, paddr)); err != nil {
		return "", "", fmt.Errorf("update script failed: %v", err)
	}
	return newInstId, addr, nil
}

type credentials struct {
	Tag         string
	Password    string
	OldPassword string
}

func extractCreds(backupFile string) (credentials, error) {
	f, err := os.Open(backupFile)
	if err != nil {
		return credentials{}, err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return credentials{}, fmt.Errorf("cannot unzip %q: %v", backupFile, err)
	}
	defer gzr.Close()
	outerTar, err := findFileInTar(gzr, "juju-backup/root.tar")
	if err != nil {
		return credentials{}, err
	}
	agentConf, err := findFileInTar(outerTar, "var/lib/juju/agents/machine-0/agent.conf")
	if err != nil {
		return credentials{}, err
	}
	data, err := ioutil.ReadAll(agentConf)
	if err != nil {
		return credentials{}, fmt.Errorf("failed to read agent config file: %v", err)
	}
	var conf interface{}
	if err := goyaml.Unmarshal(data, &conf); err != nil {
		return credentials{}, fmt.Errorf("cannot unmarshal agent config file: %v", err)
	}
	m, ok := conf.(map[interface{}]interface{})
	if !ok {
		return credentials{}, fmt.Errorf("config file unmarshalled to %T not %T", conf, m)
	}
	password, ok := m["statepassword"].(string)
	if !ok || password == "" {
		return credentials{}, fmt.Errorf("agent password not found in configuration")
	}
	oldPassword, ok := m["oldpassword"].(string)
	if !ok || oldPassword == "" {
		return credentials{}, fmt.Errorf("agent old password not found in configuration")
	}

	return credentials{
		Tag:         "machine-0",
		Password:    password,
		OldPassword: oldPassword,
	}, nil
}

func findFileInTar(r io.Reader, name string) (io.Reader, error) {
	tarr := tar.NewReader(r)
	for {
		hdr, err := tarr.Next()
		if err != nil {
			return nil, fmt.Errorf("%q not found: %v", name, err)
		}
		if path.Clean(hdr.Name) == name {
			return tarr, nil
		}
	}
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

	# If we're processing a unit agent's directly
	# and it has some relations, reset
	# the stored version of all of them to
	# ensure that any relation hooks will
	# fire.
	if [[ $agent = unit-* ]]
	then
		find $agent/state/relations -type f -exec sed -i -r 's/change-version: [0-9]+$/change-version: 0/' {} \;
	fi
	initctl start jujud-$agent
done
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
				progress("updated machine %s", machine)
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
	progress("updating machine: %v\n", m)
	addr := instance.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return fmt.Errorf("no appropriate public address found")
	}
	return runViaSsh(addr, sshArg)
}

func runViaSsh(addr string, script string) error {
	// This is taken from cmd/juju/ssh.go there is no other clear way to set user
	userAddr := "ubuntu@" + addr
	cmd := ssh.Command(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, nil)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ssh command failed: %v (%q)", err, stderrBuf.String())
	}
	progress("ssh command succedded: %q", stdoutBuf.String())
	return nil
}

func sendViaScp(file, host, destFile string) error {
	err := ssh.Copy([]string{file, "ubuntu@" + host + ":" + destFile}, nil)
	if err != nil {
		return fmt.Errorf("scp command failed: %v", err)
	}
	return nil
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
