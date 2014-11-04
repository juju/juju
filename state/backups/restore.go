// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows !linux

package backups

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/tar"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/goyaml"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/peergrouper"
)

var logger = loggo.GetLogger("juju.state.backups")

var runCommand = runExternalCommand

// runExternalCommand will run the external comand cmd with args arguments and return nil on success
// fails or the command output if it fails
func runExternalCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()

	if err == nil {
		return nil
	}

	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return errors.Annotatef(err, "error executing %q: %s", cmd, strings.Replace(string(out), "\n", "; ", -1))
	}
	return errors.Annotatef(err, "cannot execute %q", cmd)
}

// untarFiles will take the reader and output folder for a tar file, wrap in a gzip
// reader if uncompression is required and call utils/tar UntarFiles
func untarFiles(tarFile io.ReadCloser, outputFolder string, compress bool) error {
	var r io.Reader = tarFile
	var err error
	if compress {
		r, err = gzip.NewReader(r)
		if err != nil {
			return errors.Annotatef(err, "cannot uncompress tar file %q", tarFile)
		}
	}

	return tar.UntarFiles(r, outputFolder)
}

// resetReplicaSet re-initiates replica-set using the new state server
// values, this is required after a mongo restore.
// in case of failure returns error
func resetReplicaSet(dialInfo *mgo.DialInfo, memberHostPort string) error {
	params := peergrouper.InitiateMongoParams{dialInfo,
		memberHostPort,
		dialInfo.Username,
		dialInfo.Password,
	}

	return peergrouper.InitiateMongoServer(params, true)
}

var replaceableFiles = getReplaceableFiles

// getReplaceableFiles will return a map with the files/folders that need to
// be replaces so they can be deleted prior to a restore.
func getReplaceableFiles() (map[string]os.FileMode, error) {
	replaceables := map[string]os.FileMode{}
	os.Rename("/var/log/juju", "/var/log/oldjuju")

	aStat, _ := os.Stat("/var/log/oldjuju")
	os.MkdirAll("/var/log/juju", aStat.Mode())

	for _, replaceable := range []string{
		"/var/lib/juju/db",
		"/var/lib/juju",
		"/var/log/juju",
	} {
		dirStat, err := os.Stat(replaceable)
		if err != nil {
			return map[string]os.FileMode{}, err
		}
		replaceables[replaceable] = dirStat.Mode()
	}
	return replaceables, nil
}

var filesystemRoot = getFilesystemRoot

func getFilesystemRoot() string {
	return "/"
}

// prepareMachineForRestore deletes all files from the re-bootstrapped
// machine that are to be replaced by the backup and recreates those
// directories that are to contain new files; this is to avoid
// possible mixup from new/old files that lead to an inconsistent
// restored state machine.
func prepareMachineForRestore() error {
	replaceFiles, err := replaceableFiles()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve the list of folders to be cleaned before restore")
	}
	var keys []string
	for k := range replaceFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, toBeRecreated := range keys {
		fmode := replaceFiles[toBeRecreated]
		_, err := os.Stat(toBeRecreated)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.RemoveAll(toBeRecreated); err != nil {
			return err
		}
		if err := os.MkdirAll(toBeRecreated, fmode); err != nil {
			return err
		}
	}
	return nil
}

// newDialInfo returns mgo.DialInfo with the given address using the minimal
// possible setup
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

var mongorestorePath = getMongorestorePath

// getMongorestorePath will look for mongorestore binary on the system
// and return it if mongorestore actually exists.
// it will look first for the juju provided one and if not found make a
// try at a system one.
func getMongorestorePath() (string, error) {
	const mongoRestoreFullPath string = "/usr/lib/juju/bin/mongorestore"

	if _, err := os.Stat(mongoRestoreFullPath); err == nil {
		return mongoRestoreFullPath, nil
	}

	path, err := exec.LookPath("mongorestore")
	if err != nil {
		return "", err
	}
	return path, nil
}

// updateMongoEntries will update the machine entries in the restored mongo to
// reflect the real machine instanceid in case it changed (a newly bootstraped
// server)
func updateMongoEntries(newInstId instance.Id, dialInfo *mgo.DialInfo) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		errors.Annotate(err, "cannot connect to mongo to update")
	}
	defer session.Close()
	if err := session.DB("juju").C("machines").Update(bson.M{"machineid": "0"}, bson.M{"$set": bson.M{"instanceid": string(newInstId)}}); err != nil {
		return errors.Annotate(err, "cannot update machine 0 instance information")
	}
	return nil
}

