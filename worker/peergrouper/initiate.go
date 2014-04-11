package peergrouper

import (
	"fmt"

	"labix.org/v2/mgo"
	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/replicaset"
)

const bootstrapMachineId = "0"

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
	session, err := mgo.DialWithInfo(p.DialInfo)
	if err != nil {
		return fmt.Errorf("can't dial mongo to initiate replicaset: %v", err)
	}
	defer session.Close()

	// TODO(rog) remove this code when we no longer need to upgrade
	// from pre-HA-capable environments.
	if p.User != "" {
		err := session.DB("admin").Login(p.User, p.Password)
		if err != nil {
			logger.Errorf("cannot login to admin db as %q, password %q, falling back: %v", p.User, p.Password, err)
		}
	}
	cfg, err := replicaset.CurrentConfig(session)
	if err == nil && len(cfg.Members) > 0 {
		logger.Infof("replica set configuration already found: %#v", cfg)
		return nil
	}
	if err != nil && err != mgo.ErrNotFound {
		return fmt.Errorf("cannot get replica set configuration: %v", err)
	}
	err = replicaset.Initiate(
		session,
		p.MemberHostPort,
		mongo.ReplicaSetName,
		map[string]string{
			jujuMachineTag: agent.BootstrapMachineId,
		},
	)
	if err != nil {
		return fmt.Errorf("cannot initiate replica set: %v", err)
	}
	return nil
}
