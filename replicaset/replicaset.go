package replicaset

import (
	"fmt"
	"io"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

// Initiate sets up a replica set with the given replica set name with the
// single given member.  It need be called only once for a given mongo replica
// set.
//
// Note that you must set DialWithInfo and set Direct = true when dialing into a
// specific non-initiated mongo server.  The session will be set to Monotonic
// mode.
//
// See http://docs.mongodb.org/manual/reference/method/rs.initiate/ for more
// details.
func Initiate(session *mgo.Session, address, name string) error {
	session.SetMode(mgo.Monotonic, true)
	cfg := Config{
		Name:    name,
		Version: 1,
		Members: []Member{{Id: 1, Address: address}},
	}
	return session.Run(bson.D{{"replSetInitiate", cfg}}, nil)
}

// Member holds configuration information for a replica set member.
//
// See http://docs.mongodb.org/manual/reference/replica-configuration/
// for more details
type Member struct {
	// Id is a unique id for a member in a set.
	Id int `bson:"_id"`

	// Address holds the network address of the member,
	// in the form hostname:port.
	Address string `bson:"host"`

	// Arbiter holds whether the member is an arbiter only.
	// This value is optional; it defaults to false.
	Arbiter *bool `bson:"arbiterOnly,omitempty"`

	// BuildIndexes determines whether the mongod builds indexes on this member.
	// This value is optional; it defaults to true.
	BuildIndexes *bool `bson:"buildIndexes,omitempty"`

	// Hidden determines whether the replica set hides this member from
	// the output of IsMaster.
	// This value is optional; it defaults to false.
	Hidden *bool `bson:"hidden,omitempty"`

	// Priority determines eligibility of a member to become primary.
	// This value is optional; it defaults to 1.
	Priority *float64 `bson:"priority,omitempty"`

	// Tags store additional information about a replica member, often used for
	// customizing read preferences and write concern.
	Tags map[string]string `bson:"tags,omitempty"`

	// SlaveDelay describes the number of seconds behind the master that this
	// replica set member should lag rounded up to the nearest second.
	// This value is optional; it defaults to 0.
	SlaveDelay *time.Duration `bson:"slaveDelay,omitempty"`

	// Votes controls the number of votes a server has in a replica set election.
	// This value is optional; it defaults to 1.
	Votes *int `bson:"votes,omitempty"`
}

// Add adds the given members to the session's replica set.  Duplicates of
// existing replicas will be ignored.
//
// Members will have their Ids set automatically if they are not already > 0
func Add(session *mgo.Session, members ...Member) error {
	config, err := CurrentConfig(session)
	if err != nil {
		return err
	}

	config.Version++
	max := 0
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
		// let the caller specify an id if they want, treat zero as unspecified
		if newMember.Id < 1 {
			max++
			newMember.Id = max
		}
		config.Members = append(config.Members, newMember)
	}
	return session.Run(bson.D{{"replSetReconfig", config}}, nil)
}

// Remove removes members with the given addresses from the replica set. It is
// not an error to remove addresses of non-existent replica set members.
func Remove(session *mgo.Session, addrs ...string) error {
	config, err := CurrentConfig(session)
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
	err = session.Run(bson.D{{"replSetReconfig", config}}, nil)
	if err == io.EOF {
		// EOF means we got disconnected due to the Remove... this is normal.
		// Refreshing should fix us up.
		session.Refresh()
		err = nil
	}
	return err
}

