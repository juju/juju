// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/worker/peergrouper"
)

// TODO(perrito666) create an authoritative source for all possible
// uses of this const, not only here but all around juju
const restoreUserHome = "/home/ubuntu/"

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
	return string(os.PathSeparator)
}

// newDialInfo returns mgo.DialInfo with the given address using the minimal
// possible setup.
func newDialInfo(privateAddr string, conf agent.Config) (*mgo.DialInfo, error) {
	dialOpts := mongo.DialOpts{Direct: true}
	ssi, ok := conf.StateServingInfo()
	if !ok {
		return nil, errors.Errorf("cannot get state serving info to dial")
	}
	info := mongo.Info{
		Addrs:  []string{fmt.Sprintf("%s:%d", privateAddr, ssi.StatePort)},
		CACert: conf.CACert(),
	}
	dialInfo, err := mongo.DialInfo(info, dialOpts)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce a dial info")
	}
	dialInfo.Username = "admin"
	dialInfo.Password = conf.OldPassword()
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
	// TODO(perrito666): Take the Machine id from an autoritative source
	err = session.DB("juju").C("machines").Update(
		bson.M{"machineid": "0"},
		bson.M{"$set": bson.M{"instanceid": string(newInstId)}},
	)
	if err != nil {
		return errors.Annotate(err, "cannot update machine 0 instance information")
	}
	return nil
}

// assign to variables for testing purposes.
var mongoDefaultDialOpts = mongo.DefaultDialOpts
var environsNewStatePolicy = environs.NewStatePolicy

// newStateConnection tries to connect to the newly restored state server.
func newStateConnection(info *mongo.MongoInfo) (*state.State, error) {
	// We need to retry here to allow mongo to come up on the restored state server.
	// The connection might succeed due to the mongo dial retries but there may still
	// be a problem issuing database commands.
	var (
		st  *state.State
		err error
	)
	attempt := utils.AttemptStrategy{Delay: 15 * time.Second, Min: 8}
	for a := attempt.Start(); a.Next(); {
		st, err = state.Open(info, mongoDefaultDialOpts(), environsNewStatePolicy())
		if err == nil {
			return st, nil
		}
		logger.Errorf("cannot open state, retrying: %v", err)
	}
	return st, errors.Annotate(err, "cannot open state")
}

// updateAllMachines finds all machines and resets the stored state address
// in each of them. The address does not include the port.
// It is too late to go back and errors in a couple of agents have
// better chance of being fixed by the user, if we were to fail
// we risk an inconsistent state server because of one unresponsive
// agent, we should nevertheless return the err info to the user.
func updateAllMachines(privateAddress string, machines []*state.Machine) error {
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
			err := runMachineUpdate(machine.Addresses(), setAgentAddressScript(privateAddress))
			done <- errors.Annotatef(err, "failed to update machine %s", machine)
		}()
	}
	for ; pendingMachineCount > 0; pendingMachineCount-- {
		if updateErr := <-done; updateErr != nil {
			logger.Errorf("failed updating machine: %v", updateErr)
		}
	}
	// We should return errors encapsulated in a digest.
	return nil
}

// agentAddressTemplate is the template used to replace the api server data
// in the agents for the new ones if the machine has been rebootstraped.
var agentAddressTemplate = template.Must(template.New("").Parse(`
set -xu
cd /var/lib/juju/agents
for agent in *
do
	status  jujud-$agent| grep -q "^jujud-$agent start" > /dev/null
	if [ $? -eq 0 ]; then
		initctl stop jujud-$agent 
	fi
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
	# Just in case is a stale unit
	status  jujud-$agent| grep -q "^jujud-$agent stop" > /dev/null
	if [ $? -eq 0 ]; then
		initctl start jujud-$agent
	fi
done
`))

// setAgentAddressScript generates an ssh script argument to update state addresses.
func setAgentAddressScript(stateAddr string) string {
	var buf bytes.Buffer
	err := agentAddressTemplate.Execute(&buf, struct {
		Address string
	}{stateAddr})
	if err != nil {
		panic(errors.Annotate(err, "template error"))
	}
	return buf.String()
}

// runMachineUpdate connects via ssh to the machine and runs the update script.
func runMachineUpdate(allAddr []network.Address, sshArg string) error {
	addr := network.SelectPublicAddress(allAddr)
	if addr == "" {
		return errors.Errorf("no appropriate public address found")
	}
	return runViaSSH(addr, sshArg)
}

// sshCommand hods ssh.Command type for testing purposes.
var sshCommand = ssh.Command

// runViaSSH runs script in the remote machine with address addr.
func runViaSSH(addr string, script string) error {
	// This is taken from cmd/juju/ssh.go there is no other clear way to set user
	userAddr := "ubuntu@" + addr
	sshOptions := ssh.Options{}
	sshOptions.SetIdentities("/var/lib/juju/system-identity")
	userCmd := sshCommand(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, &sshOptions)
	var stderrBuf bytes.Buffer
	userCmd.Stderr = &stderrBuf
	if err := userCmd.Run(); err != nil {
		return errors.Annotatef(err, "ssh command failed: %q", stderrBuf.String())
	}
	return nil
}

// backupFile is due to be obsoleted when upload supports adding
// files to the backups db and Backups.Get becoms the only way
// to obtain a backup file handler.
func backupFile(backupId string, backup Backups) (io.ReadCloser, error) {
	var (
		fileHandler io.ReadCloser
		err         error
	)
	switch {
	case strings.HasPrefix(backupId, FilenamePrefix):
		fileName := strings.TrimPrefix(backupId, FilenamePrefix)
		fileName = restoreUserHome + fileName
		if fileHandler, err = os.Open(fileName); err != nil {
			return nil, errors.Annotatef(err, "error opening %q", fileName)
		}
	case backupId == "":
		return nil, errors.Errorf("no backup file or id given")
	default:
		if _, fileHandler, err = backup.Get(backupId); err != nil {
			return nil, errors.Annotatef(err, "could not fetch backup %q", backupId)
		}
	}
	return fileHandler, nil
}
