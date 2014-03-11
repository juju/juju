// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/juju/loggo"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.provider.ec2")

// Use shortAttempt to poll for short-term events.
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var providerInstance environProvider

type environ struct {
	name string

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex       sync.Mutex
	ecfgUnlocked    *environConfig
	ec2Unlocked     *ec2.EC2
	s3Unlocked      *s3.S3
	storageUnlocked storage.Storage
}

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ imagemetadata.SupportsCustomSources = (*environ)(nil)
var _ envtools.SupportsCustomSources = (*environ)(nil)

type ec2Instance struct {
	e *environ

	mu sync.Mutex
	*ec2.Instance
}

func (inst *ec2Instance) String() string {
	return string(inst.Id())
}

var _ instance.Instance = (*ec2Instance)(nil)

func (inst *ec2Instance) getInstance() *ec2.Instance {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.Instance
}

func (inst *ec2Instance) Id() instance.Id {
	return instance.Id(inst.getInstance().InstanceId)
}

func (inst *ec2Instance) Status() string {
	return inst.getInstance().State.Name
}

// Refresh implements instance.Refresh(), requerying the
// Instance details over the ec2 api
func (inst *ec2Instance) Refresh() error {
	_, err := inst.refresh()
	return err
}

// refresh requeries Instance details over the ec2 api.
func (inst *ec2Instance) refresh() (*ec2.Instance, error) {
	id := inst.Id()
	insts, err := inst.e.Instances([]instance.Id{id})
	if err != nil {
		return nil, err
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Instance = insts[0].(*ec2Instance).Instance
	return inst.Instance, nil
}

// Addresses implements instance.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (inst *ec2Instance) Addresses() ([]instance.Address, error) {
	// TODO(gz): Stop relying on this requerying logic, maybe remove error
	instInstance := inst.getInstance()
	if instInstance.DNSName == "" {
		// Fetch the instance information again, in case
		// the DNS information has become available.
		var err error
		instInstance, err = inst.refresh()
		if err != nil {
			return nil, err
		}
	}
	var addresses []instance.Address
	possibleAddresses := []instance.Address{
		{
			Value:        instInstance.DNSName,
			Type:         instance.HostName,
			NetworkScope: instance.NetworkPublic,
		},
		{
			Value:        instInstance.PrivateDNSName,
			Type:         instance.HostName,
			NetworkScope: instance.NetworkCloudLocal,
		},
		{
			Value:        instInstance.IPAddress,
			Type:         instance.Ipv4Address,
			NetworkScope: instance.NetworkPublic,
		},
		{
			Value:        instInstance.PrivateIPAddress,
			Type:         instance.Ipv4Address,
			NetworkScope: instance.NetworkCloudLocal,
		},
	}
	for _, address := range possibleAddresses {
		if address.Value != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func (inst *ec2Instance) DNSName() (string, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return "", err
	}
	addr := instance.SelectPublicAddress(addresses)
	if addr == "" {
		return "", instance.ErrNoDNSName
	}
	return addr, nil

}

func (inst *ec2Instance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

func (p environProvider) BoilerplateConfig() string {
	return `
# https://juju.ubuntu.com/docs/config-aws.html
amazon:
    type: ec2
    # region specifies the ec2 region. It defaults to us-east-1.
    # region: us-east-1
    #
    # access-key holds the ec2 access key. It defaults to the environment
    # variable AWS_ACCESS_KEY_ID.
    # access-key: <secret>
    #
    # secret-key holds the ec2 secret key. It defaults to the environment
    # variable AWS_SECRET_ACCESS_KEY.
    #
    # image-stream chooses a simplestreams stream to select OS images from,
    # for example daily or released images (or any other stream available on simplestreams).
    # image-stream: "released"

`[1:]
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	e := new(environ)
	e.name = cfg.Name()
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (p environProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	attrs := cfg.UnknownAttrs()
	if _, ok := attrs["control-bucket"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		attrs["control-bucket"] = fmt.Sprintf("%x", uuid.Raw())
	}
	cfg, err := cfg.Apply(attrs)
	if err != nil {
		return nil, err
	}
	return p.Open(cfg)
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p environProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		fmt.Errorf("region must be specified")
	}
	ec2Region, ok := allRegions[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", region)
	}
	return &simplestreams.MetadataLookupParams{
		Region:        region,
		Endpoint:      ec2Region.EC2Endpoint,
		Architectures: []string{"amd64", "i386"},
	}, nil
}

func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	m := make(map[string]string)
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["access-key"] = ecfg.accessKey()
	m["secret-key"] = ecfg.secretKey()
	return m, nil
}

func (environProvider) PublicAddress() (string, error) {
	return fetchMetadata("public-hostname")
}

func (environProvider) PrivateAddress() (string, error) {
	return fetchMetadata("local-hostname")
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgUnlocked = ecfg

	auth := aws.Auth{ecfg.accessKey(), ecfg.secretKey()}
	region := aws.Regions[ecfg.region()]
	e.ec2Unlocked = ec2.New(auth, region)
	e.s3Unlocked = s3.New(auth, region)

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &ec2storage{
		bucket: e.s3Unlocked.Bucket(ecfg.controlBucket()),
	}
	return nil
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) ec2() *ec2.EC2 {
	e.ecfgMutex.Lock()
	ec2 := e.ec2Unlocked
	e.ecfgMutex.Unlock()
	return ec2
}

func (e *environ) s3() *s3.S3 {
	e.ecfgMutex.Lock()
	s3 := e.s3Unlocked
	e.ecfgMutex.Unlock()
	return s3
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Storage() storage.Storage {
	e.ecfgMutex.Lock()
	stor := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return stor
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	return common.Bootstrap(ctx, e, cons)
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(e)
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (e *environ) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = e.ecfg().region()
	}
	ec2Region, ok := allRegions[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", region)
	}
	return &simplestreams.MetadataLookupParams{
		Series:        e.ecfg().DefaultSeries(),
		Region:        region,
		Endpoint:      ec2Region.EC2Endpoint,
		Architectures: []string{"amd64", "i386", "arm", "arm64", "ppc64"},
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	region := e.ecfg().region()
	ec2Region, ok := allRegions[region]
	if !ok {
		return simplestreams.CloudSpec{}, fmt.Errorf("unknown region %q", region)
	}
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: ec2Region.EC2Endpoint,
	}, nil
}

const ebsStorage = "ebs"

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	arches := possibleTools.Arches()
	stor := ebsStorage
	sources, err := imagemetadata.GetMetadataSources(e)
	if err != nil {
		return nil, nil, err
	}

	series := possibleTools.OneSeries()
	spec, err := findInstanceSpec(sources, e.Config().ImageStream(), &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series,
		Arches:      arches,
		Constraints: cons,
		Storage:     &stor,
	})
	if err != nil {
		return nil, nil, err
	}
	tools, err := possibleTools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, nil, fmt.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	machineConfig.Tools = tools[0]
	if err := environs.FinishMachineConfig(machineConfig, e.Config(), cons); err != nil {
		return nil, nil, err
	}

	userData, err := environs.ComposeUserData(machineConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make user data: %v", err)
	}
	logger.Debugf("ec2 user data; %d bytes", len(userData))
	cfg := e.Config()
	groups, err := e.setUpGroups(machineConfig.MachineId, cfg.StatePort(), cfg.APIPort())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var instResp *ec2.RunInstancesResp

	device, diskSize := getDiskSize(cons)
	for a := shortAttempt.Start(); a.Next(); {
		instResp, err = e.ec2().RunInstances(&ec2.RunInstances{
			ImageId:             spec.Image.Id,
			MinCount:            1,
			MaxCount:            1,
			UserData:            userData,
			InstanceType:        spec.InstanceType.Name,
			SecurityGroups:      groups,
			BlockDeviceMappings: []ec2.BlockDeviceMapping{device},
		})
		if err == nil || ec2ErrCode(err) != "InvalidGroup.NotFound" {
			break
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instResp.Instances) != 1 {
		return nil, nil, fmt.Errorf("expected 1 started instance, got %d", len(instResp.Instances))
	}

	inst := &ec2Instance{
		e:        e,
		Instance: &instResp.Instances[0],
	}
	logger.Infof("started instance %q", inst.Id())

	hc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &diskSize,
		// Tags currently not supported by EC2
	}
	return inst, &hc, nil
}

