// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
	annotator
	presence.Presencer
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob int

const (
	_ MachineJob = iota
	JobHostUnits
	JobManageEnviron
	JobManageNetworking

	// Deprecated in 1.18.
	JobManageStateDeprecated
)

var jobNames = map[MachineJob]multiwatcher.MachineJob{
	JobHostUnits:        multiwatcher.JobHostUnits,
	JobManageEnviron:    multiwatcher.JobManageEnviron,
	JobManageNetworking: multiwatcher.JobManageNetworking,

	// Deprecated in 1.18.
	JobManageStateDeprecated: multiwatcher.JobManageStateDeprecated,
}

// AllJobs returns all supported machine jobs.
func AllJobs() []MachineJob {
	return []MachineJob{
		JobHostUnits,
		JobManageEnviron,
		JobManageNetworking,
	}
}

// ToParams returns the job as multiwatcher.MachineJob.
func (job MachineJob) ToParams() multiwatcher.MachineJob {
	if jujuJob, ok := jobNames[job]; ok {
		return jujuJob
	}
	return multiwatcher.MachineJob(fmt.Sprintf("<unknown job %d>", int(job)))
}

// params.JobsFromJobs converts state jobs to juju jobs.
func paramsJobsFromJobs(jobs []MachineJob) []multiwatcher.MachineJob {
	jujuJobs := make([]multiwatcher.MachineJob, len(jobs))
	for i, machineJob := range jobs {
		jujuJobs[i] = machineJob.ToParams()
	}
	return jujuJobs
}

func (job MachineJob) String() string {
	return string(job.ToParams())
}

// manualMachinePrefix signals as prefix of Nonce that a machine is
// manually provisioned.
const manualMachinePrefix = "manual:"

// machineDoc represents the internal state of a machine in MongoDB.
// Note the correspondence with MachineInfo in apiserver/juju.
type machineDoc struct {
	DocID         string `bson:"_id"`
	Id            string `bson:"machineid"`
	EnvUUID       string `bson:"env-uuid"`
	Nonce         string
	Series        string
	ContainerType string
	Principals    []string
	Life          Life
	Tools         *tools.Tools `bson:",omitempty"`
	Jobs          []MachineJob
	NoVote        bool
	HasVote       bool
	PasswordHash  string
	Clean         bool
	// We store 2 different sets of addresses for the machine, obtained
	// from different sources.
	// Addresses is the set of addresses obtained by asking the provider.
	Addresses []address
	// MachineAddresses is the set of addresses obtained from the machine itself.
	MachineAddresses []address
	// The SupportedContainers attributes are used to advertise what containers this
	// machine is capable of hosting.
	SupportedContainersKnown bool
	SupportedContainers      []instance.ContainerType `bson:",omitempty"`
	// Placement is the placement directive that should be used when provisioning
	// an instance for the machine.
	Placement string `bson:",omitempty"`
}

func newMachine(st *State, doc *machineDoc) *Machine {
	machine := &Machine{
		st:  st,
		doc: *doc,
	}
	machine.annotator = annotator{
		globalKey: machine.globalKey(),
		tag:       machine.Tag(),
		st:        st,
	}
	return machine
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// Series returns the operating system series running on the machine.
func (m *Machine) Series() string {
	return m.doc.Series
}

// ContainerType returns the type of container hosting this machine.
func (m *Machine) ContainerType() instance.ContainerType {
	return instance.ContainerType(m.doc.ContainerType)
}

// machineGlobalKey returns the global database key for the identified machine.
func machineGlobalKey(id string) string {
	return "m#" + id
}

// globalKey returns the global database key for the machine.
func (m *Machine) globalKey() string {
	return machineGlobalKey(m.doc.Id)
}

// instanceData holds attributes relevant to a provisioned machine.
type instanceData struct {
	DocID      string      `bson:"_id"`
	MachineId  string      `bson:"machineid"`
	InstanceId instance.Id `bson:"instanceid"`
	EnvUUID    string      `bson:"env-uuid"`
	Status     string      `bson:"status,omitempty"`
	Arch       *string     `bson:"arch,omitempty"`
	Mem        *uint64     `bson:"mem,omitempty"`
	RootDisk   *uint64     `bson:"rootdisk,omitempty"`
	CpuCores   *uint64     `bson:"cpucores,omitempty"`
	CpuPower   *uint64     `bson:"cpupower,omitempty"`
	Tags       *[]string   `bson:"tags,omitempty"`
	AvailZone  *string     `bson:"availzone,omitempty"`
}

func hardwareCharacteristics(instData instanceData) *instance.HardwareCharacteristics {
	return &instance.HardwareCharacteristics{
		Arch:             instData.Arch,
		Mem:              instData.Mem,
		RootDisk:         instData.RootDisk,
		CpuCores:         instData.CpuCores,
		CpuPower:         instData.CpuPower,
		Tags:             instData.Tags,
		AvailabilityZone: instData.AvailZone,
	}
}

// TODO(wallyworld): move this method to a service.
func (m *Machine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if err != nil {
		return nil, err
	}
	return hardwareCharacteristics(instData), nil
}

func getInstanceData(st *State, id string) (instanceData, error) {
	instanceDataCollection, closer := st.getCollection(instanceDataC)
	defer closer()

	var instData instanceData
	err := instanceDataCollection.FindId(id).One(&instData)
	if err == mgo.ErrNotFound {
		return instanceData{}, errors.NotFoundf("instance data for machine %v", id)
	}
	if err != nil {
		return instanceData{}, fmt.Errorf("cannot get instance data for machine %v: %v", id, err)
	}
	return instData, nil
}

// Tag returns a tag identifying the machine. The String method provides a
// string representation that is safe to use as a file name. The returned name
// will be different from other Tag values returned by any other entities
// from the same state.
func (m *Machine) Tag() names.Tag {
	return names.NewMachineTag(m.Id())
}

// Life returns whether the machine is Alive, Dying or Dead.
func (m *Machine) Life() Life {
	return m.doc.Life
}

// Jobs returns the responsibilities that must be fulfilled by m's agent.
func (m *Machine) Jobs() []MachineJob {
	return m.doc.Jobs
}

// WantsVote reports whether the machine is a state server
// that wants to take part in peer voting.
func (m *Machine) WantsVote() bool {
	return hasJob(m.doc.Jobs, JobManageEnviron) && !m.doc.NoVote
}

// HasVote reports whether that machine is currently a voting
// member of the replica set.
func (m *Machine) HasVote() bool {
	return m.doc.HasVote
}

// SetHasVote sets whether the machine is currently a voting
// member of the replica set. It should only be called
// from the worker that maintains the replica set.
func (m *Machine) SetHasVote(hasVote bool) error {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"hasvote", hasVote}}}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set HasVote of machine %v: %v", m, onAbort(err, ErrDead))
	}
	m.doc.HasVote = hasVote
	return nil
}

