// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"
)

// ModelArgs represent the bare minimum information that is needed
// to represent a model.
type ModelArgs struct {
	Owner              names.UserTag
	Config             map[string]interface{}
	LatestToolsVersion version.Number
	Blocks             map[string]string
	Cloud              string
	CloudRegion        string
	CloudCredential    string
}

// NewModel returns a Model based on the args specified.
func NewModel(args ModelArgs) Model {
	m := &model{
		Version:             1,
		Owner_:              args.Owner.Id(),
		Config_:             args.Config,
		LatestToolsVersion_: args.LatestToolsVersion,
		Sequences_:          make(map[string]int),
		Blocks_:             args.Blocks,
		Cloud_:              args.Cloud,
		CloudRegion_:        args.CloudRegion,
		CloudCredential_:    args.CloudCredential,
	}
	m.setUsers(nil)
	m.setMachines(nil)
	m.setApplications(nil)
	m.setRelations(nil)
	m.setSpaces(nil)
	m.setLinkLayerDevices(nil)
	m.setSubnets(nil)
	m.setIPAddresses(nil)
	m.setSSHHostKeys(nil)
	m.setCloudImageMetadatas(nil)
	m.setActions(nil)
	m.setVolumes(nil)
	m.setFilesystems(nil)
	m.setStorages(nil)
	m.setStoragePools(nil)
	return m
}

// Serialize mirrors the Deserialize method, and makes sure that
// the same serialization method is used.
func Serialize(model Model) ([]byte, error) {
	return yaml.Marshal(model)
}

