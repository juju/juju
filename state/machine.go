// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/constraints"
	corecontainer "github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/mongo"
	stateerrors "github.com/juju/juju/state/errors"
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
	jobNames = map[MachineJob]model.MachineJob{
		JobHostUnits:   model.JobHostUnits,
		JobManageModel: model.JobManageModel,
	}
	jobMigrationValue = map[MachineJob]string{
		JobHostUnits:   "host-units",
		JobManageModel: "api-server",
	}
)

// ToParams returns the job as model.MachineJob.
func (job MachineJob) ToParams() model.MachineJob {
	if jujuJob, ok := jobNames[job]; ok {
		return jujuJob
	}
	return model.MachineJob(fmt.Sprintf("<unknown job %d>", int(job)))
}

// paramsJobsFromJobs converts state jobs to juju jobs.
func paramsJobsFromJobs(jobs []MachineJob) []model.MachineJob {
	jujuJobs := make([]model.MachineJob, len(jobs))
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
	DocID          string `bson:"_id"`
	Id             string `bson:"machineid"`
	ModelUUID      string `bson:"model-uuid"`
	Base           Base   `bson:"base"`
	Nonce          string
	ContainerType  string
	Principals     []string
	Life           Life
	Tools          *tools.Tools `bson:",omitempty"`
	Jobs           []MachineJob
	PasswordHash   string
	Clean          bool
	ForceDestroyed bool `bson:"force-destroyed"`

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

	// AgentStartedAt records the time when the machine agent started.
	AgentStartedAt time.Time `bson:"agent-started-at,omitempty"`

	// Hostname records the machine's hostname as reported by the machine agent.
	Hostname string `bson:"hostname,omitempty"`
}

func newMachine(st *State, doc *machineDoc) *Machine {
	machine := &Machine{
		st:  st,
		doc: *doc,
	}
	return machine
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.doc.Id
}

// Principals returns the principals for the machine.
func (m *Machine) Principals() []string {
	return m.doc.Principals
}

// Base returns the os base running on the machine.
func (m *Machine) Base() Base {
	return m.doc.Base
}

// ContainerType returns the type of container hosting this machine.
func (m *Machine) ContainerType() instance.ContainerType {
	return instance.ContainerType(m.doc.ContainerType)
}

// ModelUUID returns the unique identifier
// for the model that this machine is in.
func (m *Machine) ModelUUID() string {
	return m.doc.ModelUUID
}

// ForceDestroyed returns whether the destruction of a dying/dead
// machine was forced. It's always false for a machine that's alive.
func (m *Machine) ForceDestroyed() bool {
	return m.doc.ForceDestroyed
}

func (m *Machine) forceDestroyedOps() []txn.Op {
	return []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"force-destroyed", true}}}},
	}}
}

// machineGlobalKey returns the global database key for the identified machine.
func machineGlobalKey(id string) string {
	return "m#" + id
}

// machineGlobalInstanceKey returns the global database key for the identified
// machine's instance.
func machineGlobalInstanceKey(id string) string {
	return machineGlobalKey(id) + "#instance"
}

// globalInstanceKey returns the global database key for the machine's instance.
func (m *Machine) globalInstanceKey() string {
	return machineGlobalInstanceKey(m.doc.Id)
}

// machineGlobalModificationKey returns the global database key for the
// identified machine's modification changes.
func machineGlobalModificationKey(id string) string {
	return machineGlobalKey(id) + "#modification"
}

// globalModificationKey returns the global database key for the machine's
// modification changes.
func (m *Machine) globalModificationKey() string {
	return machineGlobalModificationKey(m.doc.Id)
}

// globalKey returns the global database key for the machine.
func (m *Machine) globalKey() string {
	return machineGlobalKey(m.doc.Id)
}

