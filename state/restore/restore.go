// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

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

	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/tar"
	"labix.org/v2/mgo"
	"launchpad.net/goyaml"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/peergrouper"
)

var runCommand = _runCommand

func _runCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return fmt.Errorf("error executing %q: %s", cmd, strings.Replace(string(out), "\n", "; ", -1))
	}
	return fmt.Errorf("cannot execute %q: %v", cmd, err)
}

func untarFiles(tarFile string, outputFolder string, compress bool) error {
	f, err := os.Open(tarFile)
	if err != nil {
		return fmt.Errorf("cannot open backup file %q: %v", tarFile, err)
	}
	defer f.Close()
	var r io.Reader = f
	if compress {
		r, err = gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("cannot uncompress tar file %q: %v", tarFile, err)
		}
	}
	return tar.UntarFiles(r, outputFolder)
}

func updateStateServersRecords() error {
	return nil
}

// resetReplicaSet re-initiates replica-set using the new state server
// values, this is required after a mongo restore.
// in case of failure returns error
func resetReplicaSet(dialInfo *mgo.DialInfo, memberHostPort string) error {
	params := peergrouper.InitiateMongoParams{dialInfo,
		memberHostPort,
		"",
		"",
	}
	return peergrouper.InitiateMongoServer(params, true)
}

var replaceableFiles = getReplaceableFiles

func getReplaceableFiles() (map[string]os.FileMode, error) {
	replaceables := map[string]os.FileMode{}
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
	return string(os.PathSeparator)
}

// prepareMachineForBackup deletes all files from the re-bootstrapped
// machine that are to be replaced by the backup and recreates those
// directories that are to contain new files; this is to avoid
// possible mixup from new/old files that lead to an inconsistent
// restored state machine.
func prepareMachineForBackup() error {
	replaceFiles, err := replaceableFiles()
	if err != nil {
		return fmt.Errorf("cannot retrieve the list of folders to be cleaned before backup: %v", err)
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

		if _, err := os.Stat(toBeRecreated); err != nil {
			return fmt.Errorf("There was an error creating %q: %v", toBeRecreated, err)
		}
	}
	return nil
}

