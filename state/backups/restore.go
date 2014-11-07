// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows !linux

package backups

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/tar"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/goyaml"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/peergrouper"
)

// resetReplicaSet re-initiates replica-set using the new state server
// values, this is required after a mongo restore.
// In case of failure returns error.
func resetReplicaSet(dialInfo *mgo.DialInfo, memberHostPort string) error {
	params := peergrouper.InitiateMongoParams{dialInfo,
		memberHostPort,
		dialInfo.Username,
		dialInfo.Password,
	}

	return peergrouper.InitiateMongoServer(params, true)
}

var filesystemRoot = getFilesystemRoot

func getFilesystemRoot() string {
	return "/"
}

// newDialInfo returns mgo.DialInfo with the given address using the minimal
// possible setup.
func newDialInfo(privateAddr string, conf agentConfig) (*mgo.DialInfo, error) {
	dialOpts := mongo.DialOpts{Direct: true}
	info := mongo.Info{
		Addrs:  []string{fmt.Sprintf("%s:%s", privateAddr, conf.statePort)},
		CACert: conf.cACert,
	}
	dialInfo, err := mongo.DialInfo(info, dialOpts)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce a dial info")
	}
	dialInfo.Username = conf.credentials.adminUsername
	dialInfo.Password = conf.credentials.adminPassword
	return dialInfo, nil
}

// updateMongoEntries will update the machine entries in the restored mongo to
// reflect the real machine instanceid in case it changed (a newly bootstraped
// server).
func updateMongoEntries(newInstId instance.Id, dialInfo *mgo.DialInfo) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return errors.Annotate(err, "cannot connect to mongo to update")
	}
	defer session.Close()
	if err := session.DB("juju").C("machines").Update(bson.M{"machineid": "0"}, bson.M{"$set": bson.M{"instanceid": string(newInstId)}}); err != nil {
		return errors.Annotate(err, "cannot update machine 0 instance information")
	}
	return nil
}

// credentials for mongo in backed up agent.conf.
type credentials struct {
	tag           string
	tagPassword   string
	adminUsername string
	adminPassword string
}

// agentConfig config from the backed up agent.conf.
type agentConfig struct {
	credentials credentials
	apiPort     string
	statePort   string
	cACert      string
}

// fetchAgentConfigFromBackup parses <dataDir>/machine-0/agents/machine-0/agent.conf
// and returns an agentConfig struct filled with the data that will not change
// from the backed up one (typically everything but the hosts).
func fetchAgentConfigFromBackup(innerBackupHandler io.Reader) (agentConfig, error) {
	// TODO (perrito666) obtain this pat path from the proper place here and in backup
	// if the backup contains the series of the machine we can generate it.
	_, agentConf, err := tar.FindFile(innerBackupHandler, "var/lib/juju/agents/machine-0/agent.conf")
	if err != nil {
		return agentConfig{}, errors.Annotatef(err, "could not find agent configuration in tar file")
	}

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
		return agentConfig{}, errors.Errorf("config file unmarshalled to %T not %T", conf, m)
	}

	tagUser, ok := m["tag"].(string)
	if !ok || tagUser == "" {
		return agentConfig{}, errors.Errorf("tag not found in configuration")
	}

	tagPassword, ok := m["statepassword"].(string)
	if !ok || tagPassword == "" {
		return agentConfig{}, errors.Errorf("agent tag user password not found in configuration")
	}

	adminPassword, ok := m["oldpassword"].(string)
	if !ok || adminPassword == "" {
		return agentConfig{}, errors.Errorf("agent admin password not found in configuration")
	}

	statePortNum, ok := m["stateport"].(int)
	if !ok {
		return agentConfig{}, errors.Errorf("state port not found in configuration")
	}
	statePort := strconv.Itoa(statePortNum)

	apiPortNum, ok := m["apiport"].(int)
	if !ok {
		return agentConfig{}, errors.Errorf("api port not found in configuration")
	}
	apiPort := strconv.Itoa(apiPortNum)

	cacert, ok := m["cacert"].(string)
	if !ok {
		return agentConfig{}, errors.Errorf("CACert not found in configuration")
	}

	return agentConfig{
		credentials: credentials{
			tag:           tagUser,
			tagPassword:   tagPassword,
			adminUsername: "admin",
			adminPassword: adminPassword,
		},
		statePort: statePort,
		apiPort:   apiPort,
		cACert:    cacert,
	}, nil
}