func (e *environ) StopInstances(insts []instance.Instance) error {
	ids := make([]instance.Id, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*ec2Instance).Id()
	}
	return e.terminateInstances(ids)
}

// minDiskSize is the minimum/default size (in megabytes) for ec2 root disks.
const minDiskSize uint64 = 8 * 1024

// getDiskSize translates a RootDisk constraint (or lackthereof) into a
// BlockDeviceMapping request for EC2.  megs is the size in megabytes of
// the disk that was requested.
func getDiskSize(cons constraints.Value) (dvc ec2.BlockDeviceMapping, megs uint64) {
	diskSize := minDiskSize

	if cons.RootDisk != nil {
		if *cons.RootDisk >= minDiskSize {
			diskSize = *cons.RootDisk
		} else {
			logger.Infof("Ignoring root-disk constraint of %dM because it is smaller than the EC2 image size of %dM",
				*cons.RootDisk, minDiskSize)
		}
	}

	// AWS's volume size is in gigabytes, root-disk is in megabytes,
	// so round up to the nearest gigabyte.
	volsize := int64((diskSize + 1023) / 1024)
	return ec2.BlockDeviceMapping{
			DeviceName: "/dev/sda1",
			VolumeSize: volsize,
		},
		uint64(volsize * 1024)
}

