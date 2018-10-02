// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"github.com/kr/pretty"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/tools"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	doc machineDoc
}

// MachineJob values define responsibilities that machines may be
// expected to fulfil.
type MachineJob int

const (
	_ MachineJob = iota
	JobHostUnits
	JobManageModel
)

var (
	jobNames = map[MachineJob]multiwatcher.MachineJob{
		JobHostUnits:   multiwatcher.JobHostUnits,
		JobManageModel: multiwatcher.JobManageModel,
	}
	jobMigrationValue = map[MachineJob]string{
		JobHostUnits:   "host-units",
		JobManageModel: "api-server",
	}
)

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

// MigrationValue converts the state job into a useful human readable
// string for model migration.
func (job MachineJob) MigrationValue() string {
	if value, ok := jobMigrationValue[job]; ok {
		return value
	}
	return "unknown"
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
	ModelUUID     string `bson:"model-uuid"`
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

	// Volumes contains the names of volumes attached to the machine.
	Volumes []string `bson:"volumes,omitempty"`
	// Filesystems contains the names of filesystems attached to the machine.
	Filesystems []string `bson:"filesystems,omitempty"`

	// We store 2 different sets of addresses for the machine, obtained
	// from different sources.
	// Addresses is the set of addresses obtained by asking the provider.
	Addresses []address

	// MachineAddresses is the set of addresses obtained from the machine itself.
	MachineAddresses []address

	// PreferredPublicAddress is the preferred address to be used for
	// the machine when a public address is requested.
	PreferredPublicAddress address `bson:",omitempty"`

	// PreferredPrivateAddress is the preferred address to be used for
	// the machine when a private address is requested.
	PreferredPrivateAddress address `bson:",omitempty"`

	// The SupportedContainers attributes are used to advertise what containers this
	// machine is capable of hosting.
	SupportedContainersKnown bool
	SupportedContainers      []instance.ContainerType `bson:",omitempty"`
	// Placement is the placement directive that should be used when provisioning
	// an instance for the machine.
	Placement string `bson:",omitempty"`

	// StopMongoUntilVersion holds the version that must be checked to
	// know if mongo must be stopped.
	StopMongoUntilVersion string `bson:",omitempty"`

	// UpgradeCharmProfileApplication holds the name of the application where there
	// is an charm upgrade event and a charm profile.
	UpgradeCharmProfileApplication string `bson:",omitempty"`

	// UpgradeCharmProfileCharmURL holds the charm URL when there is an charm
	// upgrade event with a charm profile.  This is used before the application
	// contains the new charm URL during a charm upgrade.
	UpgradeCharmProfileCharmURL string `bson:",omitempty"`
}

func newMachine(st *State, doc *machineDoc) *Machine {
	machine := &Machine{
		st:  st,
		doc: *doc,
	}
	return machine
}

func wantsVote(jobs []MachineJob, noVote bool) bool {
	return hasJob(jobs, JobManageModel) && !noVote
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// Principals returns the principals for the machine.
func (m *Machine) Principals() []string {
	return m.doc.Principals
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

// machineGlobalInstanceKey returns the global database key for the identified machine's instance.
func machineGlobalInstanceKey(id string) string {
	return machineGlobalKey(id) + "#instance"
}

// globalInstanceKey returns the global database key for the machinei's instance.
func (m *Machine) globalInstanceKey() string {
	return machineGlobalInstanceKey(m.doc.Id)
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
	ModelUUID  string      `bson:"model-uuid"`
	Arch       *string     `bson:"arch,omitempty"`
	Mem        *uint64     `bson:"mem,omitempty"`
	RootDisk   *uint64     `bson:"rootdisk,omitempty"`
	CpuCores   *uint64     `bson:"cpucores,omitempty"`
	CpuPower   *uint64     `bson:"cpupower,omitempty"`
	Tags       *[]string   `bson:"tags,omitempty"`
	AvailZone  *string     `bson:"availzone,omitempty"`

	// KeepInstance is set to true if, on machine removal from Juju,
	// the cloud instance should be retained.
	KeepInstance bool `bson:"keep-instance,omitempty"`

	// CharmProfiles contains the names of LXD profiles used by this machine.
	// Profiles would have been defined in the charm deployed to this machine.
	CharmProfiles []string `bson:"charm-profiles,omitempty"`
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
	instanceDataCollection, closer := st.db().GetCollection(instanceDataC)
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
	return m.MachineTag()
}

// MachineTag returns the more specific MachineTag type as opposed
// to the more generic Tag type.
func (m *Machine) MachineTag() names.MachineTag {
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

// SetKeepInstance sets whether the cloud machine instance
// will be retained when the machine is removed from Juju.
// This is only relevant if an instance exists.
func (m *Machine) SetKeepInstance(keepInstance bool) error {
	ops := []txn.Op{{
		C:      instanceDataC,
		Id:     m.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"keep-instance", keepInstance}}}},
	}}
	if err := m.st.db().RunTransaction(ops); err != nil {
		// If instance doc doesn't exist, that's ok; there's nothing to keep,
		// but that's not an error we care about.
		return errors.Annotatef(onAbort(err, nil), "cannot set KeepInstance on machine %v", m)
	}
	return nil
}

// KeepInstance reports whether a machine, when removed from
// Juju, will cause the corresponding cloud instance to be stopped.
func (m *Machine) KeepInstance() (bool, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if err != nil {
		return false, err
	}
	return instData.KeepInstance, nil
}

// CharmProfiles returns the names of any LXD profiles used by the machine,
// which were defined in the charm deployed to that machine.
func (m *Machine) CharmProfiles() ([]string, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		err = errors.NotProvisionedf("machine %v", m.Id())
	}
	if err != nil {
		return nil, err
	}
	return instData.CharmProfiles, nil
}

// SetCharmProfiles sets the names of the charm profiles used on a machine
// in its instanceData.
func (m *Machine) SetCharmProfiles(profiles []string) error {
	if len(profiles) == 0 {
		return nil
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		// Exit early if the Machine profiles doesn't need to change.
		mProfiles, err := m.CharmProfiles()
		if err != nil {
			return nil, errors.Trace(err)
		}
		mProfilesSet := set.NewStrings(mProfiles...)
		if mProfilesSet.Union(set.NewStrings(profiles...)).Size() == mProfilesSet.Size() {
			return nil, jujutxn.ErrNoOperations
		}

		ops := []txn.Op{{
			C:      instanceDataC,
			Id:     m.doc.DocID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"charm-profiles", profiles}}}},
		}}

		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	return errors.Annotatef(err, "cannot update profiles for %q to %s", m, strings.Join(profiles, ", "))
}

// WantsVote reports whether the machine is a controller
// that wants to take part in peer voting.
func (m *Machine) WantsVote() bool {
	return wantsVote(m.doc.Jobs, m.doc.NoVote)
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
	op := m.UpdateOperation()
	op.HasVote = &hasVote
	if err := m.st.ApplyOperation(op); err != nil {
		return errors.Trace(err)
	}
	m.doc.HasVote = hasVote
	return nil
}

