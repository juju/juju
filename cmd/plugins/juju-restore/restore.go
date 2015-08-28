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
	"strconv"
	"text/template"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/utils/ssh"
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


	initctl stop jujud-machine-0

	#The code apt-get throws when lock is taken
	APTOUTPUT=100 
	while [ $APTOUTPUT -gt 0 ]
	do
		# We will try to run apt-get and it can fail if other dpkg is in use
		# the subshell call is not reached by -e so we can have apt-get fail
		APTOUTPUT=$(apt-get --option=Dpkg::Options::=--force-confold --option=Dpkg::options::=--force-unsafe-io --assume-yes --quiet install mongodb-clients &> /dev/null; echo $?)
		if [ $APTOUTPUT -gt 0 ] && [ $APTOUTPUT -ne 100 ]; then
			echo "apt-get failed with an irrecoverable error $APTOUTPUT";
			exit 1
		fi
	done
	


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
		mongo --ssl -u admin -p {{.AgentConfig.Credentials.OldPassword | shquote}} localhost:{{.AgentConfig.StatePort}}/admin --eval "$1"
	}

	# wait for mongo to come up after starting the juju-db init service.
	for i in $(seq 1 100)
	do
		mongoAdminEval ' ' && break
		sleep 5
	done

	# Create a new replicaSet conf and re initiate it
	mongoAdminEval '
		conf = { "_id" : "juju", "version" : 1, "members" : [ { "_id" : 1, "host" : "{{ .PrivateAddress | printf "%s:"}}{{.AgentConfig.StatePort}}" , "tags" : { "juju-machine-id" : "0" } }]}
		rs.initiate(conf)
	'
	# This looks arbitrary but there is no clear way to determine when replicaset is initiated
	# and rs.initiate message is "this will take about a minute" so we honour that estimation
	sleep 60

	# Remove all state machines but 0, to restore HA
	mongoAdminEval '
		db = db.getSiblingDB("juju")
		db.machines.update({machineid: "0"}, {$set: {instanceid: {{.NewInstanceId | printf "%q" }} } })
		db.machines.update({machineid: "0"}, {$set: {"addresses": ["{{.Address}}"] } })
		db.instanceData.update({_id: "0"}, {$set: {instanceid: {{.NewInstanceId | printf "%q" }} } })
		db.machines.remove({machineid: {$ne:"0"}, hasvote: true})
		db.stateServers.update({"_id":"e"}, {$set:{"machineids" : ["0"]}})
		db.stateServers.update({"_id":"e"}, {$set:{"votingmachineids" : ["0"]}})
	'

	# Give time to replset to initiate
	for i in $(seq 1 20)
	do
		mongoAdminEval ' ' && break
		sleep 5
	done

	initctl stop juju-db

	# Update the agent.conf for machine-0 with the new addresses
	cd /var/lib/juju/agents

	# Remove extra state machines from conf
	REMOVECOUNT=$(grep -Ec "^-.*{{.AgentConfig.ApiPort}}$" /var/lib/juju/agents/machine-0/agent.conf )
	awk '/\-.*{{.AgentConfig.ApiPort}}$/{i++}i<1' machine-0/agent.conf > machine-0/agent.conf.new
	awk -v removecount=$REMOVECOUNT '/\-.*{{.AgentConfig.ApiPort}}$/{i++}i==removecount' machine-0/agent.conf >> machine-0/agent.conf.new
	mv machine-0/agent.conf.new  machine-0/agent.conf

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

func updateBootstrapMachineScript(instanceId instance.Id, agentConf agentConfig, addr, paddr string) string {
	return execTemplate(updateBootstrapMachineTemplate, struct {
		NewInstanceId  instance.Id
		AgentConfig    agentConfig
		Address        string
		PrivateAddress string
	}{instanceId, agentConf, addr, paddr})
}