// instanceData holds attributes relevant to a provisioned machine.
type instanceData struct {
	DocID          string      `bson:"_id"`
	MachineId      string      `bson:"machineid"`
	InstanceId     instance.Id `bson:"instanceid"`
	DisplayName    string      `bson:"display-name"`
	ModelUUID      string      `bson:"model-uuid"`
	Arch           *string     `bson:"arch,omitempty"`
	Mem            *uint64     `bson:"mem,omitempty"`
	RootDisk       *uint64     `bson:"rootdisk,omitempty"`
	RootDiskSource *string     `bson:"rootdisksource,omitempty"`
	CpuCores       *uint64     `bson:"cpucores,omitempty"`
	CpuPower       *uint64     `bson:"cpupower,omitempty"`
	Tags           *[]string   `bson:"tags,omitempty"`
	AvailZone      *string     `bson:"availzone,omitempty"`
	VirtType       *string     `bson:"virt-type,omitempty"`

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
		RootDiskSource:   instData.RootDiskSource,
		CpuCores:         instData.CpuCores,
		CpuPower:         instData.CpuPower,
		Tags:             instData.Tags,
		AvailabilityZone: instData.AvailZone,
		VirtType:         instData.VirtType,
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

// removeInstanceDataOp returns the operation needed to remove the
// instance data document associated with the given globalKey.
func removeInstanceDataOp(globalKey string) txn.Op {
	return txn.Op{
		C:      instanceDataC,
		Id:     globalKey,
		Remove: true,
	}
}

// AllInstanceData retrieves all instance data in the model
// and provides a way to query hardware characteristics and
// charm profiles by machine.
func (m *Model) AllInstanceData() (*ModelInstanceData, error) {
	coll, closer := m.st.db().GetCollection(instanceDataC)
	defer closer()

	var docs []instanceData
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all instance data for model")
	}
	all := &ModelInstanceData{
		data: make(map[string]instanceData),
	}
	for _, doc := range docs {
		all.data[doc.MachineId] = doc
	}
	return all, nil
}

// ModelInstanceData represents all the instance data for a model
// keyed on machine ID.
type ModelInstanceData struct {
	data map[string]instanceData
}

// HardwareCharacteristics returns the hardware characteristics of the
// machine. If it isn't found in the map, a nil is returned.
func (d *ModelInstanceData) HardwareCharacteristics(machineID string) *instance.HardwareCharacteristics {
	instData, found := d.data[machineID]
	if !found {
		return nil
	}
	return hardwareCharacteristics(instData)
}

// CharmProfiles returns the names of the profiles that are defined for
// the machine. If the machine isn't found in the map, a nil is returned.
func (d *ModelInstanceData) CharmProfiles(machineID string) []string {
	instData, found := d.data[machineID]
	if !found {
		return nil
	}
	return instData.CharmProfiles
}

// InstanceNames returns both the provider instance id and the user
// friendly name. If the machine isn't found, empty strings are returned.
func (d *ModelInstanceData) InstanceNames(machineID string) (instance.Id, string) {
	instData, found := d.data[machineID]
	if !found {
		return "", ""
	}
	return instData.InstanceId, instData.DisplayName
}

// Len returns the number of machines.
func (d *ModelInstanceData) Len() int {
	return len(d.data)
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
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// Exit early if the Machine profiles doesn't need to change.
		currentProfiles, err := m.CharmProfiles()
		if err != nil {
			return nil, errors.Trace(err)
		}
		currentProfileSet := set.NewStrings(currentProfiles...)
		newProfileSet := set.NewStrings(profiles...)
		if currentProfileSet.Size() == newProfileSet.Size() &&
			currentProfileSet.Intersection(newProfileSet).Size() == currentProfileSet.Size() {
			return nil, jujutxn.ErrNoOperations
		}

		assertion := bson.M{"charm-profiles": currentProfiles}
		if currentProfiles == nil {
			assertion = bson.M{"charm-profiles": bson.D{{"$exists", false}}}
		}
		ops := []txn.Op{{
			C:      instanceDataC,
			Id:     m.doc.DocID,
			Assert: assertion,
			Update: bson.D{{"$set", bson.D{{"charm-profiles", profiles}}}},
		}}

		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	return errors.Annotatef(err, "cannot update profiles for %q to %s", m, strings.Join(profiles, ", "))
}

// IsManager returns true if the machine has JobManageModel.
func (m *Machine) IsManager() bool {
	return isController(&m.doc)
}

// IsManual returns true if the machine was manually provisioned.
func (m *Machine) IsManual() (bool, error) {
	// To avoid unnecessary db lookups, a little of the
	// logic from isManualMachine() below is duplicated here
	// so we can exit early if possible.
	if strings.HasPrefix(m.doc.Nonce, manualMachinePrefix) {
		return true, nil
	}
	if m.doc.Id != "0" {
		return false, nil
	}
	modelSettings, err := readSettings(m.st.db(), settingsC, modelGlobalKey)
	if err != nil {
		return false, errors.Trace(err)
	}
	providerRaw, _ := modelSettings.Get("type")
	providerType, _ := providerRaw.(string)
	return isManualMachine(m.doc.Id, m.doc.Nonce, providerType), nil
}