func (m *Machine) setHasVoteOps(hasVote bool) ([]txn.Op, error) {
	if m.Life() == Dead {
		return nil, ErrDead
	}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"hasvote", hasVote}}}},
	}}
	return ops, nil
}

// SetStopMongoUntilVersion sets a version that is to be checked against
// the agent config before deciding if mongo must be started on a
// state server.
func (m *Machine) SetStopMongoUntilVersion(v mongo.Version) error {
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"stopmongountilversion", v.String()}}}},
	}}
	if err := m.st.db().RunTransaction(ops); err != nil {
		return fmt.Errorf("cannot set StopMongoUntilVersion %v: %v", m, onAbort(err, ErrDead))
	}
	m.doc.StopMongoUntilVersion = v.String()
	return nil
}

// StopMongoUntilVersion returns the current minimum version that
// is required for this machine to have mongo running.
func (m *Machine) StopMongoUntilVersion() (mongo.Version, error) {
	return mongo.NewVersion(m.doc.StopMongoUntilVersion)
}

// IsManager returns true if the machine has JobManageModel.
func (m *Machine) IsManager() bool {
	return hasJob(m.doc.Jobs, JobManageModel)
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
		model, err := m.st.Model()
		if err != nil {
			return false, errors.Trace(err)
		}

		cfg, err := model.ModelConfig()
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
		return nil, errors.NotFoundf("agent binaries for machine %v", m)
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
	ops, tools, err := m.setAgentVersionOps(v)
	if err != nil {
		return errors.Trace(err)
	}
	// A "raw" transaction is needed here because this function gets
	// called before database migrations have run so we don't
	// necessarily want the model UUID added to the id.
	if err := m.st.runRawTransaction(ops); err != nil {
		return onAbort(err, ErrDead)
	}
	m.doc.Tools = tools
	return nil
}

func (m *Machine) setAgentVersionOps(v version.Binary) ([]txn.Op, *tools.Tools, error) {
	if err := checkVersionValidity(v); err != nil {
		return nil, nil, err
	}
	tools := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"tools", tools}}}},
	}}
	return ops, tools, nil
}

// SetMongoPassword sets the password the agent responsible for the machine
// should use to communicate with the controllers.  Previous passwords
// are invalidated.
func (m *Machine) SetMongoPassword(password string) error {
	if !m.IsManager() {
		return errors.NotSupportedf("setting mongo password for non-controller machine %v", m)
	}
	return mongo.SetAdminMongoPassword(m.st.session, m.Tag().String(), password)
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	if len(password) < utils.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := utils.AgentPasswordHash(password)
	op := m.UpdateOperation()
	op.PasswordHash = &passwordHash
	if err := m.st.ApplyOperation(op); err != nil {
		return errors.Trace(err)
	}
	m.doc.PasswordHash = passwordHash
	return nil
}

func (m *Machine) setPasswordHashOps(passwordHash string) ([]txn.Op, error) {
	if m.doc.Life == Dead {
		return nil, ErrDead
	}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	return ops, nil
}

// PasswordValid returns whether the given password is valid
// for the given machine.
func (m *Machine) PasswordValid(password string) bool {
	agentHash := utils.AgentPasswordHash(password)
	return agentHash == m.doc.PasswordHash
}

// Destroy sets the machine lifecycle to Dying if it is Alive. It does
// nothing otherwise. Destroy will fail if the machine has principal
// units assigned, or if the machine has JobManageModel.
// If the machine has assigned units, Destroy will return
// a HasAssignedUnitsError.
func (m *Machine) Destroy() error {
	return m.advanceLifecycle(Dying)
}

// ForceDestroy queues the machine for complete removal, including the
// destruction of all units and containers on the machine.
func (m *Machine) ForceDestroy() error {
	ops, err := m.forceDestroyOps()
	if err != nil {
		return errors.Trace(err)
	}
	if err := m.st.db().RunTransaction(ops); err != txn.ErrAborted {
		return errors.Annotatef(err, "failed to run transaction: %s", pretty.Sprint(ops))
	}
	return nil
}

func (m *Machine) forceDestroyOps() ([]txn.Op, error) {
	if m.IsManager() {
		controllerInfo, err := m.st.ControllerInfo()
		if err != nil {
			return nil, errors.Annotatef(err, "reading controller info")
		}
		if len(controllerInfo.MachineIds) <= 1 {
			return nil, errors.Errorf("machine %s is the only controller machine", m.Id())
		}
		// We set the machine to Dying if it isn't already dead.
		var machineOp txn.Op
		if m.Life() < Dead {
			// Make sure we don't want the vote, and we are queued to be Dying
			machineOp = txn.Op{
				C:      machinesC,
				Id:     m.doc.DocID,
				Assert: bson.D{{"life", bson.D{{"$in", []Life{Alive, Dying}}}}},
				Update: bson.D{{"$set", bson.D{{"novote", true}, {"life", Dying}}}},
			}
		} else {
			machineOp = txn.Op{
				C:      machinesC,
				Id:     m.doc.DocID,
				Update: bson.D{{"$set", bson.D{{"novote", true}}}},
			}
		}
		controllerOp := txn.Op{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: bson.D{{"machineids", controllerInfo.MachineIds}},
		}
		// Note that ForceDestroy does *not* cleanup the replicaset, so it might cause problems.
		// However, we're letting the user handle times when the machine agent isn't running, etc.
		// We may need to update the peergrouper for this.
		return []txn.Op{
			machineOp,
			controllerOp,
			newCleanupOp(cleanupForceDestroyedMachine, m.doc.Id),
		}, nil
	} else {
		// Make sure the machine doesn't become a manager while we're destroying it
		return []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}}},
		}, newCleanupOp(cleanupForceDestroyedMachine, m.doc.Id),
		}, nil
	}
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageModel.
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
	containerRefs, closer := m.st.db().GetCollection(containerRefsC)
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

// IsContainer returns true if the machine is a container.
func (m *Machine) IsContainer() bool {
	_, isContainer := m.ParentId()
	return isContainer
}

type HasContainersError struct {
	MachineId    string
	ContainerIds []string
}

func (e *HasContainersError) Error() string {
	return fmt.Sprintf("machine %s is hosting containers %q", e.MachineId, strings.Join(e.ContainerIds, ","))
}

// IsHasContainersError reports whether or not the error is a
// HasContainersError, indicating that an attempt to destroy
// a machine failed due to it having containers.
func IsHasContainersError(err error) bool {
	_, ok := errors.Cause(err).(*HasContainersError)
	return ok
}

// HasAttachmentsError is the error returned by EnsureDead if the machine
// has attachments to resources that must be cleaned up first.
type HasAttachmentsError struct {
	MachineId   string
	Attachments []names.Tag
}

