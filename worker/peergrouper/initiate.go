// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"time"

	"github.com/juju/utils"
	"labix.org/v2/mgo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/replicaset"
)

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

// MaybeInitiateMongoServer checks for an existing mongo configuration.
// If no existing configuration is found one is created using Initiate.
func MaybeInitiateMongoServer(p InitiateMongoParams) error {
	logger.Debugf("Initiating mongo replicaset; dialInfo %#v; memberHostport %q; user %q; password %q", p.DialInfo, p.MemberHostPort, p.User, p.Password)
	defer logger.Infof("finished MaybeInitiateMongoServer")

	if len(p.DialInfo.Addrs) > 1 {
		logger.Infof("more than one member; replica set must be already initiated")
		return nil
	}
	p.DialInfo.Direct = true

	// TODO(rog) remove this code when we no longer need to upgrade
	// from pre-HA-capable environments.
	if p.User != "" {
		p.DialInfo.Username = p.User
		p.DialInfo.Password = p.Password
	}

	// Initiate may fail while mongo is initialising, so we retry until
	// we succssfully populate the replicaset config.
	var err error
	for attempt := initiateAttemptStrategy.Start(); attempt.Next(); {
		err = attemptInitiateMongoServer(p.DialInfo, p.MemberHostPort)
		if err == nil {
			logger.Infof("replica set initiated")
			return nil
		}
		if attempt.HasNext() {
			logger.Debugf("replica set initiation failed, will retry: %v", err)
		}
	}
	return fmt.Errorf("cannot initiate replica set: %v", err)
}

// attemptInitiateMongoServer attempts to initiate the replica set.
func attemptInitiateMongoServer(dialInfo *mgo.DialInfo, memberHostPort string) error {
	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return fmt.Errorf("can't dial mongo to initiate replicaset: %v", err)
	}
	defer session.Close()
	session.SetSocketTimeout(mongo.SocketTimeout)

	var cfg *replicaset.Config
	cfg, err = replicaset.CurrentConfig(session)
	if err == nil && len(cfg.Members) > 0 {
		logger.Infof("replica set configuration already found: %#v", cfg)
		return nil
	}
	if err != nil && err != mgo.ErrNotFound {
		return fmt.Errorf("cannot get replica set configuration: %v", err)
	}
	return replicaset.Initiate(
		session,
		memberHostPort,
		mongo.ReplicaSetName,
		map[string]string{
			jujuMachineTag: agent.BootstrapMachineId,
		},
	)
}
