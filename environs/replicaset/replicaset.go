package replicaset

import (
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// Initiate sets up a replica set with the given replica set name. It need be
// called only once for a given mongo replica set.
//
// Note that you must set DialWithInfo and set Direct = true when dialing into a
// specific non-initiated mongo server.
func Initiate(session *mgo.Session, address, name string) error {
	session.SetMode(mgo.Monotonic, true)
	cfg := replicaConfig{Name: name, Version: 1, Members: []Member{Member{Address: address}}}
	return session.Run(bson.D{{"replSetInitiate", cfg}}, nil)
}

// Member holds configuration information for a replica set member.
// The zero value for the type does not hold useful defaults - start
// with MemberDefaults instead.
type Member struct {
	// Id is a unique id for a member in a set.
	Id int `bson:"_id"`

	// Address holds the network address of the member,
	// in the form hostname:port.
	// See http://goo.gl/VYnZ2z
	Address string `bson:"host"`

	// Arbiter holds whether the member is an arbiter only,
	// See http://goo.gl/LbdhnR
	Arbiter *bool `bson:"arbiterOnly,omitempty"`

	// BuildIndexes determines whether the mongod builds indexes on this member,
	// defaulting to true.
	// See http://goo.gl/o3hSxg
	BuildIndexes *bool `bson:"buildIndexes,omitempty"`

	// Hidden determines whether the replica set hides this member from
	// the output of IsMaster.
	// See http://goo.gl/ERXGev
	Hidden *bool `bson:"hidden,omitempty"`

	// Priority determines eligibility of a member to become primary.
	// See http://goo.gl/kB27Ku
	Priority *float64 `bson:"priority,omitempty"`

	// SlaveDelay describes the number of seconds behind the master that this
	// replica set member should lag rounded up to the
	// nearest second.
	// See http://goo.gl/7vKUr6
	SlaveDelay *time.Duration `bson:"slaveDelay,omitempty"`

	// Votes controls the number of votes a server has in a replica set election.
	// See http://goo.gl/kgqrU1
	Votes *int `bson:"votes,omitempty"`
}

// Add adds the given members to the session's replica set.  Duplicates of
// existing replicas will be ignored.
//
// Members will have their Ids set automatically.
func Add(session *mgo.Session, members ...Member) error {
	config, err := getConfig(session)
	if err != nil {
		return err
	}

	config.Version++
	max := -1
	for _, member := range config.Members {
		if member.Id > max {
			max = member.Id
		}
	}

outerLoop:
	for _, newMember := range members {
		for _, member := range config.Members {
			if member.Address == newMember.Address {
				// already exists, skip it
				continue outerLoop
			}
		}
		max++
		newMember.Id = max
		config.Members = append(config.Members, newMember)
	}
	return session.Run(bson.D{{"replSetReconfig", config}, {"force", true}}, nil)
}

// Remove removes members with the given addresses from the replica set. It is
// not an error to remove addresses of non-existent replica set members.
func Remove(session *mgo.Session, addrs ...string) error {
	config, err := getConfig(session)
	if err != nil {
		return err
	}
	config.Version++
	for _, rem := range addrs {
		for n, repl := range config.Members {
			if repl.Address == rem {
				config.Members = append(config.Members[:n], config.Members[n+1:]...)
				break
			}
		}
	}

	return session.Run(bson.D{{"replSetReconfig", config}}, nil)
}

// Set changes the current set of replica set members.  Members will have their
// ids set automatically.
func Set(session *mgo.Session, members []Member) error {
	config, err := getConfig(session)
	if err != nil {
		return err
	}

	config.Version++
	for x := range members {
		members[x].Id = x
	}

	config.Members = members

	return session.Run(bson.D{{"replSetReconfig", config}}, nil)
}

// Config reports information about the configuration of a given mongo node
type IsMasterResults struct {
	// The following fields hold information about the specific mongodb node.
	IsMaster  bool      `bson:"ismaster"`
	Secondary bool      `bson:"secondary"`
	Arbiter   bool      `bson:"arbiterOnly"`
	Address   string    `bson:"me"`
	LocalTime time.Time `bson:"localTime"`

	// The following fields hold information about the replica set.
	ReplicaSetName string   `bson:"setName"`
	Addresses      []string `bson:"hosts"`
	Arbiters       []string `bson:"arbiters"`
	PrimaryAddress string   `bson:"primary"`
}

// IsMaster returns information about the configuration of the node that
// the given session is connected to.
func IsMaster(session *mgo.Session) (*IsMasterResults, error) {
	var results *IsMasterResults
	err := session.Run("isMaster", results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// CurrentMembers returns the current members of the replica set, keyed by
// member address.
func CurrentMembers(session *mgo.Session) ([]Member, error) {
	cfg, err := getConfig(session)
	if err != nil {
		return nil, err
	}
	return cfg.Members, nil
}

func getConfig(session *mgo.Session) (*replicaConfig, error) {
	cfg := &replicaConfig{}
	err := session.DB("local").C("system.replset").Find(nil).One(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

type replicaConfig struct {
	Name    string   `bson:"_id"`
	Version int      `bson:"version"`
	Members []Member `bson:"members"`
}

// CurrentStatus returns the status of each member, keyed by member address.
func CurrentStatus(session *mgo.Session) ([]Status, error) {
	type statuslist struct {
		Members []Status `bson:"members"`
	}
	list := &statuslist{}
	err := session.Run("replSetGetStatus", list)
	if err != nil {
		return nil, err
	}
	return list.Members, nil
}

// Status holds the status of a replica set member returned from
// replSetGetStatus.
type Status struct {
	// Address holds address of the member that the status is describing.
	// http://goo.gl/5KgCid
	Address string `bson:"name"`

	// Self holds whether this is the status for the member that
	// the session is connected to.
	// http://goo.gl/cgj16R
	Self bool `bson:"self"`

	// ErrMsg holds the most recent error or status message received
	// from the member.
	ErrMsg string `bson:"errmsg"`

	// Healthy reports whether the member is up. It is true for the
	// member that the request was made to.
	Healthy bool `bson:"health"`

	// State describes the current state of the member.
	State MemberState `bson:"myState"`

	// Uptime describes how long the member has been online.
	Uptime time.Duration `bson:"uptime"`

	// Ping describes the length of time a round-trip packet
	// takes to travel between the remote member and the local
	// instance.  It is zero for the member that the session
	// is connected to.
	Ping time.Duration `bson:"pingMS"`
}

// MemberState represents the state of a replica set member.
// See http://goo.gl/8unEn5.
type MemberState int

const (
	StartupState = iota
	PrimaryState
	SecondaryState
	RecoveringState
	FatalState
	Startup2State
	UnknownState
	ArbiterState
	DownState
	RollbackState
	ShunnedState
)

// String returns a string describing the state.
func (state MemberState) String() string {
	switch state {
	case StartupState:
		return "STARTUP"
	case PrimaryState:
		return "PRIMARY"
	case SecondaryState:
		return "SECONDARY"
	case RecoveringState:
		return "RECOVERING"
	case FatalState:
		return "FATAL"
	case Startup2State:
		return "STARTUP2"
	case UnknownState:
		return "UNKNOWN"
	case ArbiterState:
		return "ARBITER"
	case DownState:
		return "DOWN"
	case RollbackState:
		return "ROLLBACK"
	case ShunnedState:
		return "SHUNNED"
	}
	return "INVALID_MEMBER_STATE"
}

/*
// MemberDefaults holds the usual member configuration defaults.
// It is used by AddressToMember when constructing Member
// instances.
var MemberDefaults = Member{
	BuildIndexes: true,
	Priority:     1,
	Votes:        1,
}
*/

/*
// AddressToMember returns replica set configuration values for the given host
// addresses, using MemberDefaults to fill in the member field values.
func AddressToMember(hostAddrs ...string) []Member {
	members := make([]Member, 0, len(hostAddrs))
	for _, addr := range hostAddrs {
		m := MemberDefaults
		m.Address = addr
		members = append(members, m)
	}
	return members
}
*/