func (e *HasAttachmentsError) Error() string {
	return fmt.Sprintf(
		"machine %s has attachments %s",
		e.MachineId, e.Attachments,
	)
}

// IsHasAttachmentsError reports whether or not the error is a
// HasAttachmentsError, indicating that an attempt to destroy
// a machine failed due to it having storage attachments.
func IsHasAttachmentsError(err error) bool {
	_, ok := errors.Cause(err).(*HasAttachmentsError)
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
	// noUnits asserts that the machine has no principal units.
	noUnits := bson.DocElem{
		"$or", []bson.D{
			{{"principals", bson.D{{"$size", 0}}}},
			{{"principals", bson.D{{"$exists", false}}}},
		},
	}
	cleanupOp := newCleanupOp(cleanupDyingMachine, m.doc.Id)
	// multiple attempts: one with original data, one with refreshed data, and a final
	// one intended to determine the cause of failure of the preceding attempt.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		ops := []txn.Op{op}
		var asserts bson.D
		// Grab a fresh copy of the machine data.
		// We don't write to original, because the expectation is that state-
		// changing methods only set the requested change on the receiver; a case
		// could perhaps be made that this is not a helpful convention in the
		// context of the new state API, but we maintain consistency in the
		// face of uncertainty.
		if m, err = m.st.Machine(m.doc.Id); errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
		// Check that the life change is sane, and collect the assertions
		// necessary to determine that it remains so.
		switch life {
		case Dying:
			if m.doc.Life != Alive {
				return nil, jujutxn.ErrNoOperations
			}
			// Manager nodes are allowed to go to dying even when they have the vote, as that is used as the signal
			// that they should lose their vote
			asserts = append(asserts, isAliveDoc...)
		case Dead:
			if m.doc.Life == Dead {
				return nil, jujutxn.ErrNoOperations
			}
			if m.doc.HasVote {
				return nil, fmt.Errorf("machine %s is still a voting controller member", m.doc.Id)
			}
			if m.IsManager() {
				return nil, errors.Errorf("machine %s is still a controller member", m.Id())
			}
			asserts = append(asserts, bson.D{
				{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}},
				{"hasvote", bson.M{"$ne": true}},
			}...)
			asserts = append(asserts, notDeadDoc...)
		default:
			panic(fmt.Errorf("cannot advance lifecycle to %v", life))
		}
		// Check that the machine does not have any responsibilities that
		// prevent a lifecycle change.
		// If there are no alive units left on the machine, or all the applications are dying,
		// then the machine may be soon destroyed by a cleanup worker.
		// In that case, we don't want to return any error about not being able to
		// destroy a machine with units as it will be a lie.
		if life == Dying {
			canDie := true
			if hasJob(m.doc.Jobs, JobManageModel) || m.doc.HasVote {
				// If we're responsible for managing the model, make sure we ask to drop our vote
				ops[0].Update = bson.D{
					{"$set", bson.D{{"life", life}, {"novote", true}}},
				}
				controllerInfo, err := m.st.ControllerInfo()
				if err != nil {
					return nil, errors.Annotatef(err, "reading controller info")
				}
				if len(controllerInfo.MachineIds) <= 1 {
					return nil, errors.Errorf("machine %s is the only controller machine", m.Id())
				}
				controllerOp := txn.Op{
					C:      controllersC,
					Id:     modelGlobalKey,
					Assert: bson.D{{"machineids", controllerInfo.MachineIds}},
				}
				ops = append(ops, controllerOp)
			}
			var principalUnitnames []string
			for _, principalUnit := range m.doc.Principals {
				principalUnitnames = append(principalUnitnames, principalUnit)
				u, err := m.st.Unit(principalUnit)
				if err != nil {
					return nil, errors.Annotatef(err, "reading machine %s principal unit %v", m, m.doc.Principals[0])
				}
				app, err := u.Application()
				if err != nil {
					return nil, errors.Annotatef(err, "reading machine %s principal unit application %v", m, u.doc.Application)
				}
				if u.Life() == Alive && app.Life() == Alive {
					canDie = false
					break
				}
			}
			if canDie {
				containers, err := m.Containers()
				if err != nil {
					return nil, errors.Annotatef(err, "reading machine %s containers", m)
				}
				canDie = len(containers) == 0
				containerCheck := txn.Op{
					C:  containerRefsC,
					Id: m.doc.DocID,
					Assert: bson.D{{"$or", []bson.D{
						{{"children", bson.D{{"$size", 0}}}},
						{{"children", bson.D{{"$exists", false}}}},
					}}},
				}
				ops = append(ops, containerCheck)
			}
			if canDie {
				checkUnits := bson.DocElem{
					"$or", []bson.D{
						{{"principals", principalUnitnames}},
						{{"principals", bson.D{{"$size", 0}}}},
						{{"principals", bson.D{{"$exists", false}}}},
					},
				}
				ops[0].Assert = append(asserts, checkUnits)
				ops = append(ops, cleanupOp)
				txnLogger.Debugf("txn moving machine %q to %s", m.Id(), life)
				return ops, nil
			}
		}

		if len(m.doc.Principals) > 0 {
			return nil, &HasAssignedUnitsError{
				MachineId: m.doc.Id,
				UnitNames: m.doc.Principals,
			}
		}
		asserts = append(asserts, noUnits)

		if life == Dead {
			if hasJob(m.doc.Jobs, JobManageModel) {
				return nil, errors.Errorf("machine %s is still resposible for being a controller", m.Id())
			}
			// A machine may not become Dead until it has no more
			// attachments to detachable storage.
			storageAsserts, err := m.assertNoPersistentStorage()
			if err != nil {
				return nil, errors.Trace(err)
			}
			asserts = append(asserts, storageAsserts...)
		}

		// Add the additional asserts needed for this transaction.
		ops[0].Assert = asserts
		ops = append(ops, cleanupOp)
		return ops, nil
	}
	if err = m.st.db().Run(buildTxn); err == jujutxn.ErrExcessiveContention {
		err = errors.Annotatef(err, "machine %s cannot advance lifecycle", m)
	}
	return err
}

