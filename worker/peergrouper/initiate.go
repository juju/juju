// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
)

// TODO(katco): 2016-08-09: lp:1611427
var initiateAttemptStrategy = utils.AttemptStrategy{
	Total: 60 * time.Second,
	Delay: 5 * time.Second,
}

// InitiateMongoParams holds parameters for the MaybeInitiateMongo call.
type InitiateMongoParams struct {
	// DialInfo specifies how to connect to the mongo server.
	DialInfo *mgo.DialInfo

	// MemberHostPort provides the address to use for
	// the first replica set member.
	MemberHostPort string

	// User holds the user to log as in to the mongo server.
	// If it is empty, no login will take place.
	User     string
	Password string
}

// InitiateMongoServer checks for an existing mongo configuration.
// If no existing configuration is found one is created using Initiate.
func InitiateMongoServer(p InitiateMongoParams) error {
	logger.Debugf("Initiating mongo replicaset; dialInfo %#v; memberHostport %q; user %q; password %q", p.DialInfo, p.MemberHostPort, p.User, p.Password)
	defer logger.Infof("finished InitiateMongoServer")

	if len(p.DialInfo.Addrs) > 1 {
		logger.Infof("more than one member; replica set must be already initiated")
		return nil
	}
	p.DialInfo.Direct = true

	// Initiate may fail while mongo is initialising, so we retry until
	// we successfully populate the replicaset config.
	var err error
	for attempt := initiateAttemptStrategy.Start(); attempt.Next(); {
		err = attemptInitiateMongoServer(p.DialInfo, p.MemberHostPort)
		if err == nil {
			logger.Infof("replica set initiated")
			return err
		}
		if attempt.HasNext() {
			logger.Debugf("replica set initiation failed, will retry: %v", err)
		}
	}
	return errors.Annotatef(err, "cannot initiate replica set")
}

// attemptInitiateMongoServer attempts to initiate the replica set.
func attemptInitiateMongoServer(dialInfo *mgo.DialInfo, memberHostPort string) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return errors.Annotatef(err, "cannot dial mongo to initiate replicaset")
	}
	defer session.Close()
	session.SetSocketTimeout(mongo.SocketTimeout)

	return replicaset.Initiate(
		session,
		memberHostPort,
		mongo.ReplicaSetName,
		map[string]string{
			jujuMachineKey: agent.BootstrapMachineId,
		},
	)
}