// IsManager returns true if the machine has JobManageEnviron.
func (m *Machine) IsManager() bool {
	return hasJob(m.doc.Jobs, JobManageEnviron)
}

// IsManual returns true if the machine was manually provisioned.
func (m *Machine) IsManual() (bool, error) {
	// Apart from the bootstrap machine, manually provisioned
	// machines have a nonce prefixed with "manual:". This is
	// unique to manual provisioning.
	if strings.HasPrefix(m.doc.Nonce, manualMachinePrefix) {
		return true, nil
	}
	// The bootstrap machine uses BootstrapNonce, so in that
	// case we need to check if its provider type is "manual".
	// We also check for "null", which is an alias for manual.
	if m.doc.Id == "0" {
		cfg, err := m.st.EnvironConfig()
		if err != nil {
			return false, err
		}
		t := cfg.Type()
		return t == "null" || t == "manual", nil
	}
	return false, nil
}

// AgentTools returns the tools that the agent is currently running.
// It returns an error that satisfies errors.IsNotFound if the tools
// have not yet been set.
func (m *Machine) AgentTools() (*tools.Tools, error) {
	if m.doc.Tools == nil {
		return nil, errors.NotFoundf("agent tools for machine %v", m)
	}
	tools := *m.doc.Tools
	return &tools, nil
}

// checkVersionValidity checks whether the given version is suitable
// for passing to SetAgentVersion.
func checkVersionValidity(v version.Binary) error {
	if v.Series == "" || v.Arch == "" {
		return fmt.Errorf("empty series or arch")
	}
	return nil
}

// SetAgentVersion sets the version of juju that the agent is
// currently running.
func (m *Machine) SetAgentVersion(v version.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set agent version for machine %v", m)
	if err = checkVersionValidity(v); err != nil {
		return err
	}
	tools := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"tools", tools}}}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return onAbort(err, ErrDead)
	}
	m.doc.Tools = tools
	return nil
}

// SetMongoPassword sets the password the agent responsible for the machine
// should use to communicate with the state servers.  Previous passwords
// are invalidated.
func (m *Machine) SetMongoPassword(password string) error {
	if !m.IsManager() {
		return errors.NotSupportedf("setting mongo password for non-state server machine %v", m)
	}
	return mongo.SetAdminMongoPassword(m.st.db.Session, m.Tag().String(), password)
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	if len(password) < utils.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	return m.setPasswordHash(utils.AgentPasswordHash(password))
}

// setPasswordHash sets the underlying password hash in the database directly
// to the value supplied. This is split out from SetPassword to allow direct
// manipulation in tests (to check for backwards compatibility).
func (m *Machine) setPasswordHash(passwordHash string) error {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set password of machine %v: %v", m, onAbort(err, ErrDead))
	}
	m.doc.PasswordHash = passwordHash
	return nil
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func (m *Machine) getPasswordHash() string {
	return m.doc.PasswordHash
}