// assertNoPersistentStorage ensures that there are no persistent volumes or
// filesystems attached to the machine, and returns any mgo/txn assertions
// required to ensure that remains true.
func (m *Machine) assertNoPersistentStorage() (bson.D, error) {
	attachments := names.NewSet()
	for _, v := range m.doc.Volumes {
		tag := names.NewVolumeTag(v)
		detachable, err := isDetachableVolumeTag(m.st.db(), tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if detachable {
			attachments.Add(tag)
		}
	}
	for _, f := range m.doc.Filesystems {
		tag := names.NewFilesystemTag(f)
		detachable, err := isDetachableFilesystemTag(m.st.db(), tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if detachable {
			attachments.Add(tag)
		}
	}
	if len(attachments) > 0 {
		return nil, &HasAttachmentsError{
			MachineId:   m.doc.Id,
			Attachments: attachments.SortedValues(),
		}
	}
	if m.doc.Life == Dying {
		return nil, nil
	}
	// A Dying machine cannot have attachments added to it,
	// but if we're advancing from Alive to Dead then we
	// must ensure no concurrent attachments are made.
	noNewVolumes := bson.DocElem{
		"volumes", bson.D{{
			"$not", bson.D{{
				"$elemMatch", bson.D{{
					"$nin", m.doc.Volumes,
				}},
			}},
		}},
		// There are no volumes that are not in
		// the set of volumes we previously knew
		// about => the current set of volumes
		// is a subset of the previously known set.
	}
	noNewFilesystems := bson.DocElem{
		"filesystems", bson.D{{
			"$not", bson.D{{
				"$elemMatch", bson.D{{
					"$nin", m.doc.Filesystems,
				}},
			}},
		}},
	}
	return bson.D{noNewVolumes, noNewFilesystems}, nil
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

func (m *Machine) removeOps() ([]txn.Op, error) {
	if m.doc.Life != Dead {
		return nil, fmt.Errorf("machine is not dead")
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
		removeStatusOp(m.st, m.globalKey()),
		removeStatusOp(m.st, m.globalInstanceKey()),
		removeConstraintsOp(m.globalKey()),
		annotationRemoveOp(m.st, m.globalKey()),
		removeRebootDocOp(m.st, m.globalKey()),
		removeMachineBlockDevicesOp(m.Id()),
		removeModelMachineRefOp(m.st, m.Id()),
		removeSSHHostKeyOp(m.globalKey()),
	}
	linkLayerDevicesOps, err := m.removeAllLinkLayerDevicesOps()
	if err != nil {
		return nil, errors.Trace(err)
	}
	devicesAddressesOps, err := m.removeAllAddressesOps()
	if err != nil {
		return nil, errors.Trace(err)
	}
	portsOps, err := m.removePortsOps()
	if err != nil {
		return nil, errors.Trace(err)
	}

	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystemOps, err := sb.removeMachineFilesystemsOps(m)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeOps, err := sb.removeMachineVolumesOps(m)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops = append(ops, linkLayerDevicesOps...)
	ops = append(ops, devicesAddressesOps...)
	ops = append(ops, portsOps...)
	ops = append(ops, removeContainerRefOps(m.st, m.Id())...)
	ops = append(ops, filesystemOps...)
	ops = append(ops, volumeOps...)
	return ops, nil
}

// Remove removes the machine from state. It will fail if the machine
// is not Dead.
func (m *Machine) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove machine %s", m.doc.Id)
	logger.Tracef("removing machine %q", m.Id())
	// Local variable so we can re-get the machine without disrupting
	// the caller.
	machine := m
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			machine, err = machine.st.Machine(machine.Id())
			if errors.IsNotFound(err) {
				// The machine's gone away, that's fine.
				return nil, jujutxn.ErrNoOperations
			}
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops, err := machine.removeOps()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return m.st.db().Run(buildTxn)
}

// Refresh refreshes the contents of the machine from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// machine has been removed.
func (m *Machine) Refresh() error {
	mdoc, err := m.st.getMachineDoc(m.Id())
	if err != nil {
		if errors.IsNotFound(err) {
			return err
		}
		return errors.Annotatef(err, "cannot refresh machine %v", m)
	}
	m.doc = *mdoc
	return nil
}

// AgentPresence returns whether the respective remote agent is alive.
func (m *Machine) AgentPresence() (bool, error) {
	pwatcher := m.st.workers.presenceWatcher()
	return pwatcher.Alive(m.globalKey())
}

// WaitAgentPresence blocks until the respective agent is alive.
// This should really only be used in the test suite.
func (m *Machine) WaitAgentPresence(timeout time.Duration) (err error) {
	defer errors.DeferredAnnotatef(&err, "waiting for agent of machine %v", m)
	ch := make(chan presence.Change)
	pwatcher := m.st.workers.presenceWatcher()
	pwatcher.Watch(m.globalKey(), ch)
	defer pwatcher.Unwatch(m.globalKey(), ch)
	pingBatcher := m.st.getPingBatcher()
	if err := pingBatcher.Sync(); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		select {
		case change := <-ch:
			if change.Alive {
				return nil
			}
		case <-time.After(timeout):
			// TODO(fwereade): 2016-03-17 lp:1558657
			return fmt.Errorf("still not alive after timeout")
		case <-pwatcher.Dead():
			return pwatcher.Err()
		}
	}
	panic(fmt.Sprintf("presence reported dead status twice in a row for machine %v", m))
}

// SetAgentPresence signals that the agent for machine m is alive.
// It returns the started pinger.
func (m *Machine) SetAgentPresence() (*presence.Pinger, error) {
	presenceCollection := m.st.getPresenceCollection()
	recorder := m.st.getPingBatcher()
	p := presence.NewPinger(presenceCollection, m.st.modelTag, m.globalKey(),
		func() presence.PingRecorder { return m.st.getPingBatcher() })
	err := p.Start()
	if err != nil {
		return nil, err
	}
	// Make sure this Agent status is written to the database before returning.
	recorder.Sync()
	// We preform a manual sync here so that the
	// presence pinger has the most up-to-date information when it
	// starts. This ensures that commands run immediately after bootstrap
	// like status or enable-ha will have an accurate values
	// for agent-state.
	//
	// TODO: Does not work for multiple controllers. Trigger a sync across all controllers.
	if m.IsManager() {
		m.st.workers.presenceWatcher().Sync()
	}
	return p, nil
}

// InstanceId returns the provider specific instance id for this
// machine, or a NotProvisionedError, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		err = errors.NotProvisionedf("machine %v", m.Id())
	}
	if err != nil {
		return "", err
	}
	return instData.InstanceId, err
}

// InstanceStatus returns the provider specific instance status for this machine,
// or a NotProvisionedError if instance is not yet provisioned.
func (m *Machine) InstanceStatus() (status.StatusInfo, error) {
	machineStatus, err := getStatus(m.st.db(), m.globalInstanceKey(), "instance")
	if err != nil {
		logger.Warningf("error when retrieving instance status for machine: %s, %v", m.Id(), err)
		return status.StatusInfo{}, err
	}
	return machineStatus, nil
}

// SetInstanceStatus sets the provider specific instance status for a machine.
func (m *Machine) SetInstanceStatus(sInfo status.StatusInfo) (err error) {
	return setStatus(m.st.db(), setStatusParams{
		badge:     "instance",
		globalKey: m.globalInstanceKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   timeOrNow(sInfo.Since, m.st.clock()),
	})

}

// InstanceStatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this machine instance.
// Instance represents the provider underlying [v]hardware or container where
// this juju machine is deployed.
func (m *Machine) InstanceStatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        m.st.db(),
		globalKey: m.globalInstanceKey(),
		filter:    filter,
	}
	return statusHistory(args)
}