// newDialInfo returns mgo.DialInfo with the given address using the minimal
// possible setup
func newDialInfo(privateAddr string, conf agentConfig) mgo.DialInfo {
	return mgo.DialInfo{
		Addrs:    []string{fmt.Sprintf("%s:%s", privateAddr, conf.ApiPort)},
		Timeout:  30 * time.Second,
		Username: conf.Credentials.AdminUsername,
		Password: conf.Credentials.AdminPassword,
	}
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

// placeNewMongo tries to use mongorestore to replace an existing
// mongo (obtained from getMongoDbPath) with the dump in newMongoDumpPath
// returns an error if its not possible
func placeNewMongo(newMongoDumpPath string) error {
	mongoRestore, err := getMongorestorePath()
	if err != nil {
		return fmt.Errorf("mongorestore not available: %v", err)
	}
	err = runCommand(
		mongoRestore,
		"--drop",
		"--oplogReplay",
		"--dbpath", getMongoDbPath(),
		newMongoDumpPath)

	if err != nil {
		return fmt.Errorf("failed to restore database dump: %v", err)
	}

	return nil
}

type credentials struct {
	Tag           string
	TagPassword   string
	AdminUsername string
	AdminPassword string
}

type agentConfig struct {
	Credentials credentials
	ApiPort     string
	StatePort   string
}

// fetchAgentConfigFromBackup parses <dataDir>/machine-0/agents/machine-0/agent.conf
// and returns an agentConfig struct filled with the data that will not change
// from the backed up one (tipically everything but the hosts)
func fetchAgentConfigFromBackup(agentConfigFilePath string) (agentConfig, error) {
	agentConf, err := os.Open(agentConfigFilePath)
	if err != nil {
		return agentConfig{}, err
	}
	defer agentConf.Close()

	data, err := ioutil.ReadAll(agentConf)
	if err != nil {
		return agentConfig{}, fmt.Errorf("failed to read agent config file: %v", err)
	}
	var conf interface{}
	if err := goyaml.Unmarshal(data, &conf); err != nil {
		return agentConfig{}, fmt.Errorf("cannot unmarshal agent config file: %v", err)
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

	return agentConfig{
		Credentials: credentials{
			Tag:           tagUser,
			TagPassword:   tagPassword,
			AdminUsername: "admin",
			AdminPassword: adminPassword,
		},
		StatePort: statePort,
		ApiPort:   apiPort,
	}, nil
}

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
func updateAllMachines(environ environs.Environ, privateAddress string, agentConf agentConfig) error {
	privateHostPorts := fmt.Sprintf("%s:%s", privateAddress, agentConf.StatePort)
	caCert, ok := environ.Config().CACert()
	if !ok {
		return fmt.Errorf("configuration has no CA certificate")
	}
	// TODO(dfc) agenConf.Credentials should supply a Tag
	tag, err := names.ParseTag(agentConf.Credentials.Tag)
	if err != nil {
		return err
	}
	// We need to retry here to allow mongo to come up on the restored state server.
	// The connection might succeed due to the mongo dial retries but there may still
	// be a problem issuing database commands.
	var st *state.State
	attempt := utils.AttemptStrategy{Delay: 15 * time.Second, Min: 8}
	for a := attempt.Start(); a.Next(); {
		st, err = state.Open(&authentication.MongoInfo{
			Info: mongo.Info{
				Addrs:  []string{privateHostPorts},
				CACert: caCert,
			},
			Tag:      tag,
			Password: agentConf.Credentials.TagPassword,
		}, mongo.DefaultDialOpts(), environs.NewStatePolicy())
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("cannot open state: %v", err)
	}

	machines, err := st.AllMachines()
	if err != nil {
		return err
	}
	pendingMachineCount := 0
	done := make(chan error)
	for _, machine := range machines {
		// A newly resumed state server requires no updating, and more
		// than one state server is not yet support by this code.
		if machine.IsManager() || machine.Life() == state.Dead {
			continue
		}
		pendingMachineCount++
		machine := machine
		go func() {
			err := runMachineUpdate(machine, setAgentAddressScript(privateHostPorts))
			if err != nil {
				fmt.Errorf("failed to update machine %s: %v", machine, err)
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

func mustParseTemplate(templ string) *template.Template {
	t := template.New("").Funcs(template.FuncMap{
		"shquote": utils.ShQuote,
	})
	return template.Must(t.Parse(templ))
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
		panic(fmt.Errorf("template error: %v", err))
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
		return fmt.Errorf("ssh command failed: %v (%q)", err, stderrBuf.String())
	}
	return nil
}

func updateStateServerAddress(newAddress string, agentConf agentConfig) {
	// TODO: Use te same function that jujud command to update api/state server
}

// Restore extract the content of the given backup file and:
// * runs mongorestore with the backed up mongo dump
// * updates and writes configuration files
// * updates existing db entries to make sure they hold no references to
// old instances
func Restore(backupFile, privateAddress string, environ environs.Environ) error {
	workDir := os.TempDir()
	defer os.RemoveAll(workDir)
	// XXX (perrito666) obtain this from the proper place here and in backup
	const agentFile string = "var/lib/juju/agents/machine-0/agent.conf"

	// Extract outer container
	if err := untarFiles(backupFile, workDir, true); err != nil {
		return fmt.Errorf("cannot extract files from backup: %v", err)
	}
	if err := prepareMachineForBackup(); err != nil {
		return fmt.Errorf("cannot delete existing files: %v", err)
	}
	backupFilesPath := filepath.Join(workDir, "juju-backup")
	// Extract inner container
	innerBackup := filepath.Join(backupFilesPath, "root.tar")
	if err := untarFiles(innerBackup, filesystemRoot(), false); err != nil {
		return fmt.Errorf("cannot obtain system files from backup: %v", err)
	}
	// Restore backed up mongo
	mongoDump := filepath.Join(backupFilesPath, "juju-backup", "dump")
	if err := placeNewMongo(mongoDump); err != nil {
		return fmt.Errorf("error restoring state from backup: %v", err)
	}
	// Load configuration values that are to remain
	agentConf, err := fetchAgentConfigFromBackup(agentFile)
	if err != nil {
		return fmt.Errorf("cannot obtain agent configuration information: %v", err)
	}
	// Re-start replicaset with the new value for server address
	dialInfo := newDialInfo(privateAddress, agentConf)
	memberHostPort := fmt.Sprintf("%s:%s", privateAddress, agentConf.ApiPort)
	resetReplicaSet(&dialInfo, memberHostPort)
	updateStateServerAddress(privateAddress, agentConf) // XXX Stub

	err = updateAllMachines(environ, privateAddress, agentConf)
	if err != nil {
		return fmt.Errorf("cannot update agents: %v", err)
	}

	return nil
}