// PasswordValid returns whether the given password is valid
// for the given machine.
func (m *Machine) PasswordValid(password string) bool {
	agentHash := utils.AgentPasswordHash(password)
	if agentHash == m.doc.PasswordHash {
		return true
	}
	// In Juju 1.16 and older we used the slower password hash for unit
	// agents. So check to see if the supplied password matches the old
	// path, and if so, update it to the new mechanism.
	// We ignore any error in setting the password, as we'll just try again
	// next time
	if utils.UserPasswordHash(password, utils.CompatSalt) == m.doc.PasswordHash {
		logger.Debugf("%s logged in with old password hash, changing to AgentPasswordHash",
			m.Tag())
		m.setPasswordHash(agentHash)
		return true
	}
	return false
}

// Destroy sets the machine lifecycle to Dying if it is Alive. It does
// nothing otherwise. Destroy will fail if the machine has principal
// units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, Destroy will return
// a HasAssignedUnitsError.
func (m *Machine) Destroy() error {
	return m.advanceLifecycle(Dying)
}

// ForceDestroy queues the machine for complete removal, including the
// destruction of all units and containers on the machine.
func (m *Machine) ForceDestroy() error {
	if !m.IsManager() {
		ops := []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageEnviron}}}}},
		}, m.st.newCleanupOp(cleanupForceDestroyedMachine, m.doc.Id)}
		if err := m.st.runTransaction(ops); err != txn.ErrAborted {
			return err
		}
	}
	return fmt.Errorf("machine %s is required by the environment", m.doc.Id)
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, EnsureDead will return
// a HasAssignedUnitsError.
func (m *Machine) EnsureDead() error {
	return m.advanceLifecycle(Dead)
}

type HasAssignedUnitsError struct {
	MachineId string
	UnitNames []string
}

func (e *HasAssignedUnitsError) Error() string {
	return fmt.Sprintf("machine %s has unit %q assigned", e.MachineId, e.UnitNames[0])
}

func IsHasAssignedUnitsError(err error) bool {
	_, ok := err.(*HasAssignedUnitsError)
	return ok
}

// Containers returns the container ids belonging to a parent machine.
// TODO(wallyworld): move this method to a service
func (m *Machine) Containers() ([]string, error) {
	containerRefs, closer := m.st.getCollection(containerRefsC)
	defer closer()

	var mc machineContainers
	err := containerRefs.FindId(m.doc.DocID).One(&mc)
	if err == nil {
		return mc.Children, nil
	}
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("container info for machine %v", m.Id())
	}
	return nil, err
}

// ParentId returns the Id of the host machine if this machine is a container.
func (m *Machine) ParentId() (string, bool) {
	parentId := ParentId(m.Id())
	return parentId, parentId != ""
}

type HasContainersError struct {
	MachineId    string
	ContainerIds []string
}

func (e *HasContainersError) Error() string {
	return fmt.Sprintf("machine %s is hosting containers %q", e.MachineId, strings.Join(e.ContainerIds, ","))
}

func IsHasContainersError(err error) bool {
	_, ok := err.(*HasContainersError)
	return ok
}

// advanceLifecycle ensures that the machine's lifecycle is no earlier
// than the supplied value. If the machine already has that lifecycle
// value, or a later one, no changes will be made to remote state. If
// the machine has any responsibilities that preclude a valid change in
// lifecycle, it will return an error.
func (original *Machine) advanceLifecycle(life Life) (err error) {
	containers, err := original.Containers()
	if err != nil {
		return err
	}
	if len(containers) > 0 {
		return &HasContainersError{
			MachineId:    original.doc.Id,
			ContainerIds: containers,
		}
	}
	m := original
	defer func() {
		if err == nil {
			// The machine's lifecycle is known to have advanced; it may be
			// known to have already advanced further than requested, in
			// which case we set the latest known valid value.
			if m == nil {
				life = Dead
			} else if m.doc.Life > life {
				life = m.doc.Life
			}
			original.doc.Life = life
		}
	}()
	// op and
	op := txn.Op{
		C:      machinesC,
		Id:     m.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"life", life}}}},
	}
	advanceAsserts := bson.D{
		{"jobs", bson.D{{"$nin", []MachineJob{JobManageEnviron}}}},
		{"$or", []bson.D{
			{{"principals", bson.D{{"$size", 0}}}},
			{{"principals", bson.D{{"$exists", false}}}},
		}},
		{"hasvote", bson.D{{"$ne", true}}},
	}
	// multiple attempts: one with original data, one with refreshed data, and a final
	// one intended to determine the cause of failure of the preceding attempt.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If the transaction was aborted, grab a fresh copy of the machine data.
		// We don't write to original, because the expectation is that state-
		// changing methods only set the requested change on the receiver; a case
		// could perhaps be made that this is not a helpful convention in the
		// context of the new state API, but we maintain consistency in the
		// face of uncertainty.
		if attempt != 0 {
			if m, err = m.st.Machine(m.doc.Id); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		// Check that the life change is sane, and collect the assertions
		// necessary to determine that it remains so.
		switch life {
		case Dying:
			if m.doc.Life != Alive {
				return nil, jujutxn.ErrNoOperations
			}
			op.Assert = append(advanceAsserts, isAliveDoc...)
		case Dead:
			if m.doc.Life == Dead {
				return nil, jujutxn.ErrNoOperations
			}
			op.Assert = append(advanceAsserts, notDeadDoc...)
		default:
			panic(fmt.Errorf("cannot advance lifecycle to %v", life))
		}
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		if hasJob(m.doc.Jobs, JobManageEnviron) {
			// (NOTE: When we enable multiple JobManageEnviron machines,
			// this restriction will be lifted, but we will assert that the
			// machine is not voting)
			return nil, fmt.Errorf("machine %s is required by the environment", m.doc.Id)
		}
		if m.doc.HasVote {
			return nil, fmt.Errorf("machine %s is a voting replica set member", m.doc.Id)
		}
		if len(m.doc.Principals) != 0 {
			return nil, &HasAssignedUnitsError{
				MachineId: m.doc.Id,
				UnitNames: m.doc.Principals,
			}
		}
		return []txn.Op{op}, nil
	}
	if err = m.st.run(buildTxn); err == jujutxn.ErrExcessiveContention {
		err = errors.Annotatef(err, "machine %s cannot advance lifecycle", m)
	}
	return err
}