// AvailabilityZone returns the provier-specific instance availability
// zone in which the machine was provisioned.
func (m *Machine) AvailabilityZone() (string, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		return "", errors.Trace(errors.NotProvisionedf("machine %v", m.Id()))
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
	unitsCollection, closer := m.st.db().GetCollection(unitsC)
	defer closer()

	pudocs := []unitDoc{}
	err = unitsCollection.Find(bson.D{{"machineid", m.doc.Id}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	model, err := m.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, pudoc := range pudocs {
		units = append(units, newUnit(m.st, model.Type(), &pudoc))
		docs := []unitDoc{}
		err = unitsCollection.Find(bson.D{{"principal", pudoc.Name}}).All(&docs)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			units = append(units, newUnit(m.st, model.Type(), &doc))
		}
	}
	return units, nil
}

// XXX(jam): 2016-12-09 These are just copied from
// provider/maas/constraints.go, but they should be tied to machine
// constraints, *not* tied to provider/maas constraints.
// convertSpacesFromConstraints extracts spaces from constraints and converts
// them to two lists of positive and negative spaces.
func convertSpacesFromConstraints(spaces *[]string) ([]string, []string) {
	if spaces == nil || len(*spaces) == 0 {
		return nil, nil
	}
	positive, negative := parseDelimitedValues(*spaces)
	return positive, negative
}

// parseDelimitedValues parses a slice of raw values coming from constraints
// (Tags or Spaces). The result is split into two slices - positives and
// negatives (prefixed with "^"). Empty values are ignored.
func parseDelimitedValues(rawValues []string) (positives, negatives []string) {
	for _, value := range rawValues {
		if value == "" || value == "^" {
			// Neither of these cases should happen in practise, as constraints
			// are validated before setting them and empty names for spaces or
			// tags are not allowed.
			continue
		}
		if strings.HasPrefix(value, "^") {
			negatives = append(negatives, strings.TrimPrefix(value, "^"))
		} else {
			positives = append(positives, value)
		}
	}
	return positives, negatives
}

// DesiredSpaces returns the name of all spaces that this machine needs
// access to.  This is the combined value of all of the direct constraints
// for the machine, as well as the spaces listed for all bindings of units
// being deployed to that machine.
func (m *Machine) DesiredSpaces() (set.Strings, error) {
	spaces := set.NewStrings()
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	constraints, err := m.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We ignore negative spaces as it doesn't change what spaces we do want.
	positiveSpaces, _ := convertSpacesFromConstraints(constraints.Spaces)
	for _, space := range positiveSpaces {
		spaces.Add(space)
	}
	bindings := set.NewStrings()
	for _, unit := range units {
		app, err := unit.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindings, err := app.EndpointBindings()
		for _, space := range endpointBindings {
			if space != "" {
				bindings.Add(space)
			}
		}
	}
	logger.Tracef("machine %q found constraints %s and bindings %s",
		m.Id(), network.QuoteSpaceSet(spaces), network.QuoteSpaceSet(bindings))
	return spaces.Union(bindings), nil
}

// SetProvisioned sets the provider specific machine id, nonce and also metadata for
// this machine. Once set, the instance id cannot be changed.
//
// When provisioning an instance, a nonce should be created and passed
// when starting it, before adding the machine to the state. This means
// that if the provisioner crashes (or its connection to the state is
// lost) after starting the instance, we can be sure that only a single
// instance will be able to act for that machine.
func (m *Machine) SetProvisioned(
	id instance.Id,
	nonce string,
	characteristics *instance.HardwareCharacteristics,
) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set instance data for machine %q", m)

	if id == "" || nonce == "" {
		return fmt.Errorf("instance id and nonce cannot be empty")
	}

	coll, closer := m.st.db().GetCollection(instanceDataC)
	defer closer()
	count, err := coll.Find(bson.D{{"instanceid", id}}).Count()
	if err != nil {
		return errors.Trace(err)
	}
	if count > 0 {
		logger.Warningf("duplicate instance id %q already saved", id)
	}

	if characteristics == nil {
		characteristics = &instance.HardwareCharacteristics{}
	}
	instData := &instanceData{
		DocID:      m.doc.DocID,
		MachineId:  m.doc.Id,
		InstanceId: id,
		ModelUUID:  m.doc.ModelUUID,
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

	if err = m.st.db().RunTransaction(ops); err == nil {
		m.doc.Nonce = nonce
		return nil
	} else if err != txn.ErrAborted {
		return err
	} else if alive, err := isAlive(m.st, machinesC, m.doc.DocID); err != nil {
		return err
	} else if !alive {
		return machineNotAliveErr
	}
	return fmt.Errorf("already set")
}

// SetInstanceInfo is used to provision a machine and in one step sets it's
// instance id, nonce, hardware characteristics, add link-layer devices and set
// their addresses as needed.  After, set charm profiles if needed.
func (m *Machine) SetInstanceInfo(
	id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics,
	devicesArgs []LinkLayerDeviceArgs, devicesAddrs []LinkLayerDeviceAddress,
	volumes map[names.VolumeTag]VolumeInfo,
	volumeAttachments map[names.VolumeTag]VolumeAttachmentInfo,
	charmProfiles []string,
) error {
	logger.Tracef(
		"setting instance info: machine %v, deviceAddrs: %#v, devicesArgs: %#v",
		m.Id(), devicesAddrs, devicesArgs)

	if err := m.SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs); err != nil {
		return errors.Trace(err)
	}
	if err := m.SetDevicesAddressesIdempotently(devicesAddrs); err != nil {
		return errors.Trace(err)
	}

	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}

	// Record volumes and volume attachments, and set the initial
	// status: attached or attaching.
	if err := setProvisionedVolumeInfo(sb, volumes); err != nil {
		return errors.Trace(err)
	}
	if err := setMachineVolumeAttachmentInfo(sb, m.Id(), volumeAttachments); err != nil {
		return errors.Trace(err)
	}
	volumeStatus := make(map[names.VolumeTag]status.Status)
	for tag := range volumes {
		volumeStatus[tag] = status.Attaching
	}
	for tag := range volumeAttachments {
		volumeStatus[tag] = status.Attached
	}
	for tag, volStatus := range volumeStatus {
		vol, err := sb.Volume(tag)
		if err != nil {
			return errors.Trace(err)
		}
		if err := vol.SetStatus(status.StatusInfo{
			Status: volStatus,
		}); err != nil {
			return errors.Annotatef(
				err, "setting status of %s", names.ReadableString(tag),
			)
		}
	}

	if err := m.SetProvisioned(id, nonce, characteristics); err != nil {
		return errors.Trace(err)
	}
	return m.SetCharmProfiles(charmProfiles)
}

// Addresses returns any hostnames and ips associated with a machine,
// determined both by the machine itself, and by asking the provider.
//
// The addresses returned by the provider shadow any of the addresses
// that the machine reported with the same address value.
// Provider-reported addresses always come before machine-reported
// addresses. Duplicates are removed.
func (m *Machine) Addresses() (addresses []network.Address) {
	return network.MergedAddresses(networkAddresses(m.doc.MachineAddresses), networkAddresses(m.doc.Addresses))
}

