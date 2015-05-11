package replicaset

import (
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	// MaxPeers defines the maximum number of peers that mongo supports.
	MaxPeers = 7

	// maxInitiateAttempts is the maximum number of times to attempt
	// replSetInitiate for each call to Initiate.
	maxInitiateAttempts = 10

	// initiateAttemptDelay is the amount of time to sleep between failed
	// attempts to replSetInitiate.
	initiateAttemptDelay = 100 * time.Millisecond

	// maxInitiateStatusAttempts is the maximum number of attempts
	// to get the replication status after Initiate.
	maxInitiateStatusAttempts = 50

	// initiateAttemptStatusDelay is the amount of time to sleep between failed
	// attempts to replSetGetStatus.
	initiateAttemptStatusDelay = 500 * time.Millisecond

	// rsMembersUnreachableError is the error message returned from mongo
	// when it thinks that replicaset members are unreachable. This can
	// occur if replSetInitiate is executed shortly after starting up mongo.
	rsMembersUnreachableError = "all members and seeds must be reachable to initiate set"
)

var logger = loggo.GetLogger("juju.replicaset")

var (
	getCurrentStatus = CurrentStatus
	isReady          = IsReady
)

// Initiate sets up a replica set with the given replica set name with the
// single given member.  It need be called only once for a given mongo replica
// set.  The tags specified will be added as tags on the member that is created
// in the replica set.
//
// Note that you must set DialWithInfo and set Direct = true when dialing into a
// specific non-initiated mongo server.
//
// See http://docs.mongodb.org/manual/reference/method/rs.initiate/ for more
// details.
func Initiate(session *mgo.Session, address, name string, tags map[string]string) error {
	monotonicSession := session.Clone()
	defer monotonicSession.Close()
	monotonicSession.SetMode(mgo.Monotonic, true)
	cfg := Config{
		Name:    name,
		Version: 1,
		Members: []Member{{
			Id:      1,
			Address: fixIpv6Address(address),
			Tags:    tags,
		}},
	}
	logger.Infof("Initiating replicaset with config %#v", cfg)
	var err error
	for i := 0; i < maxInitiateAttempts; i++ {
		monotonicSession.Refresh()
		err = monotonicSession.Run(bson.D{{"replSetInitiate", cfg}}, nil)
		if err != nil && err.Error() == rsMembersUnreachableError {
			time.Sleep(initiateAttemptDelay)
			continue
		}
		break
	}

	// Wait for replSetInitiate to complete. Even if err != nil,
	// it may be that replSetInitiate is still in progress, so
	// attempt CurrentStatus.
	for i := 0; i < maxInitiateStatusAttempts; i++ {
		monotonicSession.Refresh()
		var status *Status
		status, err = getCurrentStatus(monotonicSession)
		if err != nil {
			logger.Warningf("Initiate: fetching replication status failed: %v", err)
		}
		if err != nil || len(status.Members) == 0 {
			time.Sleep(initiateAttemptStatusDelay)
			continue
		}
		break
	}
	return err
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

func fmtConfigForLog(config *Config) string {
	memberInfo := make([]string, len(config.Members))
	for i, member := range config.Members {
		memberInfo[i] = fmt.Sprintf("Member{%d %q %v}", member.Id, member.Address, member.Tags)

	}
	return fmt.Sprintf("{Name: %s, Version: %d, Members: {%s}}",
		config.Name, config.Version, strings.Join(memberInfo, ", "))
}

// applyReplSetConfig applies the new config to the mongo session. It also logs
// what the changes are. It checks if the replica set changes cause the DB
// connection to be dropped. If so, it Refreshes the session and tries to Ping
// again.
func applyReplSetConfig(cmd string, session *mgo.Session, oldconfig, newconfig *Config) error {
	logger.Debugf("%s() changing replica set\nfrom %s\n  to %s",
		cmd, fmtConfigForLog(oldconfig), fmtConfigForLog(newconfig))

	// newConfig here is internal and safe to mutate
	for index, member := range newconfig.Members {
		newconfig.Members[index].Address = fixIpv6Address(member.Address)
	}
	err := session.Run(bson.D{{"replSetReconfig", newconfig}}, nil)
	if err == io.EOF {
		// If the primary changes due to replSetReconfig, then all
		// current connections are dropped.
		// Refreshing should fix us up.
		logger.Debugf("got EOF while running %s(), calling session.Refresh()", cmd)
		session.Refresh()
	} else if err != nil {
		// For all errors that aren't EOF, return immediately
		return err
	}
	err = nil
	// We will only try to Ping 2 times
	for i := 0; i < 2; i++ {
		// err was either nil, or EOF and we called Refresh, so Ping to
		// make sure we're actually connected
		err = session.Ping()
		if err == nil {
			break
		}
	}
	return err
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

	oldconfig := *config
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
	return applyReplSetConfig("Add", session, &oldconfig, config)
}

// Remove removes members with the given addresses from the replica set. It is
// not an error to remove addresses of non-existent replica set members.
func Remove(session *mgo.Session, addrs ...string) error {
	config, err := CurrentConfig(session)
	if err != nil {
		return err
	}
	oldconfig := *config
	config.Version++
	for _, rem := range addrs {
		for n, repl := range config.Members {
			if repl.Address == rem {
				config.Members = append(config.Members[:n], config.Members[n+1:]...)
				break
			}
		}
	}
	return applyReplSetConfig("Remove", session, &oldconfig, config)
}

// Set changes the current set of replica set members.  Members will have their
// ids set automatically if their ids are not already > 0.
func Set(session *mgo.Session, members []Member) error {
	config, err := CurrentConfig(session)
	if err != nil {
		return err
	}

	// Copy the current configuration for logging
	oldconfig := *config
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

	return applyReplSetConfig("Set", session, &oldconfig, config)
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

	results.Address = unFixIpv6Address(results.Address)
	results.PrimaryAddress = unFixIpv6Address(results.PrimaryAddress)
	for index, address := range results.Addresses {
		results.Addresses[index] = unFixIpv6Address(address)
	}
	return results, nil
}

var ErrMasterNotConfigured = fmt.Errorf("mongo master not configured")

// MasterHostPort returns the "address:port" string for the primary
// mongo server in the replicaset. It returns ErrMasterNotConfigured if
// the replica set has not yet been initiated.
func MasterHostPort(session *mgo.Session) (string, error) {
	results, err := IsMaster(session)
	if err != nil {
		return "", err
	}
	if results.PrimaryAddress == "" {
		return "", ErrMasterNotConfigured
	}
	return results.PrimaryAddress, nil
}

// CurrentMembers returns the current members of the replica set.
func CurrentMembers(session *mgo.Session) ([]Member, error) {
	cfg, err := CurrentConfig(session)
	if err != nil {
		return nil, err
	}
	return cfg.Members, nil
}

// CurrentConfig returns the Config for the given session's replica set.  If
// there is no current config, the error returned will be mgo.ErrNotFound.
func CurrentConfig(session *mgo.Session) (*Config, error) {
	cfg := &Config{}
	monotonicSession := session.Clone()
	defer monotonicSession.Close()
	monotonicSession.SetMode(mgo.Monotonic, true)
	err := monotonicSession.DB("local").C("system.replset").Find(nil).One(cfg)
	if err == mgo.ErrNotFound {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get replset config: %s", err.Error())
	}

	members := make([]Member, len(cfg.Members), len(cfg.Members))
	for index, member := range cfg.Members {
		member.Address = unFixIpv6Address(member.Address)
		members[index] = member
	}
	cfg.Members = members
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
		return nil, fmt.Errorf("cannot get replica set status: %v", err)
	}

	for index, member := range status.Members {
		status.Members[index].Address = unFixIpv6Address(member.Address)
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

// IsReady checks on the status of all members in the replicaset
// associated with the provided session. If we can connect and the majority of
// members are ready then the result is true.
func IsReady(session *mgo.Session) (bool, error) {
	status, err := getCurrentStatus(session)
	if isConnectionNotAvailable(err) {
		// The connection dropped...
		logger.Errorf("DB connection dropped so reconnecting")
		session.Refresh()
		return false, nil
	}
	if err != nil {
		// Fail for any other reason.
		return false, errors.Trace(err)
	}

	majority := (len(status.Members) / 2) + 1
	healthy := 0
	// Check the members.
	for _, member := range status.Members {
		if member.Healthy {
			healthy += 1
		}
	}
	if healthy < majority {
		logger.Errorf("not enough members ready")
		return false, nil
	}
	return true, nil
}

var connectionErrors = []syscall.Errno{
	syscall.ECONNABORTED, // "software caused connection abort"
	syscall.ECONNREFUSED, // "connection refused"
	syscall.ECONNRESET,   // "connection reset by peer"
	syscall.ENETRESET,    // "network dropped connection on reset"
	syscall.ETIMEDOUT,    // "connection timed out"
}

func isConnectionNotAvailable(err error) bool {
	if err == nil {
		return false
	}
	// mgo returns io.EOF from session operations when the connection
	// has been dropped.
	if errors.Cause(err) == io.EOF {
		return true
	}
	// An errno may be returned so we check the connection-related ones.
	for _, errno := range connectionErrors {
		if errors.Cause(err) == errno {
			return true
		}
	}
	return false
}

// WaitUntilReady waits until all members of the replicaset are ready.
// It will retry every 10 seconds until the timeout is reached. Dropped
// connections will trigger a reconnect.
func WaitUntilReady(session *mgo.Session, timeout int) error {
	attempts := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Total: time.Duration(timeout) * time.Second,
	}
	var err error
	ready := false
	for a := attempts.Start(); !ready && a.Next(); {
		ready, err = isReady(session)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if !ready {
		return errors.Errorf("timed out after %d seconds", timeout)
	}
	return nil
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

// Turn normal ipv6 addresses into the "bad format" that mongo requires us
// to use. (Mongo can't parse square brackets in ipv6 addresses.)
func fixIpv6Address(address string) string {
	address = strings.Replace(address, "[", "", 1)
	address = strings.Replace(address, "]", "", 1)
	return address
}

// Turn "bad format" ipv6 addresses ("::1:port"), that mongo uses,  into good
// format addresses ("[::1]:port").
func unFixIpv6Address(address string) string {
	if strings.Count(address, ":") >= 2 && strings.Count(address, "[") == 0 {
		lastColon := strings.LastIndex(address, ":")
		host := address[:lastColon]
		port := address[lastColon+1:]
		return fmt.Sprintf("[%s]:%s", host, port)
	}
	return address
}
