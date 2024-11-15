// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/replicaset/v3"
	"github.com/juju/retry"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/mongo"
)

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
	logger.Debugf(context.TODO(), "Initiating mongo replicaset; dialInfo %#v; memberHostport %q; user %q; password %q", p.DialInfo, p.MemberHostPort, p.User, p.Password)
	defer logger.Infof(context.TODO(), "finished InitiateMongoServer")

	if len(p.DialInfo.Addrs) > 1 {
		logger.Infof(context.TODO(), "more than one member; replica set must be already initiated")
		return nil
	}
	p.DialInfo.Direct = true

	// Initiate may fail while mongo is initialising, so we retry until
	// we successfully populate the replicaset config.
	retryCallArgs := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 60 * time.Second,
		Delay:       5 * time.Second,
		Func: func() error {
			return attemptInitiateMongoServer(p.DialInfo, p.MemberHostPort)
		},
		NotifyFunc: func(lastError error, attempt int) {
			logger.Debugf(context.TODO(), "replica set initiation attempt %d failed: %v", attempt, lastError)
		},
	}
	err := retry.Call(retryCallArgs)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
		logger.Debugf(context.TODO(), "replica set initiation failed: %v", err)
	}
	if err == nil {
		logger.Infof(context.TODO(), "replica set initiated")
		return nil
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
			jujuNodeKey: agent.BootstrapControllerId,
		},
	)
}