func containsAddress(addresses []address, address address) bool {
	for _, addr := range addresses {
		if addr.Value == address.Value {
			return true
		}
	}
	return false
}

// PublicAddress returns a public address for the machine. If no address is
// available it returns an error that satisfies network.IsNoAddressError().
func (m *Machine) PublicAddress() (network.Address, error) {
	publicAddress := m.doc.PreferredPublicAddress.networkAddress()
	var err error
	if publicAddress.Value == "" {
		err = network.NoAddressError("public")
	}
	return publicAddress, err
}

// maybeGetNewAddress determines if the current address is the most appropriate
// match, and if not it selects the best from the slice of all available
// addresses. It returns the new address and a bool indicating if a different
// one was picked.
func maybeGetNewAddress(
	addr address,
	providerAddresses,
	machineAddresses []address,
	getAddr func([]address) network.Address,
	checkScope func(address) bool,
) (address, bool) {
	// For picking the best address, try provider addresses first.
	var newAddr address
	netAddr := getAddr(providerAddresses)
	if netAddr.Value == "" {
		netAddr = getAddr(machineAddresses)
		newAddr = fromNetworkAddress(netAddr, OriginMachine)
	} else {
		newAddr = fromNetworkAddress(netAddr, OriginProvider)
	}
	// The order of these checks is important. If the stored address is
	// empty we *always* want to check for a new address so we do that
	// first. If the stored address is unavailable we also *must* check for
	// a new address so we do that next. If the original is a machine
	// address and a provider address is available we want to switch to
	// that. Finally we check to see if a better match on scope from the
	// same origin is available.
	if addr.Value == "" {
		return newAddr, newAddr.Value != ""
	}
	if !containsAddress(providerAddresses, addr) && !containsAddress(machineAddresses, addr) {
		return newAddr, true
	}
	if Origin(addr.Origin) != OriginProvider && Origin(newAddr.Origin) == OriginProvider {
		return newAddr, true
	}
	if !checkScope(addr) {
		// If addr.Origin is machine and newAddr.Origin is provider we will
		// have already caught that, and for the inverse we don't want to
		// replace the address.
		if addr.Origin == newAddr.Origin {
			return newAddr, checkScope(newAddr)
		}
	}
	return addr, false
}

// PrivateAddress returns a private address for the machine. If no address is
// available it returns an error that satisfies network.IsNoAddressError().
func (m *Machine) PrivateAddress() (network.Address, error) {
	privateAddress := m.doc.PreferredPrivateAddress.networkAddress()
	var err error
	if privateAddress.Value == "" {
		err = network.NoAddressError("private")
	}
	return privateAddress, err
}

func (m *Machine) setPreferredAddressOps(addr address, isPublic bool) []txn.Op {
	fieldName := "preferredprivateaddress"
	current := m.doc.PreferredPrivateAddress
	if isPublic {
		fieldName = "preferredpublicaddress"
		current = m.doc.PreferredPublicAddress
	}
	// Assert that the field is either missing (never been set) or is
	// unchanged from its previous value.

	// Since using a struct in the assert also asserts ordering, and we know that mgo
	// can change the ordering, we assert on the dotted values, effectively checking each
	// of the attributes of the address.
	currentD := []bson.D{
		{{fieldName + ".value", current.Value}},
		{{fieldName + ".addresstype", current.AddressType}},
	}
	// Since scope, origin, and space have omitempty, we don't add them if they are empty.
	if current.Scope != "" {
		currentD = append(currentD, bson.D{{fieldName + ".networkscope", current.Scope}})
	}
	if current.Origin != "" {
		currentD = append(currentD, bson.D{{fieldName + ".origin", current.Origin}})
	}
	if current.SpaceName != "" {
		currentD = append(currentD, bson.D{{fieldName + ".spacename", current.SpaceName}})
	}

	assert := bson.D{{"$or", []bson.D{
		{{"$and", currentD}},
		{{fieldName, nil}}}}}

	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Update: bson.D{{"$set", bson.D{{fieldName, addr}}}},
		Assert: assert,
	}}
	logger.Tracef("setting preferred address to %v (isPublic %#v)", addr, isPublic)
	return ops
}

func (m *Machine) setPublicAddressOps(providerAddresses []address, machineAddresses []address) ([]txn.Op, *address) {
	publicAddress := m.doc.PreferredPublicAddress
	logger.Tracef(
		"machine %v: current public address: %#v \nprovider addresses: %#v \nmachine addresses: %#v",
		m.Id(), publicAddress, providerAddresses, machineAddresses)

	// Always prefer an exact match if available.
	checkScope := func(addr address) bool {
		return network.ExactScopeMatch(addr.networkAddress(), network.ScopePublic)
	}
	// Without an exact match, prefer a fallback match.
	getAddr := func(addresses []address) network.Address {
		addr, _ := network.SelectPublicAddress(networkAddresses(addresses))
		return addr
	}

	newAddr, changed := maybeGetNewAddress(publicAddress, providerAddresses, machineAddresses, getAddr, checkScope)
	if !changed {
		// No change, so no ops.
		return []txn.Op{}, nil
	}

	ops := m.setPreferredAddressOps(newAddr, true)
	return ops, &newAddr
}

func (m *Machine) setPrivateAddressOps(providerAddresses []address, machineAddresses []address) ([]txn.Op, *address) {
	privateAddress := m.doc.PreferredPrivateAddress
	// Always prefer an exact match if available.
	checkScope := func(addr address) bool {
		return network.ExactScopeMatch(
			addr.networkAddress(), network.ScopeMachineLocal, network.ScopeCloudLocal, network.ScopeFanLocal)
	}
	// Without an exact match, prefer a fallback match.
	getAddr := func(addresses []address) network.Address {
		addr, _ := network.SelectInternalAddress(networkAddresses(addresses), false)
		return addr
	}

	newAddr, changed := maybeGetNewAddress(privateAddress, providerAddresses, machineAddresses, getAddr, checkScope)
	if !changed {
		// No change, so no ops.
		return []txn.Op{}, nil
	}
	ops := m.setPreferredAddressOps(newAddr, false)
	return ops, &newAddr
}

// SetProviderAddresses records any addresses related to the machine, sourced
// by asking the provider.
func (m *Machine) SetProviderAddresses(addresses ...network.Address) error {
	err := m.setAddresses(nil, &addresses)
	return errors.Annotatef(err, "cannot set addresses of machine %v", m)
}

// ProviderAddresses returns any hostnames and ips associated with a machine,
// as determined by asking the provider.
func (m *Machine) ProviderAddresses() (addresses []network.Address) {
	for _, address := range m.doc.Addresses {
		addresses = append(addresses, address.networkAddress())
	}
	return
}