// groupInfoByName returns information on the security group
// with the given name including rules and other details.
func (e *environ) groupInfoByName(groupName string) (ec2.SecurityGroupInfo, error) {
	// Non-default VPC does not support name-based group lookups, can
	// use a filter by group name instead when support is needed.
	limitToGroups := []ec2.SecurityGroup{{Name: groupName}}
	resp, err := e.ec2().SecurityGroups(limitToGroups, nil)
	if err != nil {
		return ec2.SecurityGroupInfo{}, err
	}
	if len(resp.Groups) != 1 {
		return ec2.SecurityGroupInfo{}, fmt.Errorf("expected one security group named %q, got %v", groupName, resp.Groups)
	}
	return resp.Groups[0], nil
}

// groupByName returns the security group with the given name.
func (e *environ) groupByName(groupName string) (ec2.SecurityGroup, error) {
	groupInfo, err := e.groupInfoByName(groupName)
	return groupInfo.SecurityGroup, err
}

// addGroupFilter sets a limit an instance filter so only those machines
// with the juju environment wide security group associated will be listed.
//
// An EC2 API call is required to resolve the group name to an id, as VPC
// enabled accounts do not support name based filtering.
// TODO: Detect classic accounts and just filter by name for those.
//
// Callers must handle InvalidGroup.NotFound errors to mean the same as no
// matching instances.
func (e *environ) addGroupFilter(filter *ec2.Filter) error {
	groupName := e.jujuGroupName()
	group, err := e.groupByName(groupName)
	if err != nil {
		return err
	}
	// EC2 should support filtering with and without the 'instance.'
	// prefix, but only the form with seems to work with default VPC.
	filter.Add("instance.group-id", group.Id)
	return nil
}