func isManualMachine(id, nonce, providerType string) bool {
	// Apart from the bootstrap machine, manually provisioned
	// machines have a nonce prefixed with "manual:". This is
	// unique to manual provisioning.
	if strings.HasPrefix(nonce, manualMachinePrefix) {
		return true
	}
	// The bootstrap machine uses BootstrapNonce, so in that
	// case we need to check if its provider type is "manual".
	// We also check for "null", which is an alias for manual.
	return id == "0" && (providerType == "null" || providerType == "manual")
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
	if v.Release == "" || v.Arch == "" {
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
		return onAbort(err, stateerrors.ErrDead)
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
		return nil, stateerrors.ErrDead
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
// a HasAssignedUnitsError.  If the machine has containers, Destroy
// will return HasContainersError.
func (m *Machine) Destroy() error {
	return errors.Trace(m.advanceLifecycle(Dying, false, false, 0))
}

// DestroyWithContainers sets the machine lifecycle to Dying if it is Alive.
// It does nothing otherwise. DestroyWithContainers will fail if the machine
// has principal units assigned, or if the machine has JobManageModel. If the
// machine has assigned units, DestroyWithContainers will return a
// HasAssignedUnitsError.  The machine is allowed to have containers.  Use with
// caution.  Intended for model tear down.
func (m *Machine) DestroyWithContainers() error {
	return m.advanceLifecycle(Dying, false, true, 0)
}

// ForceDestroy queues the machine for complete removal, including the
// destruction of all units and containers on the machine.
func (m *Machine) ForceDestroy(maxWait time.Duration) error {
	ops, err := m.forceDestroyOps(maxWait)
	if err != nil {
		return errors.Trace(err)
	}
	if err := m.st.db().RunTransaction(ops); err != txn.ErrAborted {
		return errors.Annotatef(err, "failed to run transaction: %s", pretty.Sprint(ops))
	}
	return nil
}

func (m *Machine) forceDestroyOps(maxWait time.Duration) ([]txn.Op, error) {
	if m.IsManager() {
		controllerIds, err := m.st.ControllerIds()
		if err != nil {
			return nil, errors.Annotatef(err, "reading controller info")
		}
		if len(controllerIds) <= 1 {
			return nil, errors.Errorf("controller %s is the only controller", m.Id())
		}
		return []txn.Op{
			{
				C:  machinesC,
				Id: m.doc.DocID,
				Assert: bson.D{
					{"life", Alive},
					advanceLifecycleUnitAsserts(m.doc.Principals),
				},
				// To prevent race conditions, we remove the ability for new
				// units to be deployed to the machine.
				Update: bson.D{{"$pull", bson.D{{"jobs", JobHostUnits}}}},
			},
			{
				C:      controllersC,
				Id:     modelGlobalKey,
				Assert: bson.D{{"controller-ids", controllerIds}},
			},
			setControllerWantsVoteOp(m.st, m.Id(), false),
			newCleanupOp(cleanupEvacuateMachine, m.doc.Id),
		}, nil
	} else {
		// Make sure the machine doesn't become a manager while we're destroying it
		return []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"jobs", bson.D{{"$nin", []MachineJob{JobManageModel}}}}},
		}, newCleanupOp(cleanupForceDestroyedMachine, m.doc.Id, maxWait),
		}, nil
	}
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageModel.
// If the machine has assigned units, EnsureDead will return
// a HasAssignedUnitsError.
func (m *Machine) EnsureDead() error {
	return m.advanceLifecycle(Dead, false, false, 0)
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
	parentId := corecontainer.ParentId(m.Id())
	return parentId, parentId != ""
}

// IsContainer returns true if the machine is a container.
func (m *Machine) IsContainer() bool {
	_, isContainer := m.ParentId()
	return isContainer
}