func (m *Machine) removePortsOps() ([]txn.Op, error) {
	if m.doc.Life != Dead {
		return nil, errors.Errorf("machine is not dead")
	}
	ports, err := m.AllPorts()
	if err != nil {
		return nil, err
	}
	var ops []txn.Op
	for _, p := range ports {
		ops = append(ops, p.removeOps()...)
	}
	return ops, nil
}

func (m *Machine) removeNetworkInterfacesOps() ([]txn.Op, error) {
	if m.doc.Life != Dead {
		return nil, errors.Errorf("machine is not dead")
	}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: isDeadDoc,
	}}
	sel := bson.D{{"machineid", m.doc.Id}}
	networkInterfaces, closer := m.st.getCollection(networkInterfacesC)
	defer closer()

	iter := networkInterfaces.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
	var doc networkInterfaceDoc
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      networkInterfacesC,
			Id:     doc.Id,
			Remove: true,
		})
	}
	return ops, iter.Close()
}

// Remove removes the machine from state. It will fail if the machine
// is not Dead.
func (m *Machine) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove machine %s", m.doc.Id)
	if m.doc.Life != Dead {
		return fmt.Errorf("machine is not dead")
	}
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: txn.DocExists,
			Remove: true,
		},
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: isDeadDoc,
		},
		{
			C:      instanceDataC,
			Id:     m.doc.DocID,
			Remove: true,
		},
		removeStatusOp(m.st, m.globalKey()),
		removeConstraintsOp(m.st, m.globalKey()),
		removeBlockDevicesOp(m.st, m.Id()),
		removeRequestedNetworksOp(m.st, m.globalKey()),
		annotationRemoveOp(m.st, m.globalKey()),
		removeRebootDocOp(m.st, m.globalKey()),
	}
	ifacesOps, err := m.removeNetworkInterfacesOps()
	if err != nil {
		return err
	}
	portsOps, err := m.removePortsOps()
	if err != nil {
		return err
	}
	ops = append(ops, ifacesOps...)
	ops = append(ops, portsOps...)
	ops = append(ops, removeContainerRefOps(m.st, m.Id())...)
	// The only abort conditions in play indicate that the machine has already
	// been removed.
	return onAbort(m.st.runTransaction(ops), nil)
}

// Refresh refreshes the contents of the machine from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// machine has been removed.
func (m *Machine) Refresh() error {
	machines, closer := m.st.getCollection(machinesC)
	defer closer()

	var doc machineDoc
	err := machines.FindId(m.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("machine %v", m)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh machine %v", m)
	}
	m.doc = doc
	return nil
}

// AgentPresence returns whether the respective remote agent is alive.
func (m *Machine) AgentPresence() (bool, error) {
	b, err := m.st.pwatcher.Alive(m.globalKey())
	return b, err
}

// WaitAgentPresence blocks until the respective agent is alive.
func (m *Machine) WaitAgentPresence(timeout time.Duration) (err error) {
	defer errors.DeferredAnnotatef(&err, "waiting for agent of machine %v", m)
	ch := make(chan presence.Change)
	m.st.pwatcher.Watch(m.globalKey(), ch)
	defer m.st.pwatcher.Unwatch(m.globalKey(), ch)
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("still not alive after timeout")
		case <-m.st.pwatcher.Dead():
			return m.st.pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for machine %v", m))
}