func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.showDescription {
		fmt.Fprintf(ctx.Stdout, "%s\n", c.Info().Purpose)
		return nil
	}
	if err := c.Log.Start(ctx); err != nil {
		return err
	}
	agentConf, err := extractConfig(c.backupFile)
	if err != nil {
		return errors.Annotate(err, "cannot extract configuration from backup file")
	}
	progress("extracted credentials from backup file")
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	cfg, err := c.Config(store, nil)
	if err != nil {
		return err
	}
	env, err := rebootstrap(cfg, ctx, c.Constraints)
	if err != nil {
		return errors.Annotate(err, "cannot re-bootstrap environment")
	}
	progress("connecting to newly bootstrapped instance")
	var apiState api.Connection
	// The state server backend may not be ready to accept logins so we retry.
	// We'll do up to 8 retries over 2 minutes to give the server time to come up.
	// Typically we expect only 1 retry will be needed.
	attempt := utils.AttemptStrategy{Delay: 15 * time.Second, Min: 8}
	// While specifying the admin user will work for now, as soon as we allow
	// the users to have a different initial user name, or they have changed
	// the password for the admin user, this will fail.
	owner := names.NewUserTag("admin")
	for a := attempt.Start(); a.Next(); {
		apiState, err = juju.NewAPIState(owner, env, api.DefaultDialOpts())
		if err == nil || errors.Cause(err).Error() != "EOF" {
			break
		}
		progress("bootstrapped instance not ready - attempting to redial")
	}
	if err != nil {
		return errors.Annotate(err, "cannot connect to bootstrap instance")
	}
	progress("restoring bootstrap machine")
	machine0Addr, err := restoreBootstrapMachine(apiState, c.backupFile, agentConf)
	if err != nil {
		return errors.Annotate(err, "cannot restore bootstrap machine")
	}
	progress("restored bootstrap machine")

	apiState, err = juju.NewAPIState(owner, env, api.DefaultDialOpts())
	progress("opening state")
	if err != nil {
		return errors.Annotate(err, "cannot connect to api server")
	}
	progress("updating all machines")
	results, err := updateAllMachines(apiState, machine0Addr)
	if err != nil {
		return errors.Annotate(err, "cannot update machines")
	}
	var message string
	for _, result := range results {
		if result.err != nil {
			message = fmt.Sprintf("Update of machine %q failed: %v", result.machineName, result.err)
		} else {
			message = fmt.Sprintf("Succesful update of machine %q", result.machineName)
		}
		progress(message)
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
		return nil, errors.Annotate(err, "cannot enable provisioner-safe-mode")
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	instanceIds, err := env.StateServerInstances()
	switch errors.Cause(err) {
	case nil, environs.ErrNoInstances:
		// Some providers will return a nil error even
		// if there are no live state server instances.
		break
	case environs.ErrNotBootstrapped:
		return nil, errors.Trace(err)
	default:
		return nil, errors.Annotate(err, "cannot determine state server instances")
	}
	if len(instanceIds) > 0 {
		instances, err := env.Instances(instanceIds)
		switch errors.Cause(err) {
		case nil, environs.ErrPartialInstances:
			return nil, fmt.Errorf("old bootstrap instances %q still seems to exist; will not replace", instances)
		case environs.ErrNoInstances:
			// No state server instances, so keep running.
			break
		default:
			return nil, errors.Annotate(err, "cannot detect whether old instance is still running")
		}
	}
	// Remove the storage so that we can bootstrap without the provider complaining.
	if env, ok := env.(environs.EnvironStorage); ok {
		if err := env.Storage().Remove(common.StateFile); err != nil {
			return nil, errors.Annotate(err, fmt.Sprintf("cannot remove %q from storage", common.StateFile))
		}
	}

	// TODO If we fail beyond here, then we won't have a state file and
	// we won't be able to re-run this script because it fails without it.
	// We could either try to recreate the file if we fail (which is itself
	// error-prone) or we could provide a --no-check flag to make
	// it go ahead anyway without the check.

	args := bootstrap.BootstrapParams{Constraints: cons}
	if err := bootstrap.Bootstrap(envcmd.BootstrapContextNoVerify(ctx), env, args); err != nil {
		return nil, errors.Annotate(err, "cannot bootstrap new instance")
	}
	return env, nil
}