// gatherInstances tries to get information on each instance
// id whose corresponding insts slot is nil.
// It returns environs.ErrPartialInstances if the insts
// slice has not been completely filled.
func (e *environ) gatherInstances(ids []instance.Id, insts []instance.Instance) error {
	var need []string
	for i, inst := range insts {
		if inst == nil {
			need = append(need, string(ids[i]))
		}
	}
	if len(need) == 0 {
		return nil
	}
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	err := e.addGroupFilter(filter)
	if err != nil {
		if ec2ErrCode(err) == "InvalidGroup.NotFound" {
			return environs.ErrPartialInstances
		}
		return err
	}
	filter.Add("instance-id", need...)
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return err
	}
	n := 0
	// For each requested id, add it to the returned instances
	// if we find it in the response.
	for i, id := range ids {
		if insts[i] != nil {
			continue
		}
		for j := range resp.Reservations {
			r := &resp.Reservations[j]
			for k := range r.Instances {
				if r.Instances[k].InstanceId == string(id) {
					inst := r.Instances[k]
					// TODO(wallyworld): lookup the details to fill in the instance type data
					insts[i] = &ec2Instance{e: e, Instance: &inst}
					n++
				}
			}
		}
	}
	if n < len(ids) {
		return environs.ErrPartialInstances
	}
	return nil
}

func (e *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instance.Instance, len(ids))
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	var err error
	for a := shortAttempt.Start(); a.Next(); {
		err = e.gatherInstances(ids, insts)
		if err == nil || err != environs.ErrPartialInstances {
			break
		}
	}
	if err == environs.ErrPartialInstances {
		for _, inst := range insts {
			if inst != nil {
				return insts, environs.ErrPartialInstances
			}
		}
		return nil, environs.ErrNoInstances
	}
	if err != nil {
		return nil, err
	}
	return insts, nil
}

func (e *environ) AllInstances() ([]instance.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	err := e.addGroupFilter(filter)
	if err != nil {
		if ec2ErrCode(err) == "InvalidGroup.NotFound" {
			return nil, nil
		}
		return nil, err
	}
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	var insts []instance.Instance
	for _, r := range resp.Reservations {
		for i := range r.Instances {
			inst := r.Instances[i]
			// TODO(wallyworld): lookup the details to fill in the instance type data
			insts = append(insts, &ec2Instance{e: e, Instance: &inst})
		}
	}
	return insts, nil
}

func (e *environ) Destroy() error {
	return common.Destroy(e)
}

func portsToIPPerms(ports []instance.Port) []ec2.IPPerm {
	ipPerms := make([]ec2.IPPerm, len(ports))
	for i, p := range ports {
		ipPerms[i] = ec2.IPPerm{
			Protocol:  p.Protocol,
			FromPort:  p.Number,
			ToPort:    p.Number,
			SourceIPs: []string{"0.0.0.0/0"},
		}
	}
	return ipPerms
}

func (e *environ) openPortsInGroup(name string, ports []instance.Port) error {
	if len(ports) == 0 {
		return nil
	}
	// Give permissions for anyone to access the given ports.
	g, err := e.groupByName(name)
	if err != nil {
		return err
	}
	ipPerms := portsToIPPerms(ports)
	_, err = e.ec2().AuthorizeSecurityGroup(g, ipPerms)
	if err != nil && ec2ErrCode(err) == "InvalidPermission.Duplicate" {
		if len(ports) == 1 {
			return nil
		}
		// If there's more than one port and we get a duplicate error,
		// then we go through authorizing each port individually,
		// otherwise the ports that were *not* duplicates will have
		// been ignored
		for i := range ipPerms {
			_, err := e.ec2().AuthorizeSecurityGroup(g, ipPerms[i:i+1])
			if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
				return fmt.Errorf("cannot open port %v: %v", ipPerms[i], err)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot open ports: %v", err)
	}
	return nil
}

func (e *environ) closePortsInGroup(name string, ports []instance.Port) error {
	if len(ports) == 0 {
		return nil
	}
	// Revoke permissions for anyone to access the given ports.
	// Note that ec2 allows the revocation of permissions that aren't
	// granted, so this is naturally idempotent.
	g, err := e.groupByName(name)
	if err != nil {
		return err
	}
	_, err = e.ec2().RevokeSecurityGroup(g, portsToIPPerms(ports))
	if err != nil {
		return fmt.Errorf("cannot close ports: %v", err)
	}
	return nil
}

func (e *environ) portsInGroup(name string) (ports []instance.Port, err error) {
	group, err := e.groupInfoByName(name)
	if err != nil {
		return nil, err
	}
	for _, p := range group.IPPerms {
		if len(p.SourceIPs) != 1 {
			logger.Warningf("unexpected IP permission found: %v", p)
			continue
		}
		for i := p.FromPort; i <= p.ToPort; i++ {
			ports = append(ports, instance.Port{
				Protocol: p.Protocol,
				Number:   i,
			})
		}
	}
	instance.SortPorts(ports)
	return ports, nil
}

func (e *environ) OpenPorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("opened ports in global group: %v", ports)
	return nil
}