// SetAgentPresence signals that the agent for machine m is alive.
// It returns the started pinger.
func (m *Machine) SetAgentPresence() (*presence.Pinger, error) {
	presenceCollection := m.st.getPresence()
	p := presence.NewPinger(presenceCollection, m.globalKey())
	err := p.Start()
	if err != nil {
		return nil, err
	}
	// We preform a manual sync here so that the
	// presence pinger has the most up-to-date information when it
	// starts. This ensures that commands run immediately after bootstrap
	// like status or ensure-availability will have an accurate values
	// for agent-state.
	//
	// TODO: Does not work for multiple state servers. Trigger a sync across all state servers.
	if m.IsManager() {
		m.st.pwatcher.Sync()
	}
	return p, nil
}

// InstanceId returns the provider specific instance id for this
// machine, or a NotProvisionedError, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		err = NotProvisionedError(m.Id())
	}
	if err != nil {
		return "", err
	}
	return instData.InstanceId, err
}

// InstanceStatus returns the provider specific instance status for this machine,
// or a NotProvisionedError if instance is not yet provisioned.
func (m *Machine) InstanceStatus() (string, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		err = NotProvisionedError(m.Id())
	}
	if err != nil {
		return "", err
	}
	return instData.Status, err
}

// SetInstanceStatus sets the provider specific instance status for a machine.
func (m *Machine) SetInstanceStatus(status string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set instance status for machine %q", m)

	ops := []txn.Op{
		{
			C:      instanceDataC,
			Id:     m.doc.DocID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"status", status}}}},
		},
	}

	if err = m.st.runTransaction(ops); err == nil {
		return nil
	} else if err != txn.ErrAborted {
		return err
	}
	return NotProvisionedError(m.Id())
}

// AvailabilityZone returns the provier-specific instance availability
// zone in which the machine was provisioned.
func (m *Machine) AvailabilityZone() (string, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		return "", errors.Trace(NotProvisionedError(m.Id()))
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	var zone string
	if instData.AvailZone != nil {
		zone = *instData.AvailZone
	}
	return zone, nil
}

// Units returns all the units that have been assigned to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get units assigned to machine %v", m)
	unitsCollection, closer := m.st.getCollection(unitsC)
	defer closer()

	pudocs := []unitDoc{}
	err = unitsCollection.Find(bson.D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, &pudoc))
		docs := []unitDoc{}
		err = unitsCollection.Find(bson.D{{"principal", pudoc.Name}}).All(&docs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			units = append(units, newUnit(m.st, &doc))
		}
	}
	return units, nil
}

// SetProvisioned sets the provider specific machine id, nonce and also metadata for
// this machine. Once set, the instance id cannot be changed.
//
// When provisioning an instance, a nonce should be created and passed
// when starting it, before adding the machine to the state. This means
// that if the provisioner crashes (or its connection to the state is
// lost) after starting the instance, we can be sure that only a single
// instance will be able to act for that machine.
func (m *Machine) SetProvisioned(id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set instance data for machine %q", m)

	if id == "" || nonce == "" {
		return fmt.Errorf("instance id and nonce cannot be empty")
	}

	if characteristics == nil {
		characteristics = &instance.HardwareCharacteristics{}
	}
	instData := &instanceData{
		DocID:      m.doc.DocID,
		MachineId:  m.doc.Id,
		InstanceId: id,
		EnvUUID:    m.doc.EnvUUID,
		Arch:       characteristics.Arch,
		Mem:        characteristics.Mem,
		RootDisk:   characteristics.RootDisk,
		CpuCores:   characteristics.CpuCores,
		CpuPower:   characteristics.CpuPower,
		Tags:       characteristics.Tags,
		AvailZone:  characteristics.AvailabilityZone,
	}

	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: append(isAliveDoc, bson.DocElem{"nonce", ""}),
			Update: bson.D{{"$set", bson.D{{"nonce", nonce}}}},
		}, {
			C:      instanceDataC,
			Id:     m.doc.DocID,
			Assert: txn.DocMissing,
			Insert: instData,
		},
	}

	if err = m.st.runTransaction(ops); err == nil {
		m.doc.Nonce = nonce
		return nil
	} else if err != txn.ErrAborted {
		return err
	} else if alive, err := isAlive(m.st, machinesC, m.doc.DocID); err != nil {
		return err
	} else if !alive {
		return errNotAlive
	}
	return fmt.Errorf("already set")
}