// advanceLifecycle ensures that the machine's lifecycle is no earlier
// than the supplied value. If the machine already has that lifecycle
// value, or a later one, no changes will be made to remote state. If
// the machine has any responsibilities that preclude a valid change in
// lifecycle, it will return an error. dyingAllowContainers indicates
// whether the machine can have containers when moving to the dying state.
// Not allowed for moving to dead.
func (original *Machine) advanceLifecycle(life Life, force, dyingAllowContainers bool, maxWait time.Duration) (err error) {
	logger.Debugf("%s.advanceLifecycle(%s, %t, %t)", original.Id(), life, force, dyingAllowContainers)

	if life == Dead && dyingAllowContainers {
		return errors.BadRequestf("life cannot be Dead if dyingAllowContainers true.")
	}

	// A machine can be set to dying with containers, but cannot have any when
	// advanced to dead.
	if !dyingAllowContainers && life == Dying {
		if err := original.advanceLifecycleIfNoContainers(); err != nil {
			return err
		}
	}

	locked, err := original.IsLockedForSeriesUpgrade()
	if err != nil {
		return errors.Annotatef(err, "reading machine %s upgrade-series lock", original.Id())
	}
	if locked {
		return errors.Errorf("machine %s is locked for series upgrade", original.Id())
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

	ops := m.advanceLifecyleInitialOps(life)

	// multiple attempts: one with original data, one with refreshed data, and a final
	// one intended to determine the cause of failure of the preceding attempt.
	buildTxn := func(attempt int) ([]txn.Op, error) {
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
		node, err := m.st.ControllerNode(m.doc.Id)
		if err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		hasVote := err == nil && node.HasVote()

		// Check that the life change is sane, and collect the assertions
		// necessary to determine that it remains so.
		switch life {
		case Dying:
			if m.doc.Life != Alive {
				return nil, jujutxn.ErrNoOperations
			}
			// Manager nodes are allowed to go to dying even when they have
			// the vote, as that is used as the signal that they should lose
			// their vote.
			asserts = append(asserts, isAliveDoc...)
		case Dead:
			if m.doc.Life == Dead {
				return nil, jujutxn.ErrNoOperations
			}
			if hasVote || m.IsManager() {
				return nil, stateerrors.NewIsControllerMemberError(m.Id(), hasVote)
			}
			asserts = append(asserts, bson.DocElem{
				Name: "jobs", Value: bson.D{{Name: "$nin", Value: []MachineJob{JobManageModel}}}})
			asserts = append(asserts, notDeadDoc...)
			ops = append(ops, controllerAdvanceLifecyleVoteOp())
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
			if isController(&m.doc) || hasVote {
				// If we're responsible for managing the model, make sure we ask to drop our vote
				ops[0].Update = bson.D{
					{"$set", bson.D{{"life", life}}},
				}
				controllerOp, err := m.controllerIDsOp()
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, controllerOp)
				ops = append(ops, setControllerWantsVoteOp(m.st, m.doc.Id, false))
			}

			var principalUnitNames []string
			for _, principalUnit := range m.doc.Principals {
				principalUnitNames = append(principalUnitNames, principalUnit)
				canDie, err = m.assessCanDieUnit(principalUnit)
				if err != nil {
					return nil, errors.Trace(err)
				}
				if !canDie {
					break
				}
			}

			if canDie && !dyingAllowContainers {
				err := m.advanceLifecycleIfNoContainers()
				if errors.Is(err, stateerrors.HasContainersError) {
					canDie = false
				} else if err != nil {
					return nil, err
				}
				ops = append(ops, m.noContainersOp())
			}

			cleanupOp := newCleanupOp(cleanupDyingMachine, m.doc.Id, force, maxWait)
			ops = append(ops, cleanupOp)

			if canDie {
				ops[0].Assert = append(asserts, advanceLifecycleUnitAsserts(principalUnitNames))
				txnLogger.Debugf("txn moving machine %q to %s", m.Id(), life)
				return ops, nil
			}
		}

		if len(m.doc.Principals) > 0 {
			return nil, stateerrors.NewHasAssignedUnitsError(m.doc.Id, m.doc.Principals)
		}
		asserts = append(asserts, noUnitAsserts())

		if life == Dead {
			// A machine may not become Dead until it has no
			// containers.
			if err := m.advanceLifecycleIfNoContainers(); err != nil {
				return nil, err
			}
			ops = append(ops, m.noContainersOp())
			if isController(&m.doc) {
				return nil, errors.Errorf("machine %s is still responsible for being a controller", m.Id())
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
		return ops, nil
	}

	if err = m.st.db().Run(buildTxn); err == jujutxn.ErrExcessiveContention {
		err = errors.Annotatef(err, "machine %s cannot advance lifecycle", m)
	}
	return err
}

func (m *Machine) advanceLifecyleInitialOps(life Life) []txn.Op {
	return []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Update: bson.D{{"$set", bson.D{{"life", life}}}},
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     m.doc.Id,
			Assert: txn.DocMissing,
		},
	}
}

func controllerAdvanceLifecyleVoteOp() txn.Op {
	return txn.Op{
		C:  controllersC,
		Id: modelGlobalKey,
		Assert: bson.D{
			{"has-vote", bson.M{"$ne": true}},
			{"wants-vote", bson.M{"$ne": true}},
		},
	}
}

// controllerIDsOp returns an Op to assert that the machine's
// controllerIDs do not change.
func (m *Machine) controllerIDsOp() (txn.Op, error) {
	controllerIds, err := m.st.ControllerIds()
	if err != nil {
		return txn.Op{}, errors.Annotatef(err, "reading controller info")
	}
	if len(controllerIds) <= 1 {
		return txn.Op{}, errors.Errorf("controller %s is the only controller", m.Id())
	}
	return txn.Op{
		C:      controllersC,
		Id:     modelGlobalKey,
		Assert: bson.D{{"controller-ids", controllerIds}},
	}, nil
}