func (e *environ) ClosePorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("closed ports in global group: %v", ports)
	return nil
}

func (e *environ) Ports() ([]instance.Port, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment",
			e.Config().FirewallMode())
	}
	return e.portsInGroup(e.globalGroupName())
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (e *environ) terminateInstances(ids []instance.Id) error {
	if len(ids) == 0 {
		return nil
	}
	var err error
	ec2inst := e.ec2()
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = string(id)
	}
	for a := shortAttempt.Start(); a.Next(); {
		_, err = ec2inst.TerminateInstances(strs)
		if err == nil || ec2ErrCode(err) != "InvalidInstanceID.NotFound" {
			return err
		}
	}
	if len(ids) == 1 {
		return err
	}
	// If we get a NotFound error, it means that no instances have been
	// terminated even if some exist, so try them one by one, ignoring
	// NotFound errors.
	var firstErr error
	for _, id := range ids {
		_, err = ec2inst.TerminateInstances([]string{string(id)})
		if ec2ErrCode(err) == "InvalidInstanceID.NotFound" {
			err = nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (e *environ) globalGroupName() string {
	return fmt.Sprintf("%s-global", e.jujuGroupName())
}

func (e *environ) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", e.jujuGroupName(), machineId)
}

func (e *environ) jujuGroupName() string {
	return "juju-" + e.name
}

func (inst *ec2Instance) OpenPorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) ClosePorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) Ports(machineId string) ([]instance.Port, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	return inst.e.portsInGroup(name)
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same EC2 account.  In
// addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(machineId string, statePort, apiPort int) ([]ec2.SecurityGroup, error) {
	jujuGroup, err := e.ensureGroup(e.jujuGroupName(),
		[]ec2.IPPerm{
			{
				Protocol:  "tcp",
				FromPort:  22,
				ToPort:    22,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol:  "tcp",
				FromPort:  statePort,
				ToPort:    statePort,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol:  "tcp",
				FromPort:  apiPort,
				ToPort:    apiPort,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol: "tcp",
				FromPort: 0,
				ToPort:   65535,
			},
			{
				Protocol: "udp",
				FromPort: 0,
				ToPort:   65535,
			},
			{
				Protocol: "icmp",
				FromPort: -1,
				ToPort:   -1,
			},
		})
	if err != nil {
		return nil, err
	}
	var machineGroup ec2.SecurityGroup
	switch e.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = e.ensureGroup(e.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = e.ensureGroup(e.globalGroupName(), nil)
	}
	if err != nil {
		return nil, err
	}
	return []ec2.SecurityGroup{jujuGroup, machineGroup}, nil
}