// Set changes the current set of replica set members.  Members will have their
// ids set automatically if their ids are not already > 0.
func Set(session *mgo.Session, members []Member) error {
	config, err := CurrentConfig(session)
	if err != nil {
		return err
	}

	config.Version++

	// Assign ids to members that did not previously exist, starting above the
	// value of the highest id that already existed
	ids := map[string]int{}
	max := 0
	for _, m := range config.Members {
		ids[m.Address] = m.Id
		if m.Id > max {
			max = m.Id
		}
	}

	for x, m := range members {
		if id, ok := ids[m.Address]; ok {
			m.Id = id
		} else if m.Id < 1 {
			max++
			m.Id = max
		}
		members[x] = m
	}

	config.Members = members

	err = session.Run(bson.D{{"replSetReconfig", config}}, nil)
	if err == io.EOF {
		// EOF means we got disconnected due to a Remove... this is normal.
		// Refreshing should fix us up.
		session.Refresh()
		err = nil
	}
	return err
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
	results := &IsMasterResults{}
	err := session.Run("isMaster", results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// CurrentMembers returns the current members of the replica set.
func CurrentMembers(session *mgo.Session) ([]Member, error) {
	cfg, err := CurrentConfig(session)
	if err != nil {
		return nil, err
	}
	return cfg.Members, nil
}

// CurrentConfig returns the Config for the given session's replica set.
func CurrentConfig(session *mgo.Session) (*Config, error) {
	cfg := &Config{}
	err := session.DB("local").C("system.replset").Find(nil).One(cfg)
	if err != nil {
		return nil, fmt.Errorf("Error getting replset config : %s", err.Error())
	}
	return cfg, nil
}

// Config is the document stored in mongodb that defines the servers in the
// replica set
type Config struct {
	Name    string   `bson:"_id"`
	Version int      `bson:"version"`
	Members []Member `bson:"members"`
}

// CurrentStatus returns the status of the replica set for the given session.
func CurrentStatus(session *mgo.Session) (*Status, error) {
	status := &Status{}
	err := session.Run("replSetGetStatus", status)
	if err != nil {
		return nil, fmt.Errorf("Error from replSetGetStatus: %v", err)
	}
	return status, nil
}

// Status holds data about the status of members of the replica set returned
// from replSetGetStatus
//
// See http://docs.mongodb.org/manual/reference/command/replSetGetStatus/#dbcmd.replSetGetStatus
type Status struct {
	Name    string         `bson:"set"`
	Members []MemberStatus `bson:"members"`
}

// Status holds the status of a replica set member returned from
// replSetGetStatus.
type MemberStatus struct {
	// Id holds the replica set id of the member that the status is describing.
	Id int `bson:"_id"`

	// Address holds address of the member that the status is describing.
	Address string `bson:"name"`

	// Self holds whether this is the status for the member that
	// the session is connected to.
	Self bool `bson:"self"`

	// ErrMsg holds the most recent error or status message received
	// from the member.
	ErrMsg string `bson:"errmsg"`

	// Healthy reports whether the member is up. It is true for the
	// member that the request was made to.
	Healthy bool `bson:"health"`

	// State describes the current state of the member.
	State MemberState `bson:"state"`

	// Uptime describes how long the member has been online.
	Uptime time.Duration `bson:"uptime"`

	// Ping describes the length of time a round-trip packet takes to travel
	// between the remote member and the local instance.  It is zero for the
	// member that the session is connected to.
	Ping time.Duration `bson:"pingMS"`
}

// MemberState represents the state of a replica set member.
// See http://docs.mongodb.org/manual/reference/replica-states/
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

var memberStateStrings = []string{
	StartupState:    "STARTUP",
	PrimaryState:    "PRIMARY",
	SecondaryState:  "SECONDARY",
	RecoveringState: "RECOVERING",
	FatalState:      "FATAL",
	Startup2State:   "STARTUP2",
	UnknownState:    "UNKNOWN",
	ArbiterState:    "ARBITER",
	DownState:       "DOWN",
	RollbackState:   "ROLLBACK",
	ShunnedState:    "SHUNNED",
}

// String returns a string describing the state.
func (state MemberState) String() string {
	if state < 0 || int(state) >= len(memberStateStrings) {
		return "INVALID_MEMBER_STATE"
	}
	return memberStateStrings[state]
}