// SetInstanceInfo is used to provision a machine and in one steps set
// it's instance id, nonce, hardware characteristics, add networks and
// network interfaces as needed.
//
// TODO(dimitern) Do all the operations described in a single
// transaction, rather than using separate calls. Alternatively,
// we can add all the things to create/set in a document in some
// collection and have a worker that takes care of the actual work.
// Merge SetProvisioned() in here or drop it at that point.
func (m *Machine) SetInstanceInfo(
	id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics,
	networks []NetworkInfo, interfaces []NetworkInterfaceInfo) error {

	// Add the networks and interfaces first.
	for _, network := range networks {
		_, err := m.st.AddNetwork(network)
		if err != nil && errors.IsAlreadyExists(err) {
			// Ignore already existing networks.
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	for _, iface := range interfaces {
		_, err := m.AddNetworkInterface(iface)
		if err != nil && errors.IsAlreadyExists(err) {
			// Ignore already existing network interfaces.
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return m.SetProvisioned(id, nonce, characteristics)
}

// notProvisionedError records an error when a machine is not provisioned.
type notProvisionedError struct {
	machineId string
}

func NotProvisionedError(machineId string) error {
	return &notProvisionedError{machineId}
}

func (e *notProvisionedError) Error() string {
	return fmt.Sprintf("machine %v is not provisioned", e.machineId)
}

// IsNotProvisionedError returns true if err is a notProvisionedError.
func IsNotProvisionedError(err error) bool {
	_, ok := err.(*notProvisionedError)
	return ok
}

func mergedAddresses(machineAddresses, providerAddresses []address) []network.Address {
	merged := make([]network.Address, 0, len(providerAddresses)+len(machineAddresses))
	providerValues := set.NewStrings()
	for _, address := range providerAddresses {
		// Older versions of Juju may have stored an empty address so ignore it here.
		if address.Value == "" || providerValues.Contains(address.Value) {
			continue
		}
		providerValues.Add(address.Value)
		merged = append(merged, address.InstanceAddress())
	}
	for _, address := range machineAddresses {
		if !providerValues.Contains(address.Value) {
			merged = append(merged, address.InstanceAddress())
		}
	}
	return merged
}

// Addresses returns any hostnames and ips associated with a machine,
// determined both by the machine itself, and by asking the provider.
//
// The addresses returned by the provider shadow any of the addresses
// that the machine reported with the same address value.
// Provider-reported addresses always come before machine-reported
// addresses. Duplicates are removed.
func (m *Machine) Addresses() (addresses []network.Address) {
	return mergedAddresses(m.doc.MachineAddresses, m.doc.Addresses)
}

// SetAddresses records any addresses related to the machine, sourced
// by asking the provider.
func (m *Machine) SetAddresses(addresses ...network.Address) (err error) {
	if err = m.setAddresses(addresses, &m.doc.Addresses, "addresses"); err != nil {
		return fmt.Errorf("cannot set addresses of machine %v: %v", m, err)
	}
	return nil
}

// MachineAddresses returns any hostnames and ips associated with a machine,
// determined by asking the machine itself.
func (m *Machine) MachineAddresses() (addresses []network.Address) {
	for _, address := range m.doc.MachineAddresses {
		addresses = append(addresses, address.InstanceAddress())
	}
	return
}

// SetMachineAddresses records any addresses related to the machine, sourced
// by asking the machine.
func (m *Machine) SetMachineAddresses(addresses ...network.Address) (err error) {
	if err = m.setAddresses(addresses, &m.doc.MachineAddresses, "machineaddresses"); err != nil {
		return fmt.Errorf("cannot set machine addresses of machine %v: %v", m, err)
	}
	return nil
}

// setAddresses updates the machine's addresses (either Addresses or
// MachineAddresses, depending on the field argument).
func (m *Machine) setAddresses(addresses []network.Address, field *[]address, fieldName string) error {
	var changed bool
	envConfig, err := m.st.EnvironConfig()
	if err != nil {
		return err
	}
	network.SortAddresses(addresses, envConfig.PreferIPv6())
	stateAddresses := instanceAddressesToAddresses(addresses)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		changed = false
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, err
			}
		}
		if m.doc.Life == Dead {
			return nil, ErrDead
		}
		op := txn.Op{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: append(bson.D{{fieldName, *field}}, notDeadDoc...),
		}
		if !addressesEqual(addresses, addressesToInstanceAddresses(*field)) {
			op.Update = bson.D{{"$set", bson.D{{fieldName, stateAddresses}}}}
			changed = true
		}
		return []txn.Op{op}, nil
	}
	switch err := m.st.run(buildTxn); err {
	case nil:
	case jujutxn.ErrExcessiveContention:
		return errors.Annotatef(err, "cannot set %s for machine %s", fieldName, m)
	default:
		return err
	}
	if !changed {
		return nil
	}
	*field = stateAddresses
	return nil
}

// RequestedNetworks returns the list of network names the machine
// should be on. Unlike networks specified with constraints, these
// networks are required to be present on the machine.
func (m *Machine) RequestedNetworks() ([]string, error) {
	return readRequestedNetworks(m.st, m.globalKey())
}

// Networks returns the list of configured networks on the machine.
// The configured and requested networks on a machine must match.
func (m *Machine) Networks() ([]*Network, error) {
	requestedNetworks, err := m.RequestedNetworks()
	if err != nil {
		return nil, err
	}
	docs := []networkDoc{}

	networksCollection, closer := m.st.getCollection(networksC)
	defer closer()

	sel := bson.D{{"name", bson.D{{"$in", requestedNetworks}}}}
	err = networksCollection.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	networks := make([]*Network, len(docs))
	for i, doc := range docs {
		networks[i] = newNetwork(m.st, &doc)
	}
	return networks, nil
}

// NetworkInterfaces returns the list of configured network interfaces
// of the machine.
func (m *Machine) NetworkInterfaces() ([]*NetworkInterface, error) {
	networkInterfaces, closer := m.st.getCollection(networkInterfacesC)
	defer closer()

	docs := []networkInterfaceDoc{}
	err := networkInterfaces.Find(bson.D{{"machineid", m.doc.Id}}).All(&docs)
	if err != nil {
		return nil, err
	}
	ifaces := make([]*NetworkInterface, len(docs))
	for i, doc := range docs {
		ifaces[i] = newNetworkInterface(m.st, &doc)
	}
	return ifaces, nil
}

// AddNetworkInterface creates a new network interface with the given
// args for this machine. The machine must be alive and not yet
// provisioned, and there must be no other interface with the same MAC
// address on the same network, or the same name on that machine for
// this to succeed. If a network interface already exists, the
// returned error satisfies errors.IsAlreadyExists.
func (m *Machine) AddNetworkInterface(args NetworkInterfaceInfo) (iface *NetworkInterface, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add network interface %q to machine %q", args.InterfaceName, m.doc.Id)

	if args.MACAddress == "" {
		return nil, fmt.Errorf("MAC address must be not empty")
	}
	if _, err = net.ParseMAC(args.MACAddress); err != nil {
		return nil, err
	}
	if args.InterfaceName == "" {
		return nil, fmt.Errorf("interface name must be not empty")
	}
	doc := newNetworkInterfaceDoc(m.doc.Id, m.st.EnvironUUID(), args)
	ops := []txn.Op{{
		C:      networksC,
		Id:     m.st.docID(args.NetworkName),
		Assert: txn.DocExists,
	}, {
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: isAliveDoc,
	}, {
		C:      networkInterfacesC,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: doc,
	}}

	err = m.st.runTransaction(ops)
	switch err {
	case txn.ErrAborted:
		if _, err = m.st.Network(args.NetworkName); err != nil {
			return nil, err
		}
		if err = m.Refresh(); err != nil {
			return nil, err
		} else if m.doc.Life != Alive {
			return nil, fmt.Errorf("machine is not alive")
		}
		// Should never happen.
		logger.Errorf("unhandled assert while adding network interface doc %#v", doc)
	case nil:
		// We have a unique key restrictions on the following fields:
		// - InterfaceName, MachineId
		// - MACAddress, NetworkName
		// These will cause the insert to fail if there is another record
		// with the same combination of values in the table.
		// The txn logic does not report insertion errors, so we check
		// that the record has actually been inserted correctly before
		// reporting success.
		networkInterfaces, closer := m.st.getCollection(networkInterfacesC)
		defer closer()

		if err = networkInterfaces.FindId(doc.Id).One(&doc); err == nil {
			return newNetworkInterface(m.st, doc), nil
		}
		sel := bson.D{{"interfacename", args.InterfaceName}, {"machineid", m.doc.Id}}
		if err = networkInterfaces.Find(sel).One(nil); err == nil {
			return nil, errors.AlreadyExistsf("%q on machine %q", args.InterfaceName, m.doc.Id)
		}
		sel = bson.D{{"macaddress", args.MACAddress}, {"networkname", args.NetworkName}}
		if err = networkInterfaces.Find(sel).One(nil); err == nil {
			return nil, errors.AlreadyExistsf("MAC address %q on network %q", args.MACAddress, args.NetworkName)
		}
		// Should never happen.
		logger.Errorf("unknown error while adding network interface doc %#v", doc)
	}
	return nil, err
}

// CheckProvisioned returns true if the machine was provisioned with the given nonce.
func (m *Machine) CheckProvisioned(nonce string) bool {
	return nonce == m.doc.Nonce && nonce != ""
}

// String returns a unique description of this machine.
func (m *Machine) String() string {
	return m.doc.Id
}

// Placement returns the machine's Placement structure that should be used when
// provisioning an instance for the machine.
func (m *Machine) Placement() string {
	return m.doc.Placement
}

// Constraints returns the exact constraints that should apply when provisioning
// an instance for the machine.
func (m *Machine) Constraints() (constraints.Value, error) {
	return readConstraints(m.st, m.globalKey())
}

// SetConstraints sets the exact constraints to apply when provisioning an
// instance for the machine. It will fail if the machine is Dead, or if it
// is already provisioned.
func (m *Machine) SetConstraints(cons constraints.Value) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set constraints")
	unsupported, err := m.st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting constraints on machine %q: unsupported constraints: %v", m.Id(), strings.Join(unsupported, ","))
	} else if err != nil {
		return err
	}
	notSetYet := bson.D{{"nonce", ""}}
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: append(isAliveDoc, notSetYet...),
		},
		setConstraintsOp(m.st, m.globalKey(), cons),
	}
	// make multiple attempts to push the ErrExcessiveContention case out of the
	// realm of plausibility: it implies local state indicating unprovisioned,
	// and remote state indicating provisioned (reasonable); but which changes
	// back to unprovisioned and then to provisioned again with *very* specific
	// timing in the course of this loop.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if m, err = m.st.Machine(m.doc.Id); err != nil {
				return nil, err
			}
		}
		if m.doc.Life != Alive {
			return nil, errNotAlive
		}
		if _, err := m.InstanceId(); err == nil {
			return nil, fmt.Errorf("machine is already provisioned")
		} else if !IsNotProvisionedError(err) {
			return nil, err
		}
		return ops, nil
	}
	return m.st.run(buildTxn)
}

