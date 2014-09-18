// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows !linux

package restore

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

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/peergrouper"
)

var logger = loggo.GetLogger("juju.state.restore")

var runCommand = _runCommand

func _runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()

	afile, err := os.Create("/home/ubuntu/" + strings.Replace(cmd, "/", "_", -1))
	defer afile.Close()
	afile.Write(out)
	for _, arg := range args {
		afile.WriteString(fmt.Sprintf("\n%s\n", arg))
	}
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return errors.Annotatef(err, "error executing %q: %s", cmd, strings.Replace(string(out), "\n", "; ", -1))
	}
	return errors.Annotatef(err, "cannot execute %q", cmd)
}

func untarFiles(tarFile string, outputFolder string, compress bool) error {
	f, err := os.Open(tarFile)
	if err != nil {
		return errors.Annotatef(err, "cannot open backup file %q", tarFile)
	}
	defer f.Close()
	var r io.Reader = f
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

var getMongorestorePath = _getMongorestorePath

func _getMongorestorePath() (string, error) {
	const mongoDumpPath string = "/usr/lib/juju/bin/mongorestore"

	if _, err := os.Stat(mongoDumpPath); err == nil {
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath("mongorestore")
	if err != nil {
		return "", err
	}
	return path, nil
}

var getMongoDbPath = _getMongoDbPath

func _getMongoDbPath() string {
	return "/var/lib/juju/db"
}

func updateMongoEntries(newInstId instance.Id, dialInfo *mgo.DialInfo) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		errors.Annotate(err, "cannot connect to mongo to update")
	}
	defer session.Close()
	if err := session.DB("juju").C("machines").Update(bson.M{"_id": "0"}, bson.M{"$set": bson.M{"instanceid": string(newInstId)}}); err != nil {
		return errors.Annotate(err, "cannot update machine 0 instance information")
	}
	return nil
}

func getMongoRestoreArgsForVersion(version int, dumpPath string) ([]string, error) {
	MGORestoreVersions := map[int][]string{}

	MGORestoreVersions[0] = []string{
		"--drop",
		"--dbpath", getMongoDbPath(),
		dumpPath}

	MGORestoreVersions[1] = []string{
		"--drop",
		"--oplogReplay",
		"--dbpath", getMongoDbPath(),
		dumpPath}
	if restoreCommand, ok := MGORestoreVersions[version]; ok {
		return restoreCommand, nil
	}
	return nil, fmt.Errorf("no restore command for backup version %d", version)
}

// placeNewMongo tries to use mongorestore to replace an existing
// mongo (obtained from getMongoDbPath) with the dump in newMongoDumpPath
// returns an error if its not possible
func placeNewMongo(newMongoDumpPath string) error {
	mongoRestore, err := getMongorestorePath()
	if err != nil {
		return errors.Annotate(err, "mongorestore not available")
	}
	// TODO(perrito666): When there is a new backup version add mechanism to determine it
	mgoRestoreArgs, err := getMongoRestoreArgsForVersion(0, newMongoDumpPath)
	if err != nil {
		return fmt.Errorf("cannot restore this backup version")
	}
	if err = _runCommand(
		"initctl",
		"stop",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to stop mongo")
	}

	err = _runCommand(mongoRestore, mgoRestoreArgs...)

	if err != nil {
		return errors.Annotate(err, "failed to restore database dump")
	}

	if err = _runCommand(
		"initctl",
		"start",
		"juju-db"); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}

	return nil
}

type credentials struct {
	tag           string
	tagPassword   string
	adminUsername string
	adminPassword string
}

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

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
func updateAllMachines(privateAddress string, agentConf agentConfig) error {
	privateHostPorts := fmt.Sprintf("%s:%s", privateAddress, agentConf.statePort)
	caCert := agentConf.cACert
	// TODO(dfc) agenConf.credentials should supply a Tag
	tag, err := names.ParseTag(agentConf.credentials.tag)
	if err != nil {
		return err
	}
	// We need to retry here to allow mongo to come up on the restored state server.
	// The connection might succeed due to the mongo dial retries but there may still
	// be a problem issuing database commands.
	var st *state.State
	attempt := utils.AttemptStrategy{Delay: 15 * time.Second, Min: 8}
	for a := attempt.Start(); a.Next(); {
		st, err = state.Open(&mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{fmt.Sprintf("localhost:%s", agentConf.statePort),},
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
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}

	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	pendingMachineCount := 0
	done := make(chan error)
	for _, machine := range machines {
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet supported by this code.
		if machine.IsManager() || machine.Life() == state.Dead {
			continue
		}
		pendingMachineCount++
		machine := machine
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

func runViaSSH(addr string, script string) error {
	// This is taken from cmd/juju/ssh.go there is no other clear way to set user
	userAddr := "ubuntu@" + addr
	userCmd := ssh.Command(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, nil)
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


// Restore extract the content of the given backup file and:
// * runs mongorestore with the backed up mongo dump
// * updates and writes configuration files
// * updates existing db entries to make sure they hold no references to
// old instances
func Restore(backupFile, privateAddress string, status *state.State) error { //, environ environs.Environ) error {
	machine, err := status.Machine("0")
	if err != nil {
		return errors.Annotate(err, "cannot find bootstrap machine in status")
	}
	newInstId, err := machine.InstanceId()
	if err != nil {
		return errors.Annotate(err, "cannot get instance id for bootstraped machine")
	}

	workDir := os.TempDir()

	// XXX (perrito666) obtain this from the proper place here and in backup
	const agentFile string = "var/lib/juju/agents/machine-0/agent.conf"

	// Extract outer container
	if err := untarFiles(backupFile, workDir, true); err != nil {
		return errors.Annotate(err, "cannot extract files from backup")
	}
	if err := prepareMachineForRestore(); err != nil {
		return errors.Annotate(err, "cannot delete existing files")
	}
	backupFilesPath := filepath.Join(workDir, "juju-backup")

	defer os.RemoveAll(backupFilesPath)
	// Extract inner container
	innerBackup := filepath.Join(backupFilesPath, "root.tar")
	if err := untarFiles(innerBackup, filesystemRoot(), false); err != nil {
		return errors.Annotate(err, "cannot obtain system files from backup")
	}
	// Restore backed up mongo
	mongoDump := filepath.Join(backupFilesPath, "dump")
	if err := placeNewMongo(mongoDump); err != nil {
		return errors.Annotate(err, "error restoring state from backup")
	}

	// Load configuration values that are to remain
	agentConfFile, err := os.Open(agentFile)
	if err != nil {
		return errors.Annotate(err, "cannot open agent configuration file")
	}
	defer agentConfFile.Close()

	agentConf, err := fetchAgentConfigFromBackup(agentConfFile)
	if err != nil {
		return errors.Annotate(err, "cannot obtain agent configuration information")
	}

	// Re-start replicaset with the new value for server address
	dialInfo, err := newDialInfo(privateAddress, agentConf)
	if err != nil {
		return errors.Annotate(err, "cannot produce dial information")
	}
	memberHostPort := fmt.Sprintf("%s:%s", privateAddress, agentConf.statePort)
	err = resetReplicaSet(dialInfo, memberHostPort)
	if err != nil {
		return errors.Annotate(err, "cannot reset replicaSet")
	}

	// Update entries for machine 0 to point to the newest instance
	err = updateMongoEntries(newInstId, dialInfo)
	if err != nil {
		return errors.Annotate(err, "cannot update mongo entries")
	}

	/* err = updateAllMachines(privateAddress, agentConf)
	if err != nil {
		return fmt.Errorf("cannot update agents: %v", err)
	}
*/
	return nil
}