// getMongoRestoreArgsForVersion returns a string slice containing the args to be used
// to call mongo restore since these can change depending on the backup method
// Version 0: a dump made with --db, stoping the state server.
// Version 1: a dump made with --oplog with a running state server.
func getMongoRestoreArgsForVersion(version int, dumpPath string) ([]string, error) {
	MGORestoreVersions := map[int][]string{}

	MGORestoreVersions[0] = []string{
		"--drop",
		"--dbpath", agent.DefaultDataDir,
		dumpPath}

	MGORestoreVersions[1] = []string{
		"--drop",
		"--oplogReplay",
		"--dbpath", agent.DefaultDataDir,
		dumpPath}
	if restoreCommand, ok := MGORestoreVersions[version]; ok {
		return restoreCommand, nil
	}
	return nil, errors.Errorf("this backup file is incompatible with the current version of juju")
}

// placeNewMongo tries to use mongorestore to replace an existing
// mongo with the dump in newMongoDumpPath
// returns an error if its not possible
func placeNewMongo(newMongoDumpPath string, version int) error {
	mongoRestore, err := mongorestorePath()
	if err != nil {
		return errors.Annotate(err, "mongorestore not available")
	}

	mgoRestoreArgs, err := getMongoRestoreArgsForVersion(version, newMongoDumpPath)
	if err != nil {
		return fmt.Errorf("cannot restore this backup version")
	}
	if err = runCommand(
		"initctl",
		"stop",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to stop mongo")
	}

	err = runCommand(mongoRestore, mgoRestoreArgs...)

	if err != nil {
		return errors.Annotate(err, "failed to restore database dump")
	}

	if err = runCommand(
		"initctl",
		"start",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}

	return nil
}

// credentials for mongo in backed up agent.conf
type credentials struct {
	tag           string
	tagPassword   string
	adminUsername string
	adminPassword string
}

// agentConfig config from the backed up agent.conf
type agentConfig struct {
	credentials credentials
	apiPort     string
	statePort   string
	cACert      string
}

// fetchAgentConfigFromBackup parses <dataDir>/machine-0/agents/machine-0/agent.conf
// and returns an agentConfig struct filled with the data that will not change
// from the backed up one (typically everything but the hosts)
func fetchAgentConfigFromBackup(agentConf io.Reader) (agentConfig, error) {

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

	tagUser, ok := m["tag"].(string)
	if !ok || tagUser == "" {
		return agentConfig{}, fmt.Errorf("tag not found in configuration")
	}

	tagPassword, ok := m["statepassword"].(string)
	if !ok || tagPassword == "" {
		return agentConfig{}, fmt.Errorf("agent tag user password not found in configuration")
	}

	adminPassword, ok := m["oldpassword"].(string)
	if !ok || adminPassword == "" {
		return agentConfig{}, fmt.Errorf("agent admin password not found in configuration")
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

	cacert, ok := m["cacert"].(string)
	if !ok {
		return agentConfig{}, fmt.Errorf("CACert not found in configuration")
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

// newStateConnection tries to connect to the newly restored state server
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
	for key, _ := range machines {
		// key is used to have machine be scope bound to the loop iteration
		machine := machines[key]
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet supported by this code.
		if machine.IsManager() || machine.Life() == state.Dead {
			continue
		}
		pendingMachineCount++

		go func() {
			err := runMachineUpdate(machine, setAgentAddressScript(privateHostPorts))
			if err != nil {
				errors.Annotatef(err, "failed to update machine %s", machine)
			}
			done <- err
		}()
	}
	err = nil
	for ; pendingMachineCount > 0; pendingMachineCount-- {
		if updateErr := <-done; updateErr != nil && err == nil {
			err = errors.Annotate(updateErr, "machine update failed")
		}
	}
	return err
}

func mustParseTemplate(templ string) *template.Template {
	return template.Must(template.New("").Parse(templ))
}

// agentAddressTemplate is the template used to replace the api server data
// in the agents for the new ones if the machine has been rebootstraped
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

// setAgentAddressScript generates an ssh script argument to update state addresses
func setAgentAddressScript(stateAddr string) string {
	return execTemplate(agentAddressTemplate, struct {
		Address string
	}{stateAddr})
}

// runMachineUpdate connects via ssh to the machine and runs the update script
func runMachineUpdate(m *state.Machine, sshArg string) error {
	addr := network.SelectPublicAddress(m.Addresses())
	if addr == "" {
		return fmt.Errorf("no appropriate public address found")
	}
	return runViaSSH(addr, sshArg)
}

// runViaSSH runs script in the remote machine with address addr
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
func backupVersion(backupMetadata *metadata.Metadata, backupFilesPath string) int {
	backupMetadataFile := true
	if _, err := os.Stat(filepath.Join(backupFilesPath, "metadata.json")); os.IsNotExist(err) {
		backupMetadataFile = false
	}
	if backupMetadata == nil && !backupMetadataFile {
		return 0
	}
	return 1
}