// Status returns the status of the machine.
func (m *Machine) Status() (status Status, info string, data map[string]interface{}, err error) {
	doc, err := getStatus(m.st, m.globalKey())
	if err != nil {
		return "", "", nil, err
	}
	status = doc.Status
	info = doc.StatusInfo
	data = doc.StatusData
	return
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status Status, info string, data map[string]interface{}) error {
	doc := statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}
	// If a machine is not yet provisioned, we allow its status
	// to be set back to pending (when a retry is to occur).
	_, err := m.InstanceId()
	allowPending := IsNotProvisionedError(err)
	if err := doc.validateSet(allowPending); err != nil {
		return err
	}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
	},
		updateStatusOp(m.st, m.globalKey(), doc),
	}
	if err := m.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set status of machine %q: %v", m, onAbort(err, errNotAlive))
	}
	return nil
}

// Clean returns true if the machine does not have any deployed units or containers.
func (m *Machine) Clean() bool {
	return m.doc.Clean
}

// SupportedContainers returns any containers this machine is capable of hosting, and a bool
// indicating if the supported containers have been determined or not.
func (m *Machine) SupportedContainers() ([]instance.ContainerType, bool) {
	return m.doc.SupportedContainers, m.doc.SupportedContainersKnown
}

// SupportsNoContainers records the fact that this machine doesn't support any containers.
func (m *Machine) SupportsNoContainers() (err error) {
	if err = m.updateSupportedContainers([]instance.ContainerType{}); err != nil {
		return err
	}
	return m.markInvalidContainers()
}