// newStateConnection tries to connect to the newly restored state server.
func newStateConnection(agentConf agentConfig) (*state.State, error) {
	caCert := agentConf.cACert
	// TODO(dfc) agenConf.credentials should supply a Tag
	tag, err := names.ParseTag(agentConf.credentials.tag)
	if err != nil {
		return nil, errors.Annotate(err, "cannot obtain tag from agent config")
	}
	// We need to retry here to allow mongo to come up on the restored state server.
	// The connection might succeed due to the mongo dial retries but there may still
	// be a problem issuing database commands.
	var st *state.State
	attempt := utils.AttemptStrategy{Delay: 15 * time.Second, Min: 8}
	for a := attempt.Start(); a.Next(); {
		st, err = state.Open(&mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{fmt.Sprintf("localhost:%s", agentConf.statePort)},
				CACert: caCert,
			},
			Tag:      tag,
			Password: agentConf.credentials.tagPassword,
		}, mongo.DefaultDialOpts(), environs.NewStatePolicy())
		if err == nil {
			break
		}
		logger.Errorf("cannot open state, retrying: %v", err)
	}

	return st, errors.Annotate(err, "cannot open state")

}

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
func updateAllMachines(privateAddress string, agentConf agentConfig, st *state.State) error {
	privateHostPorts := fmt.Sprintf("%s:%s", privateAddress, agentConf.statePort)
	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	pendingMachineCount := 0
	done := make(chan error)
	for key := range machines {
		// key is used to have machine be scope bound to the loop iteration.
		machine := machines[key]
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet supported by this code.
		if machine.IsManager() || machine.Life() == state.Dead {
			continue
		}
		pendingMachineCount++

		go func() {
			err := runMachineUpdate(machine, setAgentAddressScript(privateHostPorts))
			done <- errors.Annotatef(err, "failed to update machine %s", machine)
		}()
	}
	err = nil
	for ; pendingMachineCount > 0; pendingMachineCount-- {
		if updateErr := <-done; updateErr != nil && err == nil {
			err = errors.Annotate(updateErr, "machine update failed")
		}
	}
	// error is annotated in the above iteration.
	return err
}

func mustParseTemplate(templ string) *template.Template {
	return template.Must(template.New("").Parse(templ))
}

// agentAddressTemplate is the template used to replace the api server data
// in the agents for the new ones if the machine has been rebootstraped.
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

func execTemplate(tmpl *template.Template, data interface{}) string {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, data)
	if err != nil {
		panic(errors.Annotate(err, "template error"))
	}
	return buf.String()
}

// setAgentAddressScript generates an ssh script argument to update state addresses.
func setAgentAddressScript(stateAddr string) string {
	return execTemplate(agentAddressTemplate, struct {
		Address string
	}{stateAddr})
}

// runMachineUpdate connects via ssh to the machine and runs the update script.
func runMachineUpdate(m *state.Machine, sshArg string) error {
	addr := network.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return errors.Errorf("no appropriate public address found")
	}
	return runViaSSH(addr, sshArg)
}

// runViaSSH runs script in the remote machine with address addr.
func runViaSSH(addr string, script string) error {
	// This is taken from cmd/juju/ssh.go there is no other clear way to set user
	userAddr := "ubuntu@" + addr
	sshOptions := ssh.Options{}
	sshOptions.SetIdentities("/var/lib/juju/system-identity")
	userCmd := ssh.Command(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, &sshOptions)
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	userCmd.Stderr = &stderrBuf
	userCmd.Stdout = &stdoutBuf
	err := userCmd.Run()
	if err != nil {
		return errors.Annotatef(err, "ssh command failed: %q", stderrBuf.String())
	}
	return nil
}

// backupVersion will use information from the backup file and metadata (if available)
// to determine which backup version this file belongs to.
// Once Metadata is given a version option we can version backups
// we could use juju version to signal this, but currently:
// Version 0: juju backup plugin (a bash script)
// Version 1: juju backups create (first implementation) for the
// moment this version is determined by checking for metadata but not
// its contents.
func backupVersion(backupMetadata *metadata.Metadata, backupFilesPath string) (int, error) {
	backupMetadataFile := true
	if _, err := os.Stat(filepath.Join(backupFilesPath, "metadata.json")); os.IsNotExist(err) {
		backupMetadataFile = false
	} else if err != nil {
		return 0, errors.Annotate(err, "cannot read metadata file")
	}
	if backupMetadata == nil && !backupMetadataFile {
		return 0, nil
	}
	return 1, nil
}