// MachineAddresses returns any hostnames and ips associated with a machine,
// determined by asking the machine itself.
func (m *Machine) MachineAddresses() (addresses []network.Address) {
	for _, address := range m.doc.MachineAddresses {
		addresses = append(addresses, address.networkAddress())
	}
	return
}

// SetMachineAddresses records any addresses related to the machine, sourced
// by asking the machine.
func (m *Machine) SetMachineAddresses(addresses ...network.Address) error {
	err := m.setAddresses(&addresses, nil)
	return errors.Annotatef(err, "cannot set machine addresses of machine %v", m)
}

// setAddresses updates the machine's addresses (either Addresses or
// MachineAddresses, depending on the field argument). Changes are
// only predicated on the machine not being Dead; concurrent address
// changes are ignored.
func (m *Machine) setAddresses(machineAddresses, providerAddresses *[]network.Address) error {
	var (
		machineStateAddresses, providerStateAddresses []address
		newPrivate, newPublic                         *address
		err                                           error
	)
	machine := m
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			if machine, err = machine.st.Machine(machine.doc.Id); err != nil {
				return nil, err
			}
		}
		var ops []txn.Op
		ops, machineStateAddresses, providerStateAddresses, newPrivate, newPublic, err = machine.setAddressesOps(
			machineAddresses, providerAddresses,
		)
		if err != nil {
			return nil, err
		}
		return ops, nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	m.doc.MachineAddresses = machineStateAddresses
	m.doc.Addresses = providerStateAddresses
	if newPrivate != nil {
		oldPrivate := m.doc.PreferredPrivateAddress.networkAddress()
		m.doc.PreferredPrivateAddress = *newPrivate
		logger.Infof(
			"machine %q preferred private address changed from %q to %q",
			m.Id(), oldPrivate, newPrivate.networkAddress(),
		)
	}
	if newPublic != nil {
		oldPublic := m.doc.PreferredPublicAddress.networkAddress()
		m.doc.PreferredPublicAddress = *newPublic
		logger.Infof(
			"machine %q preferred public address changed from %q to %q",
			m.Id(), oldPublic, newPublic.networkAddress(),
		)
	}
	return nil
}

func (m *Machine) setAddressesOps(
	machineAddresses, providerAddresses *[]network.Address,
) (_ []txn.Op, machineStateAddresses, providerStateAddresses []address, newPrivate, newPublic *address, _ error) {

	if m.doc.Life == Dead {
		return nil, nil, nil, nil, nil, ErrDead
	}

	fromNetwork := func(in []network.Address, origin Origin) []address {
		sorted := make([]network.Address, len(in))
		copy(sorted, in)
		network.SortAddresses(sorted)
		return fromNetworkAddresses(sorted, origin)
	}

	var set bson.D
	machineStateAddresses = m.doc.MachineAddresses
	providerStateAddresses = m.doc.Addresses
	if machineAddresses != nil {
		machineStateAddresses = fromNetwork(*machineAddresses, OriginMachine)
		set = append(set, bson.DocElem{"machineaddresses", machineStateAddresses})
	}
	if providerAddresses != nil {
		providerStateAddresses = fromNetwork(*providerAddresses, OriginProvider)
		set = append(set, bson.DocElem{"addresses", providerStateAddresses})
	}

	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", set}},
	}}

	setPrivateAddressOps, newPrivate := m.setPrivateAddressOps(providerStateAddresses, machineStateAddresses)
	setPublicAddressOps, newPublic := m.setPublicAddressOps(providerStateAddresses, machineStateAddresses)
	ops = append(ops, setPrivateAddressOps...)
	ops = append(ops, setPublicAddressOps...)
	return ops, machineStateAddresses, providerStateAddresses, newPrivate, newPublic, nil
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
	op := m.UpdateOperation()
	op.Constraints = &cons
	return m.st.ApplyOperation(op)
}

func (m *Machine) setConstraintsOps(cons constraints.Value) ([]txn.Op, error) {
	unsupported, err := m.st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting constraints on machine %q: unsupported constraints: %v",
			m.Id(), strings.Join(unsupported, ","),
		)
	} else if err != nil {
		return nil, err
	}

	if m.doc.Life != Alive {
		return nil, machineNotAliveErr
	}
	if _, err := m.InstanceId(); err == nil {
		return nil, fmt.Errorf("machine is already provisioned")
	} else if !errors.IsNotProvisioned(err) {
		return nil, err
	}

	notSetYet := bson.D{{"nonce", ""}}
	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: append(isAliveDoc, notSetYet...),
	}}
	mcons, err := m.st.resolveMachineConstraints(cons)
	if err != nil {
		return nil, err
	}
	ops = append(ops, setConstraintsOp(m.globalKey(), mcons))
	return ops, nil
}

// Status returns the status of the machine.
func (m *Machine) Status() (status.StatusInfo, error) {
	mStatus, err := getStatus(m.st.db(), m.globalKey(), "machine")
	if err != nil {
		return mStatus, err
	}
	return mStatus, nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(statusInfo status.StatusInfo) error {
	switch statusInfo.Status {
	case status.Started, status.Stopped:
	case status.Error:
		if statusInfo.Message == "" {
			return errors.Errorf("cannot set status %q without info", statusInfo.Status)
		}
	case status.Pending:
		// If a machine is not yet provisioned, we allow its status
		// to be set back to pending (when a retry is to occur).
		_, err := m.InstanceId()
		allowPending := errors.IsNotProvisioned(err)
		if allowPending {
			break
		}
		fallthrough
	case status.Down:
		return errors.Errorf("cannot set status %q", statusInfo.Status)
	default:
		return errors.Errorf("cannot set invalid status %q", statusInfo.Status)
	}
	return setStatus(m.st.db(), setStatusParams{
		badge:     "machine",
		globalKey: m.globalKey(),
		status:    statusInfo.Status,
		message:   statusInfo.Message,
		rawData:   statusInfo.Data,
		updated:   timeOrNow(statusInfo.Since, m.st.clock()),
	})
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this machine.
func (m *Machine) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        m.st.db(),
		globalKey: m.globalKey(),
		filter:    filter,
	}
	return statusHistory(args)
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
	if err = m.st.db().RunTransaction(ops); err != nil {
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
			statusInfo, err := container.Status()
			if err != nil {
				logger.Errorf("finding status of container %v to mark as invalid: %v", containerId, err)
				continue
			}
			if statusInfo.Status == status.Pending {
				containerType := ContainerTypeFromId(containerId)
				now := m.st.clock().Now()
				s := status.StatusInfo{
					Status:  status.Error,
					Message: "unsupported container",
					Data:    map[string]interface{}{"type": containerType},
					Since:   &now,
				}
				container.SetStatus(s)
			} else {
				logger.Errorf("unsupported container %v has unexpected status %v", containerId, statusInfo.Status)
			}
		}
	}
	return nil
}