func restoreBootstrapMachine(st api.Connection, backupFile string, agentConf agentConfig) (addr string, err error) {
	client := st.Client()
	addr, err = client.PublicAddress("0")
	if err != nil {
		return "", errors.Annotate(err, "cannot get public address of bootstrap machine")
	}
	paddr, err := client.PrivateAddress("0")
	if err != nil {
		return "", errors.Annotate(err, "cannot get private address of bootstrap machine")
	}
	status, err := client.Status(nil)
	if err != nil {
		return "", errors.Annotate(err, "cannot get environment status")
	}
	info, ok := status.Machines["0"]
	if !ok {
		return "", fmt.Errorf("cannot find bootstrap machine in status")
	}
	newInstId := instance.Id(info.InstanceId)

	progress("copying backup file to bootstrap host")
	if err := sendViaScp(backupFile, addr, "~/juju-backup.tgz"); err != nil {
		return "", errors.Annotate(err, "cannot copy backup file to bootstrap instance")
	}
	progress("updating bootstrap machine")
	if err := runViaSsh(addr, updateBootstrapMachineScript(newInstId, agentConf, addr, paddr)); err != nil {
		return "", errors.Annotate(err, "update script failed")
	}
	return addr, nil
}

type credentials struct {
	Tag         string
	Password    string
	OldPassword string
}

type agentConfig struct {
	Credentials credentials
	ApiPort     string
	StatePort   string
}

func extractMachineID(archive *os.File) (string, error) {
	paths := backups.NewCanonicalArchivePaths()

	gzr, err := gzip.NewReader(archive)
	if err != nil {
		return "", errors.Annotate(err, fmt.Sprintf("cannot unzip %q", archive.Name()))
	}
	defer gzr.Close()

	metaFile, err := findFileInTar(gzr, paths.MetadataFile)
	if errors.IsNotFound(err) {
		// Older archives don't have a metadata file and always have machine-0.
		return "0", nil
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	meta, err := backups.NewMetadataJSONReader(metaFile)
	if err != nil {
		return "", errors.Trace(err)
	}
	return meta.Origin.Machine, nil
}

func extractConfig(backupFile string) (agentConfig, error) {
	f, err := os.Open(backupFile)
	if err != nil {
		return agentConfig{}, err
	}
	defer f.Close()

	// Extract the machine tag.
	machineID, err := extractMachineID(f)
	if err != nil {
		return agentConfig{}, err
	}
	_, err = f.Seek(0, os.SEEK_SET)
	if err != nil {
		return agentConfig{}, err
	}
	tag := names.NewMachineTag(machineID)

	// Extract the config file.
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return agentConfig{}, errors.Annotate(err, fmt.Sprintf("cannot unzip %q", backupFile))
	}
	defer gzr.Close()
	outerTar, err := findFileInTar(gzr, "juju-backup/root.tar")
	if err != nil {
		return agentConfig{}, err
	}
	// TODO(ericsnowcurrently) This should come from an authoritative source.
	const confFilename = "var/lib/juju/agents/%s/agent.conf"
	agentConf, err := findFileInTar(outerTar, fmt.Sprintf(confFilename, tag))
	if err != nil {
		return agentConfig{}, err
	}

	// Extract the config data.
	data, err := ioutil.ReadAll(agentConf)
	if err != nil {
		return agentConfig{}, errors.Annotate(err, "failed to read agent config file")
	}
	var conf interface{}
	if err := goyaml.Unmarshal(data, &conf); err != nil {
		return agentConfig{}, errors.Annotate(err, "cannot unmarshal agent config file")
	}
	m, ok := conf.(map[interface{}]interface{})
	if !ok {
		return agentConfig{}, fmt.Errorf("config file unmarshalled to %T not %T", conf, m)
	}
	password, ok := m["statepassword"].(string)
	if !ok || password == "" {
		return agentConfig{}, fmt.Errorf("agent password not found in configuration")
	}
	oldPassword, ok := m["oldpassword"].(string)
	if !ok || oldPassword == "" {
		return agentConfig{}, fmt.Errorf("agent old password not found in configuration")
	}
	statePortNum, ok := m["stateport"].(int)
	if !ok {
		return agentConfig{}, fmt.Errorf("state port not found in configuration")
	}

	statePort := strconv.Itoa(statePortNum)
	apiPortNum, ok := m["apiport"].(int)
	if !ok {
		return agentConfig{}, fmt.Errorf("api port not found in configuration")
	}
	apiPort := strconv.Itoa(apiPortNum)

	return agentConfig{
		Credentials: credentials{
			Tag:         "machine-0",
			Password:    password,
			OldPassword: oldPassword,
		},
		StatePort: statePort,
		ApiPort:   apiPort,
	}, nil
}