// SetSupportedContainers sets the list of containers supported by this machine.
func (m *Machine) SetSupportedContainers(containers []instance.ContainerType) (err error) {
	if len(containers) == 0 {
		return fmt.Errorf("at least one valid container type is required")
	}
	for _, container := range containers {
		if container == instance.NONE {
			return fmt.Errorf("%q is not a valid container type", container)
		}
	}
	if err = m.updateSupportedContainers(containers); err != nil {
		return err
	}
	return m.markInvalidContainers()
}

func isSupportedContainer(container instance.ContainerType, supportedContainers []instance.ContainerType) bool {
	for _, supportedContainer := range supportedContainers {
		if supportedContainer == container {
			return true
		}
	}
	return false
}

// updateSupportedContainers sets the supported containers on this host machine.
func (m *Machine) updateSupportedContainers(supportedContainers []instance.ContainerType) (err error) {
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: notDeadDoc,
			Update: bson.D{
				{"$set", bson.D{
					{"supportedcontainers", supportedContainers},
					{"supportedcontainersknown", true},
				}}},
		},
	}
	if err = m.st.runTransaction(ops); err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot update supported containers of machine %v: %v", m, err)
		return err
	}
	m.doc.SupportedContainers = supportedContainers
	m.doc.SupportedContainersKnown = true
	return nil
}

// markInvalidContainers sets the status of any container belonging to this machine
// as being in error if the container type is not supported.
func (m *Machine) markInvalidContainers() error {
	currentContainers, err := m.Containers()
	if err != nil {
		return err
	}
	for _, containerId := range currentContainers {
		if !isSupportedContainer(ContainerTypeFromId(containerId), m.doc.SupportedContainers) {
			container, err := m.st.Machine(containerId)
			if err != nil {
				logger.Errorf("loading container %v to mark as invalid: %v", containerId, err)
				continue
			}
			// There should never be a circumstance where an unsupported container is started.
			// Nonetheless, we check and log an error if such a situation arises.
			status, _, _, err := container.Status()
			if err != nil {
				logger.Errorf("finding status of container %v to mark as invalid: %v", containerId, err)
				continue
			}
			if status == StatusPending {
				containerType := ContainerTypeFromId(containerId)
				container.SetStatus(
					StatusError, "unsupported container", map[string]interface{}{"type": containerType})
			} else {
				logger.Errorf("unsupported container %v has unexpected status %v", containerId, status)
			}
		}
	}
	return nil
}

// SetMachineBlockDevices sets the block devices visible on the machine.
func (m *Machine) SetMachineBlockDevices(devices []storage.BlockDevice) error {
	return setMachineBlockDevices(m.st, m.Id(), devices)
}

// BlockDevices gets the aggregated list of block devices attached to the
// machine.
func (m *Machine) BlockDevices() ([]storage.BlockDevice, error) {
	return getBlockDevices(m.st, m.Id())
}