// SetMachineBlockDevices sets the block devices visible on the machine.
func (m *Machine) SetMachineBlockDevices(info ...BlockDeviceInfo) error {
	return setMachineBlockDevices(m.st, m.Id(), info)
}

// VolumeAttachments returns the machine's volume attachments.
func (m *Machine) VolumeAttachments() ([]VolumeAttachment, error) {
	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sb.MachineVolumeAttachments(m.MachineTag())
}

// AddAction is part of the ActionReceiver interface.
func (m *Machine) AddAction(name string, payload map[string]interface{}) (Action, error) {
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		return nil, errors.Errorf("cannot add action %q to a machine; only predefined actions allowed", name)
	}

	// Reject bad payloads before attempting to insert defaults.
	err := spec.ValidateParams(payload)
	if err != nil {
		return nil, err
	}
	payloadWithDefaults, err := spec.InsertDefaults(payload)
	if err != nil {
		return nil, err
	}

	model, err := m.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return model.EnqueueAction(m.Tag(), name, payloadWithDefaults)
}

// CancelAction is part of the ActionReceiver interface.
func (m *Machine) CancelAction(action Action) (Action, error) {
	return action.Finish(ActionResults{Status: ActionCancelled})
}

// WatchActionNotifications is part of the ActionReceiver interface.
func (m *Machine) WatchActionNotifications() StringsWatcher {
	return m.st.watchEnqueuedActionsFilteredBy(m)
}

// Actions is part of the ActionReceiver interface.
func (m *Machine) Actions() ([]Action, error) {
	return m.st.matchingActions(m)
}

// CompletedActions is part of the ActionReceiver interface.
func (m *Machine) CompletedActions() ([]Action, error) {
	return m.st.matchingActionsCompleted(m)
}

// PendingActions is part of the ActionReceiver interface.
func (m *Machine) PendingActions() ([]Action, error) {
	return m.st.matchingActionsPending(m)
}

// RunningActions is part of the ActionReceiver interface.
func (m *Machine) RunningActions() ([]Action, error) {
	return m.st.matchingActionsRunning(m)
}

// UpdateMachineSeries updates the series for the Machine.
func (m *Machine) UpdateMachineSeries(series string, force bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		// Exit early if the Machine series doesn't need to change.
		if m.Series() == series {
			return nil, jujutxn.ErrNoOperations
		}

		principals := m.Principals() // unit names
		verifiedUnits, err := m.VerifyUnitsSeries(principals, series, force)
		if err != nil {
			return nil, err
		}

		ops := []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"life", Alive}, {"principals", principals}},
			Update: bson.D{{"$set", bson.D{{"series", series}}}},
		}}
		for _, unit := range verifiedUnits {
			curl, _ := unit.CharmURL()
			ops = append(ops, txn.Op{
				C:  unitsC,
				Id: unit.doc.DocID,
				Assert: bson.D{{"life", Alive},
					{"charmurl", curl},
					{"subordinates", unit.SubordinateNames()}},
				Update: bson.D{{"$set", bson.D{{"series", series}}}},
			})
		}

		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	return errors.Annotatef(err, "cannot update series for %q to %s", m, series)
}

// VerifyUnitsSeries iterates over the units with the input names, and checks
// that the application for each supports the input series.
// Recursion is used to verify all subordinates, with the results accrued into
// a slice before returning.
func (m *Machine) VerifyUnitsSeries(unitNames []string, series string, force bool) ([]*Unit, error) {
	var results []*Unit
	for _, u := range unitNames {
		unit, err := m.st.Unit(u)
		if err != nil {
			return nil, err
		}
		app, err := unit.Application()
		if err != nil {
			return nil, err
		}
		err = app.VerifySupportedSeries(series, force)
		if err != nil {
			return nil, err
		}

		subordinates := unit.SubordinateNames()
		subUnits, err := m.VerifyUnitsSeries(subordinates, series, force)
		if err != nil {
			return nil, err
		}
		results = append(results, unit)
		results = append(results, subUnits...)
	}
	return results, nil
}

// SetUpgradeCharmProfile sets a application name and a charm url for
// machine's needing a charm profile change.  For a container only.
func (m *Machine) SetUpgradeCharmProfile(appName, chURL string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		life := m.Life()
		if life == Dead || life == Dying {
			return nil, ErrDead
		}

		ops := []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"life", Alive}},
			Update: bson.D{{"$set", bson.D{{"upgradecharmprofilecharmurl", chURL},
				{"upgradecharmprofileapplication", appName}}}},
		}}

		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		return err
	}
	m.doc.UpgradeCharmProfileApplication = appName
	m.doc.UpgradeCharmProfileCharmURL = chURL
	return nil
}

// UpdateOperation returns a model operation that will update the machine.
func (m *Machine) UpdateOperation() *UpdateMachineOperation {
	return &UpdateMachineOperation{m: &Machine{st: m.st, doc: m.doc}}
}

// UpdateMachineOperation is a model operation for updating a machine.
type UpdateMachineOperation struct {
	// m holds the machine to update.
	m *Machine

	AgentVersion      *version.Binary
	Constraints       *constraints.Value
	HasVote           *bool
	MachineAddresses  *[]network.Address
	ProviderAddresses *[]network.Address
	PasswordHash      *string
}

// Build is part of the ModelOperation interface.
func (op *UpdateMachineOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.m.Refresh(); err != nil {
			return nil, err
		}
	}

	var allOps []txn.Op

	if op.AgentVersion != nil {
		ops, _, err := op.m.setAgentVersionOps(*op.AgentVersion)
		if err != nil {
			return nil, errors.Annotate(err, "cannot set agent version")
		}
		allOps = append(allOps, ops...)
	}

	if op.Constraints != nil {
		ops, err := op.m.setConstraintsOps(*op.Constraints)
		if err != nil {
			return nil, errors.Annotate(err, "cannot set constraints")
		}
		allOps = append(allOps, ops...)
	}

	if op.HasVote != nil {
		ops, err := op.m.setHasVoteOps(*op.HasVote)
		if err != nil {
			return nil, errors.Annotate(err, "cannot set has-vote")
		}
		allOps = append(allOps, ops...)
	}

	if op.MachineAddresses != nil || op.ProviderAddresses != nil {
		ops, _, _, _, _, err := op.m.setAddressesOps(op.MachineAddresses, op.ProviderAddresses)
		if err != nil {
			return nil, errors.Annotate(err, "cannot set addresses")
		}
		allOps = append(allOps, ops...)
	}

	if op.PasswordHash != nil {
		ops, err := op.m.setPasswordHashOps(*op.PasswordHash)
		if err != nil {
			return nil, errors.Annotate(err, "cannot set password")
		}
		allOps = append(allOps, ops...)
	}

	return allOps, nil
}

// Done is part of the ModelOperation interface.
func (op *UpdateMachineOperation) Done(err error) error {
	return errors.Annotatef(err, "updating machine %q", op.m)
}
