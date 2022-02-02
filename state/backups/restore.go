// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package backups

import (
	"bytes"
	"net"
	"os"
	"strconv"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/worker/peergrouper"
)

// resetReplicaSet re-initiates replica-set using the new controller
// values, this is required after a mongo restore.
// In case of failure returns error.
func resetReplicaSet(dialInfo *mgo.DialInfo, memberHostPort string) error {
	params := peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: memberHostPort,
		User:           dialInfo.Username,
		Password:       dialInfo.Password,
	}
	return peergrouper.InitiateMongoServer(params)
}

// tagUserCredentials is a convenience function that extracts the
// tag user and apipassword, required to access mongodb.
func tagUserCredentials(conf agent.Config) (string, string, error) {
	username := conf.Tag().String()
	var password string
	// TODO(perrito) we might need an accessor for the actual state password
	// just in case it ever changes from the same as api password.
	apiInfo, ok := conf.APIInfo()
	if ok {
		password = apiInfo.Password
	} else {
		// There seems to be no way to reach this inconsistence other than making a
		// backup on a machine where these fields are corrupted and even so I find
		// no reasonable way to reach this state, yet since APIInfo has it as a
		// possibility I prefer to handle it, we cannot recover from this since
		// it would mean that the agent.conf is corrupted.
		return "", "", errors.New("cannot obtain password to access the controller")
	}
	return username, password, nil
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
		Addrs:  []string{net.JoinHostPort(privateAddr, strconv.Itoa(ssi.StatePort))},
		CACert: conf.CACert(),
	}
	dialInfo, err := mongo.DialInfo(info, dialOpts)
	if err != nil {
		return nil, errors.Annotate(err, "cannot produce a dial info")
	}
	oldPassword := conf.OldPassword()
	if oldPassword != "" {
		dialInfo.Username = "admin"
		dialInfo.Password = conf.OldPassword()
	} else {
		dialInfo.Username, dialInfo.Password, err = tagUserCredentials(conf)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return dialInfo, nil
}

// updateMongoEntries will update the machine entries in the restored mongo to
// reflect the real machine instanceid in case it changed (a newly bootstraped
// server).
func updateMongoEntries(newInstId instance.Id, newMachineId, oldMachineId string, dialInfo *mgo.DialInfo) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return errors.Annotate(err, "cannot connect to mongo to update")
	}
	defer session.Close()
	// TODO(perrito666): Take the Machine id from an autoritative source
	err = session.DB("juju").C("machines").Update(
		bson.M{"machineid": oldMachineId},
		bson.M{"$set": bson.M{"instanceid": string(newInstId)}},
	)
	if err != nil {
		return errors.Annotatef(err, "cannot update machine %s instance information", newMachineId)
	}
	return nil
}

// agentAddressAndRelationsTemplate is the template used to replace the api server data
// in the agents for the new ones if the machine has been rebootstraped it will also reset
// the relations so hooks will re-fire.
var agentAddressAndRelationsTemplate = template.Must(template.New("").Parse(`
set -xu
cd /var/lib/juju/agents
for agent in *
do
	service jujud-$agent stop > /dev/null

	# The below statement will work in cases where there
	# is a private address for the api server only
	# or where there are a private and a public, which are
	# the two common cases.
	sed -i.old -r "/^(stateaddresses|apiaddresses):/{
		n
		s/- .*(:[0-9]+)/- {{.Address}}\1/
		n
		s/- .*(:[0-9]+)/- {{.PubAddress}}\1/
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
	service jujud-$agent start > /dev/null
done
`))

// setAgentAddressScript generates an ssh script argument to update state addresses.
func setAgentAddressScript(stateAddr, statePubAddr string) string {
	var buf bytes.Buffer
	err := agentAddressAndRelationsTemplate.Execute(&buf, struct {
		Address    string
		PubAddress string
	}{stateAddr, statePubAddr})
	if err != nil {
		panic(errors.Annotate(err, "template error"))
	}
	return buf.String()
}

// sshCommand hods ssh.Command type for testing purposes.
var sshCommand = ssh.Command

// runViaSSH runs script in the remote machine with address addr.
func runViaSSH(addr string, script string) error {
	sshOptions := ssh.Options{}
	sshOptions.SetIdentities("/var/lib/juju/system-identity")
	// Disable host key checking. We're not pushing across anything
	// sensitive, and there's no guarantee that the machine would
	// have published up-to-date host key information.
	sshOptions.SetStrictHostKeyChecking(ssh.StrictHostChecksNo)
	sshOptions.SetKnownHostsFile(os.DevNull)

	userAddr := "ubuntu@" + addr
	userCmd := sshCommand(userAddr, []string{"sudo", "-n", "bash", "-c " + utils.ShQuote(script)}, &sshOptions)
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	userCmd.Stdout = &stdoutBuf
	userCmd.Stderr = &stderrBuf
	logger.Debugf("updating %s, script:\n%s", addr, script)
	if err := userCmd.Run(); err != nil {
		return errors.Annotatef(err, "ssh command failed: %q", stderrBuf.String())
	}
	logger.Debugf("result %s\nstdout: \n%s\nstderr: %s", addr, stdoutBuf.String(), stderrBuf.String())
	return nil
}