// noContainersOp returns an Op to assert that the machine
// has no containers.
func (m *Machine) noContainersOp() txn.Op {
	return txn.Op{
		C:  containerRefsC,
		Id: m.doc.DocID,
		Assert: bson.D{{"$or", []bson.D{
			{{"children", bson.D{{"$size", 0}}}},
			{{"children", bson.D{{"$exists", false}}}},
		}}},
	}
}

// assessCanDieUnit returns true if the machine can die, based on
// evaluating the provided unit.
func (m *Machine) assessCanDieUnit(principalUnit string) (bool, error) {
	canDie := true
	u, err := m.st.Unit(principalUnit)
	if err != nil {
		return false, errors.Annotatef(err, "reading machine %s principal unit %v", m, m.doc.Principals[0])
	}
	app, err := u.Application()
	if err != nil {
		return false, errors.Annotatef(err, "reading machine %s principal unit application %v", m, u.doc.Application)
	}
	if u.Life() == Alive && app.Life() == Alive {
		canDie = false
	}
	return canDie, nil
}

// noUnitAsserts returns bson DocElem which assert that there are
// no units for the machine.
func noUnitAsserts() bson.DocElem {
	return bson.DocElem{
		Name: "$or", Value: []bson.D{
			{{"principals", bson.D{{"$size", 0}}}},
			{{"principals", bson.D{{"$exists", false}}}},
		},
	}
}

// advanceLifecycleUnitAsserts returns bson DocElem which assert that there are
// no units for the machine, or that the list of units has not changed.
func advanceLifecycleUnitAsserts(principalUnitNames []string) bson.DocElem {
	return bson.DocElem{
		Name: "$or", Value: []bson.D{
			{{Name: "principals", Value: principalUnitNames}},
			{{Name: "principals", Value: bson.D{{"$size", 0}}}},
			{{Name: "principals", Value: bson.D{{"$exists", false}}}},
		},
	}
}

// advanceLifecycleIfNoContainers determines if the machine has
// containers, if so, returns the appropriate error.
func (m *Machine) advanceLifecycleIfNoContainers() error {
	containers, err := m.Containers()
	if err != nil {
		return errors.Annotatef(err, "reading machine %s containers", m)
	}

	if len(containers) > 0 {
		return stateerrors.NewHasContainersError(m.doc.Id, containers)
	}
	return nil
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
		return nil, stateerrors.NewHasAttachmentsError(m.doc.Id, attachments.SortedValues())
	}
	if m.doc.Life == Dying {
		return nil, nil
	}
	// A Dying machine cannot have attachments added to it,
	// but if we're advancing from Alive to Dead then we
	// must ensure no concurrent attachments are made.
	noNewVolumes := bson.DocElem{
		Name: "volumes", Value: bson.D{{
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
		Name: "filesystems", Value: bson.D{{
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
	machRanges, err := m.OpenedPortRanges()
	if err != nil {
		return nil, err
	}

	mpr := machRanges.(*machinePortRanges)
	if !mpr.Persisted() {
		return nil, nil
	}
	return mpr.removeOps(), nil
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
		removeStatusOp(m.st, m.globalModificationKey()),
		removeConstraintsOp(m.globalKey()),
		annotationRemoveOp(m.st, m.globalKey()),
		removeRebootDocOp(m.st, m.globalKey()),
		removeMachineBlockDevicesOp(m.Id()),
		removeModelMachineRefOp(m.st, m.Id()),
		removeSSHHostKeyOp(m.globalKey()),
		removeInstanceDataOp(m.doc.DocID),
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

	ops = append(ops, removeControllerNodeOp(m.st, m.Id()))
	ops = append(ops, linkLayerDevicesOps...)
	ops = append(ops, devicesAddressesOps...)
	ops = append(ops, portsOps...)
	ops = append(ops, removeContainerRefOps(m.st, m.Id())...)
	ops = append(ops, filesystemOps...)
	ops = append(ops, volumeOps...)
	ops = append(ops, removeMachineVirtualHostKeyOps(m.st, m.Id())...)

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

// InstanceId returns the provider specific instance id for this
// machine, or a NotProvisionedError, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	instId, _, err := m.InstanceNames()
	return instId, err
}

// InstanceNames returns both the provider's instance id and a user-friendly
// display name. The display name is intended used for human input and
// is ignored internally.
func (m *Machine) InstanceNames() (instance.Id, string, error) {
	instData, err := getInstanceData(m.st, m.Id())
	if errors.IsNotFound(err) {
		err = errors.NotProvisionedf("machine %v", m.Id())
	}
	if err != nil {
		return "", "", err
	}
	return instData.InstanceId, instData.DisplayName, nil
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
		clock:     m.st.clock(),
	}
	return statusHistory(args)
}

// ModificationStatus returns the provider specific modification status for
// this machine or NotProvisionedError if instance is not yet provisioned.
func (m *Machine) ModificationStatus() (status.StatusInfo, error) {
	machineStatus, err := getStatus(m.st.db(), m.globalModificationKey(), "modification")
	if err != nil {
		logger.Warningf("error when retrieving instance status for machine: %s, %v", m.Id(), err)
		return status.StatusInfo{}, err
	}
	return machineStatus, nil
}

// SetModificationStatus sets the provider specific modification status
// for a machine. Allowing the propagation of status messages to the
// operator.
func (m *Machine) SetModificationStatus(sInfo status.StatusInfo) (err error) {
	return setStatus(m.st.db(), setStatusParams{
		badge:     "modification",
		globalKey: m.globalModificationKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   timeOrNow(sInfo.Since, m.st.clock()),
	})
}

// AvailabilityZone returns the provider-specific instance availability
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

// ApplicationNames returns the names of applications
// represented by units running on the machine.
func (m *Machine) ApplicationNames() ([]string, error) {
	units, err := m.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	apps := set.NewStrings()
	for _, unit := range units {
		apps.Add(unit.ApplicationName())
	}
	return apps.SortedValues(), nil
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
	}
	return units, nil
}

// SetProvisioned stores the machine's provider-specific details in the
// database. These details are used to infer that the machine has
// been provisioned.
//
// When provisioning an instance, a nonce should be created and passed
// when starting it, before adding the machine to the state. This means
// that if the provisioner crashes (or its connection to the state is
// lost) after starting the instance, we can be sure that only a single
// instance will be able to act for that machine.
//
// Once set, the instance id cannot be changed. A non-empty instance id
// will be detected as a provisioned machine.
func (m *Machine) SetProvisioned(
	id instance.Id,
	displayName string,
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
		DocID:          m.doc.DocID,
		MachineId:      m.doc.Id,
		InstanceId:     id,
		DisplayName:    displayName,
		ModelUUID:      m.doc.ModelUUID,
		Arch:           characteristics.Arch,
		Mem:            characteristics.Mem,
		RootDisk:       characteristics.RootDisk,
		RootDiskSource: characteristics.RootDiskSource,
		CpuCores:       characteristics.CpuCores,
		CpuPower:       characteristics.CpuPower,
		Tags:           characteristics.Tags,
		AvailZone:      characteristics.AvailabilityZone,
		VirtType:       characteristics.VirtType,
	}

	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: append(notDeadDoc, bson.DocElem{Name: "nonce", Value: ""}),
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
	} else if aliveOrDying, err := isNotDead(m.st, machinesC, m.doc.DocID); err != nil {
		return err
	} else if !aliveOrDying {
		return errDeadOrGone
	}
	return fmt.Errorf("already set")
}