// Deserialize constructs a Model from a serialized YAML byte stream. The
// normal use for this is to construct the Model representation after getting
// the byte stream from an API connection or read from a file.
func Deserialize(bytes []byte) (Model, error) {
	var source map[string]interface{}
	err := yaml.Unmarshal(bytes, &source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := importModel(source)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// parseLinkLayerDeviceGlobalKey is used to validate that the parent device
// referenced by a LinkLayerDevice exists. Copied from state to avoid exporting
// and will be replaced by device.ParentMachineID() at some point.
func parseLinkLayerDeviceGlobalKey(globalKey string) (machineID, deviceName string, canBeGlobalKey bool) {
	if !strings.Contains(globalKey, "#") {
		// Can't be a global key.
		return "", "", false
	}
	keyParts := strings.Split(globalKey, "#")
	if len(keyParts) != 4 || (keyParts[0] != "m" && keyParts[2] != "d") {
		// Invalid global key format.
		return "", "", true
	}
	machineID, deviceName = keyParts[1], keyParts[3]
	return machineID, deviceName, true
}

// parentId returns the id of the host machine if machineId a container id, or ""
// if machineId is not for a container.
func parentId(machineId string) string {
	idParts := strings.Split(machineId, "/")
	if len(idParts) < 3 {
		return ""
	}
	return strings.Join(idParts[:len(idParts)-2], "/")
}

type model struct {
	Version int `yaml:"version"`

	Owner_  string                 `yaml:"owner"`
	Config_ map[string]interface{} `yaml:"config"`
	Blocks_ map[string]string      `yaml:"blocks,omitempty"`

	LatestToolsVersion_ version.Number `yaml:"latest-tools,omitempty"`

	Users_            users            `yaml:"users"`
	Machines_         machines         `yaml:"machines"`
	Applications_     applications     `yaml:"applications"`
	Relations_        relations        `yaml:"relations"`
	Spaces_           spaces           `yaml:"spaces"`
	LinkLayerDevices_ linklayerdevices `yaml:"link-layer-devices"`
	IPAddresses_      ipaddresses      `yaml:"ip-addresses"`
	Subnets_          subnets          `yaml:"subnets"`

	CloudImageMetadata_ cloudimagemetadataset `yaml:"cloud-image-metadata"`

	Actions_ actions `yaml:"actions"`

	SSHHostKeys_ sshHostKeys `yaml:"ssh-host-keys"`

	Sequences_ map[string]int `yaml:"sequences"`

	Annotations_ `yaml:"annotations,omitempty"`

	Constraints_ *constraints `yaml:"constraints,omitempty"`

	Cloud_           string `yaml:"cloud"`
	CloudRegion_     string `yaml:"cloud-region,omitempty"`
	CloudCredential_ string `yaml:"cloud-credential,omitempty"`

	Volumes_      volumes      `yaml:"volumes"`
	Filesystems_  filesystems  `yaml:"filesystems"`
	Storages_     storages     `yaml:"storages"`
	StoragePools_ storagepools `yaml:"storage-pools"`
}

func (m *model) Tag() names.ModelTag {
	// Here we make the assumption that the environment UUID is set
	// correctly in the Config.
	value := m.Config_["uuid"]
	// Explicitly ignore the 'ok' aspect of the cast. If we don't have it
	// and it is wrong, we panic. Here we fully expect it to exist, but
	// paranoia says 'never panic', so worst case is we have an empty string.
	uuid, _ := value.(string)
	return names.NewModelTag(uuid)
}

// Owner implements Model.
func (m *model) Owner() names.UserTag {
	return names.NewUserTag(m.Owner_)
}

// Config implements Model.
func (m *model) Config() map[string]interface{} {
	// TODO: consider returning a deep copy.
	return m.Config_
}

// UpdateConfig implements Model.
func (m *model) UpdateConfig(config map[string]interface{}) {
	for key, value := range config {
		m.Config_[key] = value
	}
}

// LatestToolsVersion implements Model.
func (m *model) LatestToolsVersion() version.Number {
	return m.LatestToolsVersion_
}

// Blocks implements Model.
func (m *model) Blocks() map[string]string {
	return m.Blocks_
}

// Implement length-based sort with ByLen type.
type ByName []User

func (a ByName) Len() int           { return len(a) }
func (a ByName) Less(i, j int) bool { return a[i].Name().Canonical() < a[j].Name().Canonical() }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// Users implements Model.
func (m *model) Users() []User {
	var result []User
	for _, user := range m.Users_.Users_ {
		result = append(result, user)
	}
	sort.Sort(ByName(result))
	return result
}

// AddUser implements Model.
func (m *model) AddUser(args UserArgs) {
	m.Users_.Users_ = append(m.Users_.Users_, newUser(args))
}

func (m *model) setUsers(userList []*user) {
	m.Users_ = users{
		Version: 1,
		Users_:  userList,
	}
}

// Machines implements Model.
func (m *model) Machines() []Machine {
	var result []Machine
	for _, machine := range m.Machines_.Machines_ {
		result = append(result, machine)
	}
	return result
}

// AddMachine implements Model.
func (m *model) AddMachine(args MachineArgs) Machine {
	machine := newMachine(args)
	m.Machines_.Machines_ = append(m.Machines_.Machines_, machine)
	return machine
}

func (m *model) setMachines(machineList []*machine) {
	m.Machines_ = machines{
		Version:   1,
		Machines_: machineList,
	}
}

// Applications implements Model.
func (m *model) Applications() []Application {
	var result []Application
	for _, application := range m.Applications_.Applications_ {
		result = append(result, application)
	}
	return result
}

func (m *model) application(name string) *application {
	for _, application := range m.Applications_.Applications_ {
		if application.Name() == name {
			return application
		}
	}
	return nil
}

// AddApplication implements Model.
func (m *model) AddApplication(args ApplicationArgs) Application {
	application := newApplication(args)
	m.Applications_.Applications_ = append(m.Applications_.Applications_, application)
	return application
}

func (m *model) setApplications(applicationList []*application) {
	m.Applications_ = applications{
		Version:       1,
		Applications_: applicationList,
	}
}

// Relations implements Model.
func (m *model) Relations() []Relation {
	var result []Relation
	for _, relation := range m.Relations_.Relations_ {
		result = append(result, relation)
	}
	return result
}

// AddRelation implements Model.
func (m *model) AddRelation(args RelationArgs) Relation {
	relation := newRelation(args)
	m.Relations_.Relations_ = append(m.Relations_.Relations_, relation)
	return relation
}

func (m *model) setRelations(relationList []*relation) {
	m.Relations_ = relations{
		Version:    1,
		Relations_: relationList,
	}
}

// Spaces implements Model.
func (m *model) Spaces() []Space {
	var result []Space
	for _, space := range m.Spaces_.Spaces_ {
		result = append(result, space)
	}
	return result
}

// AddSpace implements Model.
func (m *model) AddSpace(args SpaceArgs) Space {
	space := newSpace(args)
	m.Spaces_.Spaces_ = append(m.Spaces_.Spaces_, space)
	return space
}

func (m *model) setSpaces(spaceList []*space) {
	m.Spaces_ = spaces{
		Version: 1,
		Spaces_: spaceList,
	}
}

// LinkLayerDevices implements Model.
func (m *model) LinkLayerDevices() []LinkLayerDevice {
	var result []LinkLayerDevice
	for _, device := range m.LinkLayerDevices_.LinkLayerDevices_ {
		result = append(result, device)
	}
	return result
}

// AddLinkLayerDevice implements Model.
func (m *model) AddLinkLayerDevice(args LinkLayerDeviceArgs) LinkLayerDevice {
	device := newLinkLayerDevice(args)
	m.LinkLayerDevices_.LinkLayerDevices_ = append(m.LinkLayerDevices_.LinkLayerDevices_, device)
	return device
}

func (m *model) setLinkLayerDevices(devicesList []*linklayerdevice) {
	m.LinkLayerDevices_ = linklayerdevices{
		Version:           1,
		LinkLayerDevices_: devicesList,
	}
}

// Subnets implements Model.
func (m *model) Subnets() []Subnet {
	var result []Subnet
	for _, subnet := range m.Subnets_.Subnets_ {
		result = append(result, subnet)
	}
	return result
}

// AddSubnet implemets Model.
func (m *model) AddSubnet(args SubnetArgs) Subnet {
	subnet := newSubnet(args)
	m.Subnets_.Subnets_ = append(m.Subnets_.Subnets_, subnet)
	return subnet
}

func (m *model) setSubnets(subnetList []*subnet) {
	m.Subnets_ = subnets{
		Version:  1,
		Subnets_: subnetList,
	}
}

// IPAddresses implements Model.
func (m *model) IPAddresses() []IPAddress {
	var result []IPAddress
	for _, addr := range m.IPAddresses_.IPAddresses_ {
		result = append(result, addr)
	}
	return result
}

// AddIPAddress implements Model.
func (m *model) AddIPAddress(args IPAddressArgs) IPAddress {
	addr := newIPAddress(args)
	m.IPAddresses_.IPAddresses_ = append(m.IPAddresses_.IPAddresses_, addr)
	return addr
}

func (m *model) setIPAddresses(addressesList []*ipaddress) {
	m.IPAddresses_ = ipaddresses{
		Version:      1,
		IPAddresses_: addressesList,
	}
}

// SSHHostKeys implements Model.
func (m *model) SSHHostKeys() []SSHHostKey {
	var result []SSHHostKey
	for _, addr := range m.SSHHostKeys_.SSHHostKeys_ {
		result = append(result, addr)
	}
	return result
}

// AddSSHHostKey implements Model.
func (m *model) AddSSHHostKey(args SSHHostKeyArgs) SSHHostKey {
	addr := newSSHHostKey(args)
	m.SSHHostKeys_.SSHHostKeys_ = append(m.SSHHostKeys_.SSHHostKeys_, addr)
	return addr
}

func (m *model) setSSHHostKeys(addressesList []*sshHostKey) {
	m.SSHHostKeys_ = sshHostKeys{
		Version:      1,
		SSHHostKeys_: addressesList,
	}
}

// CloudImageMetadatas implements Model.
func (m *model) CloudImageMetadata() []CloudImageMetadata {
	var result []CloudImageMetadata
	for _, addr := range m.CloudImageMetadata_.CloudImageMetadata_ {
		result = append(result, addr)
	}
	return result
}

// Actions implements Model.
func (m *model) Actions() []Action {
	var result []Action
	for _, addr := range m.Actions_.Actions_ {
		result = append(result, addr)
	}
	return result
}

// AddCloudImageMetadata implements Model.
func (m *model) AddCloudImageMetadata(args CloudImageMetadataArgs) CloudImageMetadata {
	addr := newCloudImageMetadata(args)
	m.CloudImageMetadata_.CloudImageMetadata_ = append(m.CloudImageMetadata_.CloudImageMetadata_, addr)
	return addr
}

func (m *model) setCloudImageMetadatas(cloudimagemetadataList []*cloudimagemetadata) {
	m.CloudImageMetadata_ = cloudimagemetadataset{
		Version:             1,
		CloudImageMetadata_: cloudimagemetadataList,
	}
}

// AddAction implements Model.
func (m *model) AddAction(args ActionArgs) Action {
	addr := newAction(args)
	m.Actions_.Actions_ = append(m.Actions_.Actions_, addr)
	return addr
}

func (m *model) setActions(actionsList []*action) {
	m.Actions_ = actions{
		Version:  1,
		Actions_: actionsList,
	}
}

// Sequences implements Model.
func (m *model) Sequences() map[string]int {
	return m.Sequences_
}

// SetSequence implements Model.
func (m *model) SetSequence(name string, value int) {
	m.Sequences_[name] = value
}

// Constraints implements HasConstraints.
func (m *model) Constraints() Constraints {
	if m.Constraints_ == nil {
		return nil
	}
	return m.Constraints_
}

// SetConstraints implements HasConstraints.
func (m *model) SetConstraints(args ConstraintsArgs) {
	m.Constraints_ = newConstraints(args)
}

// Cloud implements Model.
func (m *model) Cloud() string {
	return m.Cloud_
}

// CloudRegion implements Model.
func (m *model) CloudRegion() string {
	return m.CloudRegion_
}

// CloudCredential implements Model.
func (m *model) CloudCredential() string {
	return m.CloudCredential_
}

// Volumes implements Model.
func (m *model) Volumes() []Volume {
	var result []Volume
	for _, volume := range m.Volumes_.Volumes_ {
		result = append(result, volume)
	}
	return result
}

// AddVolume implemets Model.
func (m *model) AddVolume(args VolumeArgs) Volume {
	volume := newVolume(args)
	m.Volumes_.Volumes_ = append(m.Volumes_.Volumes_, volume)
	return volume
}

func (m *model) setVolumes(volumeList []*volume) {
	m.Volumes_ = volumes{
		Version:  1,
		Volumes_: volumeList,
	}
}

// Filesystems implements Model.
func (m *model) Filesystems() []Filesystem {
	var result []Filesystem
	for _, filesystem := range m.Filesystems_.Filesystems_ {
		result = append(result, filesystem)
	}
	return result
}

// AddFilesystem implemets Model.
func (m *model) AddFilesystem(args FilesystemArgs) Filesystem {
	filesystem := newFilesystem(args)
	m.Filesystems_.Filesystems_ = append(m.Filesystems_.Filesystems_, filesystem)
	return filesystem
}

func (m *model) setFilesystems(filesystemList []*filesystem) {
	m.Filesystems_ = filesystems{
		Version:      1,
		Filesystems_: filesystemList,
	}
}

// Storages implements Model.
func (m *model) Storages() []Storage {
	var result []Storage
	for _, storage := range m.Storages_.Storages_ {
		result = append(result, storage)
	}
	return result
}

// AddStorage implemets Model.
func (m *model) AddStorage(args StorageArgs) Storage {
	storage := newStorage(args)
	m.Storages_.Storages_ = append(m.Storages_.Storages_, storage)
	return storage
}

func (m *model) setStorages(storageList []*storage) {
	m.Storages_ = storages{
		Version:   1,
		Storages_: storageList,
	}
}

// StoragePools implements Model.
func (m *model) StoragePools() []StoragePool {
	var result []StoragePool
	for _, pool := range m.StoragePools_.Pools_ {
		result = append(result, pool)
	}
	return result
}

// AddStoragePool implemets Model.
func (m *model) AddStoragePool(args StoragePoolArgs) StoragePool {
	pool := newStoragePool(args)
	m.StoragePools_.Pools_ = append(m.StoragePools_.Pools_, pool)
	return pool
}

func (m *model) setStoragePools(poolList []*storagepool) {
	m.StoragePools_ = storagepools{
		Version: 1,
		Pools_:  poolList,
	}
}

// Validate implements Model.
func (m *model) Validate() error {
	// A model needs an owner.
	if m.Owner_ == "" {
		return errors.NotValidf("missing model owner")
	}
	allMachines := set.NewStrings()
	unitsWithOpenPorts := set.NewStrings()
	for _, machine := range m.Machines_.Machines_ {
		if err := m.validateMachine(machine, allMachines, unitsWithOpenPorts); err != nil {
			return errors.Trace(err)
		}
	}
	allApplications := set.NewStrings()
	allUnits := set.NewStrings()
	for _, application := range m.Applications_.Applications_ {
		if err := application.Validate(); err != nil {
			return errors.Trace(err)
		}
		allApplications.Add(application.Name())
		allUnits = allUnits.Union(application.unitNames())
	}
	// Make sure that all the unit names specified in machine opened ports
	// exist as units of applications.
	unknownUnitsWithPorts := unitsWithOpenPorts.Difference(allUnits)
	if len(unknownUnitsWithPorts) > 0 {
		return errors.Errorf("unknown unit names in open ports: %s", unknownUnitsWithPorts.SortedValues())
	}

	err := m.validateRelations()
	if err != nil {
		return errors.Trace(err)
	}

	err = m.validateSubnets()
	if err != nil {
		return errors.Trace(err)
	}

	err = m.validateLinkLayerDevices()
	if err != nil {
		return errors.Trace(err)
	}
	err = m.validateAddresses()
	if err != nil {
		return errors.Trace(err)
	}

	err = m.validateStorage(allMachines, allApplications, allUnits)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (m *model) validateMachine(machine Machine, allMachineIDs, unitsWithOpenPorts set.Strings) error {
	if err := machine.Validate(); err != nil {
		return errors.Trace(err)
	}
	allMachineIDs.Add(machine.Id())
	for _, op := range machine.OpenedPorts() {
		for _, pr := range op.OpenPorts() {
			unitsWithOpenPorts.Add(pr.UnitName())
		}
	}
	for _, container := range machine.Containers() {
		err := m.validateMachine(container, allMachineIDs, unitsWithOpenPorts)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m *model) validateStorage(allMachineIDs, allApplications, allUnits set.Strings) error {
	appsAndUnits := allApplications.Union(allUnits)
	allStorage := set.NewStrings()
	for i, storage := range m.Storages_.Storages_ {
		if err := storage.Validate(); err != nil {
			return errors.Annotatef(err, "storage[%d]", i)
		}
		allStorage.Add(storage.Tag().Id())
		owner, err := storage.Owner()
		if err != nil {
			return errors.Wrap(err, errors.NotValidf("storage[%d] owner (%s)", i, owner))
		}
		ownerID := owner.Id()
		if !appsAndUnits.Contains(ownerID) {
			return errors.NotValidf("storage[%d] owner (%s)", i, ownerID)
		}
		for _, unit := range storage.Attachments() {
			if !allUnits.Contains(unit.Id()) {
				return errors.NotValidf("storage[%d] attachment referencing unknown unit %q", i, unit)
			}
		}
	}
	allVolumes := set.NewStrings()
	for i, volume := range m.Volumes_.Volumes_ {
		if err := volume.Validate(); err != nil {
			return errors.Annotatef(err, "volume[%d]", i)
		}
		allVolumes.Add(volume.Tag().Id())
		if storeID := volume.Storage().Id(); storeID != "" && !allStorage.Contains(storeID) {
			return errors.NotValidf("volume[%d] referencing unknown storage %q", i, storeID)
		}
		for j, attachment := range volume.Attachments() {
			if machineID := attachment.Machine().Id(); !allMachineIDs.Contains(machineID) {
				return errors.NotValidf("volume[%d].attachment[%d] referencing unknown machine %q", i, j, machineID)
			}
		}
	}
	for i, filesystem := range m.Filesystems_.Filesystems_ {
		if err := filesystem.Validate(); err != nil {
			return errors.Annotatef(err, "filesystem[%d]", i)
		}
		if storeID := filesystem.Storage().Id(); storeID != "" && !allStorage.Contains(storeID) {
			return errors.NotValidf("filesystem[%d] referencing unknown storage %q", i, storeID)
		}
		if volID := filesystem.Volume().Id(); volID != "" && !allVolumes.Contains(volID) {
			return errors.NotValidf("filesystem[%d] referencing unknown volume %q", i, volID)
		}
		for j, attachment := range filesystem.Attachments() {
			if machineID := attachment.Machine().Id(); !allMachineIDs.Contains(machineID) {
				return errors.NotValidf("filesystem[%d].attachment[%d] referencing unknown machine %q", i, j, machineID)
			}
		}
	}

	return nil
}

// validateSubnets makes sure that any spaces referenced by subnets exist.
func (m *model) validateSubnets() error {
	spaceNames := set.NewStrings()
	for _, space := range m.Spaces_.Spaces_ {
		spaceNames.Add(space.Name())
	}
	for _, subnet := range m.Subnets_.Subnets_ {
		if subnet.SpaceName() == "" {
			continue
		}
		if !spaceNames.Contains(subnet.SpaceName()) {
			return errors.Errorf("subnet %q references non-existent space %q", subnet.CIDR(), subnet.SpaceName())
		}
	}

	return nil
}

func (m *model) machineMaps() (map[string]Machine, map[string]map[string]LinkLayerDevice) {
	machineIDs := make(map[string]Machine)
	for _, machine := range m.Machines_.Machines_ {
		addMachinesToMap(machine, machineIDs)
	}

	// Build a map of all devices for each machine.
	machineDevices := make(map[string]map[string]LinkLayerDevice)
	for _, device := range m.LinkLayerDevices_.LinkLayerDevices_ {
		_, ok := machineDevices[device.MachineID()]
		if !ok {
			machineDevices[device.MachineID()] = make(map[string]LinkLayerDevice)
		}
		machineDevices[device.MachineID()][device.Name()] = device
	}
	return machineIDs, machineDevices
}

func addMachinesToMap(machine Machine, machineIDs map[string]Machine) {
	machineIDs[machine.Id()] = machine
	for _, container := range machine.Containers() {
		addMachinesToMap(container, machineIDs)
	}
}

// validateAddresses makes sure that the machine and device referenced by IP
// addresses exist.
func (m *model) validateAddresses() error {
	machineIDs, machineDevices := m.machineMaps()
	for _, addr := range m.IPAddresses_.IPAddresses_ {
		_, ok := machineIDs[addr.MachineID()]
		if !ok {
			return errors.Errorf("ip address %q references non-existent machine %q", addr.Value(), addr.MachineID())
		}
		_, ok = machineDevices[addr.MachineID()][addr.DeviceName()]
		if !ok {
			return errors.Errorf("ip address %q references non-existent device %q", addr.Value(), addr.DeviceName())
		}
		if ip := net.ParseIP(addr.Value()); ip == nil {
			return errors.Errorf("ip address has invalid value %q", addr.Value())
		}
		if addr.SubnetCIDR() == "" {
			return errors.Errorf("ip address %q has empty subnet CIDR", addr.Value())
		}
		if _, _, err := net.ParseCIDR(addr.SubnetCIDR()); err != nil {
			return errors.Errorf("ip address %q has invalid subnet CIDR %q", addr.Value(), addr.SubnetCIDR())
		}

		if addr.GatewayAddress() != "" {
			if ip := net.ParseIP(addr.GatewayAddress()); ip == nil {
				return errors.Errorf("ip address %q has invalid gateway address %q", addr.Value(), addr.GatewayAddress())
			}
		}
	}
	return nil
}

// validateLinkLayerDevices makes sure that any machines referenced by link
// layer devices exist.
func (m *model) validateLinkLayerDevices() error {
	machineIDs, machineDevices := m.machineMaps()
	for _, device := range m.LinkLayerDevices_.LinkLayerDevices_ {
		machine, ok := machineIDs[device.MachineID()]
		if !ok {
			return errors.Errorf("device %q references non-existent machine %q", device.Name(), device.MachineID())
		}
		if device.Name() == "" {
			return errors.Errorf("device has empty name: %#v", device)
		}
		if device.MACAddress() != "" {
			if _, err := net.ParseMAC(device.MACAddress()); err != nil {
				return errors.Errorf("device %q has invalid MACAddress %q", device.Name(), device.MACAddress())
			}
		}
		if device.ParentName() == "" {
			continue
		}
		hostMachineID, parentDeviceName, canBeGlobalKey := parseLinkLayerDeviceGlobalKey(device.ParentName())
		if !canBeGlobalKey {
			hostMachineID = device.MachineID()
			parentDeviceName = device.ParentName()
		}
		parentDevice, ok := machineDevices[hostMachineID][parentDeviceName]
		if !ok {
			return errors.Errorf("device %q has non-existent parent %q", device.Name(), parentDeviceName)
		}
		if !canBeGlobalKey {
			if device.Name() == parentDeviceName {
				return errors.Errorf("device %q is its own parent", device.Name())
			}
			continue
		}
		// The device is on a container.
		if parentDevice.Type() != "bridge" {
			return errors.Errorf("device %q on a container but not a bridge", device.Name())
		}
		parentId := parentId(machine.Id())
		if parentId == "" {
			return errors.Errorf("ParentName %q for non-container machine %q", device.ParentName(), machine.Id())
		}
		if parentDevice.MachineID() != parentId {
			return errors.Errorf("parent machine of device %q not host machine %q", device.Name(), parentId)
		}
	}
	return nil
}

// validateRelations makes sure that for each endpoint in each relation there
// are settings for all units of that application for that endpoint.
func (m *model) validateRelations() error {
	for _, relation := range m.Relations_.Relations_ {
		for _, ep := range relation.Endpoints_.Endpoints_ {
			// Check application exists.
			application := m.application(ep.ApplicationName())
			if application == nil {
				return errors.Errorf("unknown application %q for relation id %d", ep.ApplicationName(), relation.Id())
			}
			// Check that all units have settings.
			applicationUnits := application.unitNames()
			epUnits := ep.unitNames()
			if missingSettings := applicationUnits.Difference(epUnits); len(missingSettings) > 0 {
				return errors.Errorf("missing relation settings for units %s in relation %d", missingSettings.SortedValues(), relation.Id())
			}
			if extraSettings := epUnits.Difference(applicationUnits); len(extraSettings) > 0 {
				return errors.Errorf("settings for unknown units %s in relation %d", extraSettings.SortedValues(), relation.Id())
			}
		}
	}
	return nil
}

// importModel constructs a new Model from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
//
// This method is a package internal serialisation method.
func importModel(source map[string]interface{}) (*model, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Trace(err)
	}

	importFunc, ok := modelDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type modelDeserializationFunc func(map[string]interface{}) (*model, error)

var modelDeserializationFuncs = map[int]modelDeserializationFunc{
	1: importModelV1,
}

func importModelV1(source map[string]interface{}) (*model, error) {
	fields := schema.Fields{
		"owner":                schema.String(),
		"cloud":                schema.String(),
		"cloud-region":         schema.String(),
		"config":               schema.StringMap(schema.Any()),
		"latest-tools":         schema.String(),
		"blocks":               schema.StringMap(schema.String()),
		"users":                schema.StringMap(schema.Any()),
		"machines":             schema.StringMap(schema.Any()),
		"applications":         schema.StringMap(schema.Any()),
		"relations":            schema.StringMap(schema.Any()),
		"ssh-host-keys":        schema.StringMap(schema.Any()),
		"cloud-image-metadata": schema.StringMap(schema.Any()),
		"actions":              schema.StringMap(schema.Any()),
		"ip-addresses":         schema.StringMap(schema.Any()),
		"spaces":               schema.StringMap(schema.Any()),
		"subnets":              schema.StringMap(schema.Any()),
		"link-layer-devices":   schema.StringMap(schema.Any()),
		"volumes":              schema.StringMap(schema.Any()),
		"filesystems":          schema.StringMap(schema.Any()),
		"storages":             schema.StringMap(schema.Any()),
		"storage-pools":        schema.StringMap(schema.Any()),
		"sequences":            schema.StringMap(schema.Int()),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"latest-tools": schema.Omit,
		"blocks":       schema.Omit,
		"cloud-region": schema.Omit,
	}
	addAnnotationSchema(fields, defaults)
	addConstraintsSchema(fields, defaults)
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "model v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &model{
		Version:    1,
		Owner_:     valid["owner"].(string),
		Config_:    valid["config"].(map[string]interface{}),
		Sequences_: make(map[string]int),
		Blocks_:    convertToStringMap(valid["blocks"]),
		Cloud_:     valid["cloud"].(string),
	}
	result.importAnnotations(valid)
	sequences := valid["sequences"].(map[string]interface{})
	for key, value := range sequences {
		result.SetSequence(key, int(value.(int64)))
	}

	if constraintsMap, ok := valid["constraints"]; ok {
		constraints, err := importConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Constraints_ = constraints
	}

	if availableTools, ok := valid["latest-tools"]; ok {
		num, err := version.Parse(availableTools.(string))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.LatestToolsVersion_ = num
	}

	if region, ok := valid["cloud-region"]; ok {
		result.CloudRegion_ = region.(string)
	}

	if credential, ok := valid["cloud-credential"]; ok {
		result.CloudCredential_ = credential.(string)
	}

	userMap := valid["users"].(map[string]interface{})
	users, err := importUsers(userMap)
	if err != nil {
		return nil, errors.Annotate(err, "users")
	}
	result.setUsers(users)

	machineMap := valid["machines"].(map[string]interface{})
	machines, err := importMachines(machineMap)
	if err != nil {
		return nil, errors.Annotate(err, "machines")
	}
	result.setMachines(machines)

	applicationMap := valid["applications"].(map[string]interface{})
	applications, err := importApplications(applicationMap)
	if err != nil {
		return nil, errors.Annotate(err, "applications")
	}
	result.setApplications(applications)

	relationMap := valid["relations"].(map[string]interface{})
	relations, err := importRelations(relationMap)
	if err != nil {
		return nil, errors.Annotate(err, "relations")
	}
	result.setRelations(relations)

	spaceMap := valid["spaces"].(map[string]interface{})
	spaces, err := importSpaces(spaceMap)
	if err != nil {
		return nil, errors.Annotate(err, "spaces")
	}
	result.setSpaces(spaces)

	deviceMap := valid["link-layer-devices"].(map[string]interface{})
	devices, err := importLinkLayerDevices(deviceMap)
	if err != nil {
		return nil, errors.Annotate(err, "link-layer-devices")
	}
	result.setLinkLayerDevices(devices)

	subnetsMap := valid["subnets"].(map[string]interface{})
	subnets, err := importSubnets(subnetsMap)
	if err != nil {
		return nil, errors.Annotate(err, "subnets")
	}
	result.setSubnets(subnets)

	addressMap := valid["ip-addresses"].(map[string]interface{})
	addresses, err := importIPAddresses(addressMap)
	if err != nil {
		return nil, errors.Annotate(err, "ip-addresses")
	}
	result.setIPAddresses(addresses)

	sshHostKeyMap := valid["ssh-host-keys"].(map[string]interface{})
	hostKeys, err := importSSHHostKeys(sshHostKeyMap)
	if err != nil {
		return nil, errors.Annotate(err, "ssh-host-keys")
	}
	result.setSSHHostKeys(hostKeys)

	cloudimagemetadataMap := valid["cloud-image-metadata"].(map[string]interface{})
	cloudimagemetadata, err := importCloudImageMetadata(cloudimagemetadataMap)
	if err != nil {
		return nil, errors.Annotate(err, "cloud-image-metadata")
	}
	result.setCloudImageMetadatas(cloudimagemetadata)

	actionsMap := valid["actions"].(map[string]interface{})
	actions, err := importActions(actionsMap)
	if err != nil {
		return nil, errors.Annotate(err, "actions")
	}
	result.setActions(actions)

	volumes, err := importVolumes(valid["volumes"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Annotate(err, "volumes")
	}
	result.setVolumes(volumes)

	filesystems, err := importFilesystems(valid["filesystems"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Annotate(err, "filesystems")
	}
	result.setFilesystems(filesystems)

	storages, err := importStorages(valid["storages"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Annotate(err, "storages")
	}
	result.setStorages(storages)

	pools, err := importStoragePools(valid["storage-pools"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Annotate(err, "storage-pools")
	}
	result.setStoragePools(pools)

	return result, nil
}