// zeroGroup holds the zero security group.
var zeroGroup ec2.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
// Any entries in perms without SourceIPs will be granted for
// the named group only.
func (e *environ) ensureGroup(name string, perms []ec2.IPPerm) (g ec2.SecurityGroup, err error) {
	ec2inst := e.ec2()
	resp, err := ec2inst.CreateSecurityGroup(name, "juju group")
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return zeroGroup, err
	}

	var have permSet
	if err == nil {
		g = resp.SecurityGroup
	} else {
		resp, err := ec2inst.SecurityGroups(ec2.SecurityGroupNames(name), nil)
		if err != nil {
			return zeroGroup, err
		}
		info := resp.Groups[0]
		// It's possible that the old group has the wrong
		// description here, but if it does it's probably due
		// to something deliberately playing games with juju,
		// so we ignore it.
		g = info.SecurityGroup
		have = newPermSetForGroup(info.IPPerms, g)
	}
	want := newPermSetForGroup(perms, g)
	revoke := make(permSet)
	for p := range have {
		if !want[p] {
			revoke[p] = true
		}
	}
	if len(revoke) > 0 {
		_, err := ec2inst.RevokeSecurityGroup(g, revoke.ipPerms())
		if err != nil {
			return zeroGroup, fmt.Errorf("cannot revoke security group: %v", err)
		}
	}

	add := make(permSet)
	for p := range want {
		if !have[p] {
			add[p] = true
		}
	}
	if len(add) > 0 {
		_, err := ec2inst.AuthorizeSecurityGroup(g, add.ipPerms())
		if err != nil {
			return zeroGroup, fmt.Errorf("cannot authorize securityGroup: %v", err)
		}
	}
	return g, nil
}

// permKey represents a permission for a group or an ip address range
// to access the given range of ports. Only one of groupName or ipAddr
// should be non-empty.
type permKey struct {
	protocol string
	fromPort int
	toPort   int
	groupId  string
	ipAddr   string
}

type permSet map[permKey]bool

// newPermSetForGroup returns a set of all the permissions in the
// given slice of IPPerms. It ignores the name and owner
// id in source groups, and any entry with no source ips will
// be granted for the given group only.
func newPermSetForGroup(ps []ec2.IPPerm, group ec2.SecurityGroup) permSet {
	m := make(permSet)
	for _, p := range ps {
		k := permKey{
			protocol: p.Protocol,
			fromPort: p.FromPort,
			toPort:   p.ToPort,
		}
		if len(p.SourceIPs) > 0 {
			for _, ip := range p.SourceIPs {
				k.ipAddr = ip
				m[k] = true
			}
		} else {
			k.groupId = group.Id
			m[k] = true
		}
	}
	return m
}

// ipPerms returns m as a slice of permissions usable
// with the ec2 package.
func (m permSet) ipPerms() (ps []ec2.IPPerm) {
	// We could compact the permissions, but it
	// hardly seems worth it.
	for p := range m {
		ipp := ec2.IPPerm{
			Protocol: p.protocol,
			FromPort: p.fromPort,
			ToPort:   p.toPort,
		}
		if p.ipAddr != "" {
			ipp.SourceIPs = []string{p.ipAddr}
		} else {
			ipp.SourceGroups = []ec2.UserSecurityGroup{{Id: p.groupId}}
		}
		ps = append(ps, ipp)
	}
	return
}

// If the err is of type *ec2.Error, ec2ErrCode returns
// its code, otherwise it returns the empty string.
func ec2ErrCode(err error) string {
	ec2err, _ := err.(*ec2.Error)
	if ec2err == nil {
		return ""
	}
	return ec2err.Code
}

// metadataHost holds the address of the instance metadata service.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var metadataHost = "http://169.254.169.254"

// fetchMetadata fetches a single atom of data from the ec2 instance metadata service.
// http://docs.amazonwebservices.com/AWSEC2/latest/UserGuide/AESDG-chapter-instancedata.html
func fetchMetadata(name string) (value string, err error) {
	uri := fmt.Sprintf("%s/2011-01-01/meta-data/%s", metadataHost, name)
	defer utils.ErrorContextf(&err, "cannot get %q", uri)
	for a := shortAttempt.Start(); a.Next(); {
		var resp *http.Response
		resp, err = http.Get(uri)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("bad http response %v", resp.Status)
			continue
		}
		var data []byte
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(data)), nil
	}
	return
}

// GetImageSources returns a list of sources which are used to search for simplestreams image metadata.
func (e *environ) GetImageSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	sources := []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseImagesPath)}
	return sources, nil
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *environ) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	sources := []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}
	return sources, nil
}