// SetInstanceInfo is used to provision a machine and in one step sets its
// instance ID, nonce, hardware characteristics, add link-layer devices and set
// their addresses as needed.  After, set charm profiles if needed.
func (m *Machine) SetInstanceInfo(
	id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
	devicesArgs []LinkLayerDeviceArgs, devicesAddrs []LinkLayerDeviceAddress,
	volumes map[names.VolumeTag]VolumeInfo,
	volumeAttachments map[names.VolumeTag]VolumeAttachmentInfo,
	charmProfiles []string,
) error {
	logger.Tracef(
		"setting instance info: machine %v, deviceAddrs: %#v, devicesArgs: %#v",
		m.Id(), devicesAddrs, devicesArgs)

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

	if err := m.SetProvisioned(id, displayName, nonce, characteristics); err != nil {
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
func (m *Machine) Addresses() (addresses network.SpaceAddresses) {
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
func (m *Machine) PublicAddress() (network.SpaceAddress, error) {
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
	getAddr func([]address) network.SpaceAddress,
	checkScope func(address) bool,
) (address, bool) {
	// For picking the best address, try provider addresses first.
	var newAddr address
	netAddr := getAddr(providerAddresses)
	if netAddr.Value == "" {
		netAddr = getAddr(machineAddresses)
		newAddr = fromNetworkAddress(netAddr, network.OriginMachine)
	} else {
		newAddr = fromNetworkAddress(netAddr, network.OriginProvider)
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
	if network.Origin(addr.Origin) != network.OriginProvider &&
		network.Origin(newAddr.Origin) == network.OriginProvider {
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
func (m *Machine) PrivateAddress() (network.SpaceAddress, error) {
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
	if current.SpaceID != "" {
		currentD = append(currentD, bson.D{{fieldName + ".spaceid", current.SpaceID}})
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
	getAddr := func(addresses []address) network.SpaceAddress {
		addr, _ := networkAddresses(addresses).OneMatchingScope(network.ScopeMatchPublic)
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
	getAddr := func(addresses []address) network.SpaceAddress {
		addr, _ := networkAddresses(addresses).OneMatchingScope(network.ScopeMatchCloudLocal)
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
func (m *Machine) SetProviderAddresses(addresses ...network.SpaceAddress) error {
	err := m.setAddresses(nil, &addresses)
	return errors.Annotatef(err, "cannot set addresses of machine %v", m)
}

// ProviderAddresses returns any hostnames and ips associated with a machine,
// as determined by asking the provider.
func (m *Machine) ProviderAddresses() (addresses network.SpaceAddresses) {
	for _, address := range m.doc.Addresses {
		addresses = append(addresses, address.networkAddress())
	}
	return
}

// MachineAddresses returns any hostnames and ips associated with a machine,
// determined by asking the machine itself.
func (m *Machine) MachineAddresses() (addresses network.SpaceAddresses) {
	for _, address := range m.doc.MachineAddresses {
		addresses = append(addresses, address.networkAddress())
	}
	return
}

// SetMachineAddresses records any addresses related to the machine, sourced
// by asking the machine.
func (m *Machine) SetMachineAddresses(addresses ...network.SpaceAddress) error {
	err := m.setAddresses(&addresses, nil)
	return errors.Annotatef(err, "cannot set machine addresses of machine %v", m)
}

// setAddresses updates the machine's addresses (either Addresses or
// MachineAddresses, depending on the field argument). Changes are
// only predicated on the machine not being Dead; concurrent address
// changes are ignored.
func (m *Machine) setAddresses(machineAddresses, providerAddresses *[]network.SpaceAddress) error {
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
		if isController(&m.doc) {
			if err := m.st.maybeUpdateControllerCharm(m.doc.PreferredPublicAddress.Value); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (st *State) maybeUpdateControllerCharm(publicAddr string) error {
	controllerApp, err := st.Application(bootstrap.ControllerApplicationName)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	return controllerApp.UpdateCharmConfig(model.GenerationMaster, charm.Settings{
		"controller-url": api.ControllerAPIURL(publicAddr, controllerCfg.APIPort()),
	})
}

func (m *Machine) setAddressesOps(
	machineAddresses, providerAddresses *[]network.SpaceAddress,
) (_ []txn.Op, machineStateAddresses, providerStateAddresses []address, newPrivate, newPublic *address, _ error) {

	if m.doc.Life == Dead {
		return nil, nil, nil, nil, nil, stateerrors.ErrDead
	}

	fromNetwork := func(in network.SpaceAddresses, origin network.Origin) []address {
		sorted := make(network.SpaceAddresses, len(in))
		copy(sorted, in)
		sort.Sort(sorted)
		return fromNetworkAddresses(sorted, origin)
	}

	var set bson.D
	machineStateAddresses = m.doc.MachineAddresses
	providerStateAddresses = m.doc.Addresses
	if machineAddresses != nil {
		machineStateAddresses = fromNetwork(*machineAddresses, network.OriginMachine)
		set = append(set, bson.DocElem{Name: "machineaddresses", Value: machineStateAddresses})
	}
	if providerAddresses != nil {
		providerStateAddresses = fromNetwork(*providerAddresses, network.OriginProvider)
		set = append(set, bson.DocElem{Name: "addresses", Value: providerStateAddresses})
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
		clock:     m.st.clock(),
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
	if m.doc.SupportedContainersKnown {
		if len(m.doc.SupportedContainers) == len(supportedContainers) {
			equal := true
			types := make(map[instance.ContainerType]struct{}, len(m.doc.SupportedContainers))
			for _, v := range m.doc.SupportedContainers {
				types[v] = struct{}{}
			}
			for _, v := range supportedContainers {
				if _, ok := types[v]; !ok {
					equal = false
					break
				}
			}
			if equal {
				return nil
			}
		}
	}
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
		err = onAbort(err, stateerrors.ErrDead)
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
		if !isSupportedContainer(corecontainer.ContainerTypeFromId(containerId), m.doc.SupportedContainers) {
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
				containerType := corecontainer.ContainerTypeFromId(containerId)
				now := m.st.clock().Now()
				s := status.StatusInfo{
					Status:  status.Error,
					Message: "unsupported container",
					Data:    map[string]interface{}{"type": containerType},
					Since:   &now,
				}
				_ = container.SetStatus(s)
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

// PrepareActionPayload returns the payload to use in creating an action for this machine.
// Note that the use of spec.InsertDefaults mutates payload.
func (m *Machine) PrepareActionPayload(name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (map[string]interface{}, bool, string, error) {
	if len(name) == 0 {
		return nil, false, "", errors.New("no action name given")
	}

	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		return nil, false, "", errors.Errorf("cannot add action %q to a machine; only predefined actions allowed", name)
	}

	// Reject bad payloads before attempting to insert defaults.
	err := spec.ValidateParams(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}
	payloadWithDefaults, err := spec.InsertDefaults(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}

	runParallel := spec.Parallel
	if parallel != nil {
		runParallel = *parallel
	}
	runExecutionGroup := spec.ExecutionGroup
	if executionGroup != nil {
		runExecutionGroup = *executionGroup
	}

	return payloadWithDefaults, runParallel, runExecutionGroup, nil
}

// CancelAction is part of the ActionReceiver interface.
func (m *Machine) CancelAction(action Action) (Action, error) {
	return action.Finish(ActionResults{Status: ActionCancelled})
}

// WatchActionNotifications is part of the ActionReceiver interface.
func (m *Machine) WatchActionNotifications() StringsWatcher {
	return m.st.watchActionNotificationsFilteredBy(m)
}

// WatchPendingActionNotifications is part of the ActionReceiver interface.
func (m *Machine) WatchPendingActionNotifications() StringsWatcher {
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

// UpdateMachineSeries updates the base for the Machine.
func (m *Machine) UpdateMachineSeries(base Base) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		// Exit early if the Machine base doesn't need to change.
		if m.Base().String() == base.String() {
			return nil, jujutxn.ErrNoOperations
		}

		units, err := m.Units()
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      machinesC,
			Id:     m.doc.DocID,
			Assert: bson.D{{"life", Alive}, {"principals", m.Principals()}},
			Update: bson.D{{"$set", bson.D{{"base", base}}}},
		}}
		for _, unit := range units {
			ops = append(ops, txn.Op{
				C:  unitsC,
				Id: unit.doc.DocID,
				Assert: bson.D{{"life", Alive},
					{"charmurl", unit.CharmURL()},
					{"subordinates", unit.SubordinateNames()}},
				Update: bson.D{{"$set",
					bson.D{{"base", base}}}},
			})
		}

		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	return errors.Annotatef(err, "updating series for machine %q", m)
}

// RecordAgentStartInformation updates the host name (if non-empty) reported
// by the machine agent and sets the agent start time to the current time.
func (m *Machine) RecordAgentStartInformation(hostname string) error {
	now := m.st.clock().Now()
	update := bson.D{
		{"agent-started-at", now},
	}

	if hostname != "" {
		update = append(update, bson.DocElem{"hostname", hostname})
	}

	ops := []txn.Op{{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", update}},
	}}

	if err := m.st.db().RunTransaction(ops); err != nil {
		// If instance doc doesn't exist, that's ok; there's nothing to keep,
		// but that's not an error we care about.
		return errors.Annotatef(onAbort(err, nil), "cannot update agent hostname/start time for machine %q", m)
	}
	m.doc.AgentStartedAt = now
	if hostname != "" {
		m.doc.Hostname = hostname
	}
	return nil
}

// AgentStartTime returns the last recorded timestamp when the machine agent
// was started.
func (m *Machine) AgentStartTime() time.Time {
	return m.doc.AgentStartedAt
}

// Hostname returns the hostname reported by the machine agent.
func (m *Machine) Hostname() string {
	return m.doc.Hostname
}

// AssertAliveOp returns an assert-only transaction operation
// that ensures the machine is alive.
func (m *Machine) AssertAliveOp() txn.Op {
	return txn.Op{
		C:      machinesC,
		Id:     m.doc.DocID,
		Assert: isAliveDoc,
	}
}

// assertMachineNotDeadOp returns an assert-only transaction operation that
// ensures the machine is not dead.
func assertMachineNotDeadOp(st *State, machineID string) txn.Op {
	return txn.Op{
		C:      machinesC,
		Id:     st.docID(machineID),
		Assert: notDeadDoc,
	}
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
	MachineAddresses  *[]network.SpaceAddress
	ProviderAddresses *[]network.SpaceAddress
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

func (st *State) GetManualMachineArches() (set.Strings, error) {
	instanceDataCollection, closer := st.db().GetCollection(instanceDataC)
	defer closer()

	var archDocs []struct {
		Arch string `bson:"arch"`
	}

	err := instanceDataCollection.Find(bson.M{
		"instanceid": bson.M{
			"$regex": "^" + manualMachinePrefix,
		},
	}).Select(bson.M{"arch": 1}).All(&archDocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get the set of architectures for manual machines: %v", err)
	}

	archSet := set.NewStrings()
	for _, archDoc := range archDocs {
		archSet.Add(archDoc.Arch)
	}
	return archSet, nil
}