func findFileInTar(r io.Reader, name string) (io.Reader, error) {
	tarr := tar.NewReader(r)
	for {
		hdr, err := tarr.Next()
		if err == io.EOF {
			return nil, errors.NotFoundf(name)
		}
		if err != nil {
			return nil, errors.Annotatef(err, "while looking for %q", name)
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

type restoreResult struct {
	machineName string
	err         error
}

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
func updateAllMachines(apiState api.Connection, stateAddr string) ([]restoreResult, error) {
	client := apiState.Client()
	status, err := client.Status(nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get status")
	}
	pendingMachineCount := 0
	done := make(chan restoreResult)

	for _, machineStatus := range status.Machines {
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet support by this plugin.
		if machineStatus.HasVote || machineStatus.WantsVote || machineStatus.Life == "dead" {
			continue
		}
		pendingMachineCount++
		machine := machineStatus
		go func() {
			err := runMachineUpdate(client, machine.Id, setAgentAddressScript(stateAddr))
			if err != nil {

				logger.Errorf("failed to update machine %s: %v", machine.Id, err)
			} else {
				progress("updated machine %s", machine.Id)
			}
			r := restoreResult{machineName: machine.Id, err: err}
			done <- r
		}()
	}
	results := make([]restoreResult, pendingMachineCount)
	for ; pendingMachineCount > 0; pendingMachineCount-- {
		results[pendingMachineCount-1] = <-done
	}
	return results, nil
}

// runMachineUpdate connects via ssh to the machine and runs the update script
func runMachineUpdate(client *api.Client, id string, sshArg string) error {
	progress("updating machine: %v\n", id)
	addr, err := client.PublicAddress(id)
	if err != nil {
		return errors.Annotate(err, "no public address found")
	}
	return runViaSsh(addr, sshArg)
}

func runViaSsh(addr string, script string) error {
	// This is taken from cmd/juju/ssh.go there is no other clear way to set user
	userAddr := "ubuntu@" + addr
	userCmd := ssh.Command(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, nil)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	userCmd.Stderr = &stderrBuf
	userCmd.Stdout = &stdoutBuf
	err := userCmd.Run()
	if err != nil {
		return errors.Annotate(err, fmt.Sprintf("ssh command failed: (%q)", stderrBuf.String()))
	}
	progress("ssh command succedded: %q", stdoutBuf.String())
	return nil
}

func sendViaScp(file, host, destFile string) error {
	err := ssh.Copy([]string{file, "ubuntu@" + host + ":" + destFile}, nil)
	if err != nil {
		return errors.Annotate(err, "scp command failed")
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
		panic(errors.Annotate(err, "template error"))
	}
	return buf.String()
}
