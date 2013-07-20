// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
	ecfgMutex             sync.Mutex
	ecfgUnlocked          *environConfig
	ec2Unlocked           *ec2.EC2
	s3Unlocked            *s3.S3
	storageUnlocked       environs.Storage
	publicStorageUnlocked environs.StorageReader // optional.
}

var _ environs.Environ = (*environ)(nil)

type ec2Instance struct {
	e *environ
	*ec2.Instance
	arch     *string
	instType *instances.InstanceType
}

func (inst *ec2Instance) String() string {
	return inst.InstanceId
}

var _ instance.Instance = (*ec2Instance)(nil)

func (inst *ec2Instance) Id() instance.Id {
	return instance.Id(inst.InstanceId)
}

func (inst *ec2Instance) hardwareCharacteristics() *instance.HardwareCharacteristics {
	hc := &instance.HardwareCharacteristics{Arch: inst.arch}
	if inst.instType != nil {
		hc.Mem = &inst.instType.Mem
		hc.CpuCores = &inst.instType.CpuCores
		hc.CpuPower = inst.instType.CpuPower
	}
	return hc
}

func (inst *ec2Instance) DNSName() (string, error) {
	if inst.Instance.DNSName != "" {
		return inst.Instance.DNSName, nil
	}
	// Fetch the instance information again, in case
	// the DNS information has become available.
	insts, err := inst.e.Instances([]instance.Id{inst.Id()})
	if err != nil {
		return "", err
	}
	freshInst := insts[0].(*ec2Instance).Instance
	if freshInst.DNSName == "" {
		return "", instance.ErrNoDNSName
	}
	inst.Instance.DNSName = freshInst.DNSName
	return freshInst.DNSName, nil
}

func (inst *ec2Instance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(inst)
}

func (p environProvider) BoilerplateConfig() string {
	return `
## https://juju.ubuntu.com/get-started/amazon/
amazon:
  type: ec2
  admin-secret: {{rand}}
  # globally unique S3 bucket name
  control-bucket: juju-{{rand}}
  # override if your workstation is running a different series to which you are deploying
  # default-series: precise
  # region defaults to us-east-1, override if required
  # region: us-east-1
  # Usually set via the env variable AWS_ACCESS_KEY_ID, but can be specified here
  # access-key: <secret>
  # Usually set via the env variable AWS_SECRET_ACCESS_KEY, but can be specified here
  # secret-key: <secret>

`[1:]
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Infof("environs/ec2: opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	e.name = cfg.Name()
	return e, nil
}

func (environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	m := make(map[string]interface{})
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
	publicBucketRegion := aws.Regions[ecfg.publicBucketRegion()]
	e.ec2Unlocked = ec2.New(auth, region)
	e.s3Unlocked = s3.New(auth, region)

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &storage{
		bucket: e.s3Unlocked.Bucket(ecfg.controlBucket()),
	}
	if ecfg.publicBucket() != "" {
		e.publicStorageUnlocked = &storage{
			bucket: s3.New(auth, publicBucketRegion).Bucket(ecfg.publicBucket()),
		}
	} else {
		e.publicStorageUnlocked = nil
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

func (e *environ) Storage() environs.Storage {
	e.ecfgMutex.Lock()
	storage := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return storage
}

func (e *environ) PublicStorage() environs.StorageReader {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	if e.publicStorageUnlocked == nil {
		return environs.EmptyStorage
	}
	return e.publicStorageUnlocked
}

// TODO(bug 1199847): Much of this work can be shared between providers.
func (e *environ) Bootstrap(cons constraints.Value) error {
	// The bootstrap instance gets machine id "0".  This is not related to
	// instance ids.  Juju assigns the machine ID.
	const machineID = "0"
	log.Infof("environs/ec2: bootstrapping environment %q", e.name)
	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	if err := environs.VerifyBootstrapInit(e, shortAttempt); err != nil {
		return err
	}

	possibleTools, err := environs.FindBootstrapTools(e, cons)
	if err != nil {
		return err
	}
	err = environs.CheckToolsSeries(possibleTools, e.Config().DefaultSeries())
	if err != nil {
		return err
	}
	stateFileURL, err := environs.CreateStateFile(e.Storage())
	if err != nil {
		return err
	}

	machineConfig := environs.NewBootstrapMachineConfig(machineID, stateFileURL)

	// TODO(wallyworld) - save bootstrap machine metadata
	inst, characteristics, err := e.internalStartInstance(cons, possibleTools, machineConfig)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = environs.SaveState(e.Storage(), &environs.BootstrapState{
		StateInstances:  []instance.Id{inst.Id()},
		Characteristics: []instance.HardwareCharacteristics{*characteristics},
	})
	if err != nil {
		// ignore error on StopInstance because the previous error is
		// more important.
		e.StopInstances([]instance.Instance{inst})
		return fmt.Errorf("cannot save state: %v", err)
	}
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to use S3 to make sure that only one succeeds.
	// Perhaps consider using SimpleDB for state storage
	// which would enable that possibility.
	return nil
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	return environs.StateInfo(e)
}

// getImageBaseURLs returns a list of URLs which are used to search for simplestreams image metadata.
func (e *environ) getImageBaseURLs() ([]string, error) {
	// Use the default simplestreams base URL.
	return []string{imagemetadata.DefaultBaseURL}, nil
}

// TODO(bug 1199847): This work can be shared between providers.
func (e *environ) StartInstance(machineId, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {
	possibleTools, err := environs.FindInstanceTools(e, series, cons)
	if err != nil {
		return nil, nil, err
	}
	err = environs.CheckToolsSeries(possibleTools, series)
	if err != nil {
		return nil, nil, err
	}
	machineConfig := environs.NewMachineConfig(machineId, machineNonce, stateInfo, apiInfo)

	return e.internalStartInstance(cons, possibleTools, machineConfig)
}

const ebsStorage = "ebs"

// internalStartInstance is the internal version of StartInstance, used by
// Bootstrap as well as via StartInstance itself.
// TODO(bug 1199847): Some of this work can be shared between providers.
func (e *environ) internalStartInstance(cons constraints.Value, possibleTools tools.List, machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {
	series := possibleTools.Series()
	if len(series) != 1 {
		panic(fmt.Errorf("should have gotten tools for one series, got %v", series))
	}
	arches := possibleTools.Arches()
	storage := ebsStorage
	baseURLs, err := e.getImageBaseURLs()
	if err != nil {
		return nil, nil, err
	}
	spec, err := findInstanceSpec(baseURLs, &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series[0],
		Arches:      arches,
		Constraints: cons,
		Storage:     &storage,
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
	log.Debugf("environs/ec2: ec2 user data; %d bytes", len(userData))
	config := e.Config()
	groups, err := e.setUpGroups(machineConfig.MachineId, config.StatePort(), config.APIPort())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var instances *ec2.RunInstancesResp

	for a := shortAttempt.Start(); a.Next(); {
		instances, err = e.ec2().RunInstances(&ec2.RunInstances{
			ImageId:        spec.Image.Id,
			MinCount:       1,
			MaxCount:       1,
			UserData:       userData,
			InstanceType:   spec.InstanceType.Name,
			SecurityGroups: groups,
		})
		if err == nil || ec2ErrCode(err) != "InvalidGroup.NotFound" {
			break
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instances.Instances) != 1 {
		return nil, nil, fmt.Errorf("expected 1 started instance, got %d", len(instances.Instances))
	}
	inst := &ec2Instance{
		e:        e,
		Instance: &instances.Instances[0],
		arch:     &spec.Image.Arch,
		instType: &spec.InstanceType,
	}
	log.Infof("environs/ec2: started instance %q", inst.Id())
	return inst, inst.hardwareCharacteristics(), nil
}

func (e *environ) StopInstances(insts []instance.Instance) error {
	ids := make([]instance.Id, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*ec2Instance).Id()
	}
	return e.terminateInstances(ids)
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
	filter.Add("group-name", e.jujuGroupName())
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
	filter.Add("group-name", e.jujuGroupName())
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

func (e *environ) Destroy(ensureInsts []instance.Instance) error {
	log.Infof("environs/ec2: destroying environment %q", e.name)
	insts, err := e.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[instance.Id]bool)
	var ids []instance.Id
	for _, inst := range insts {
		ids = append(ids, inst.Id())
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := instance.Id(inst.(*ec2Instance).InstanceId)
		if !found[id] {
			ids = append(ids, id)
			found[id] = true
		}
	}
	err = e.terminateInstances(ids)
	if err != nil {
		return err
	}

	return e.Storage().RemoveAll()
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
	ipPerms := portsToIPPerms(ports)
	g := ec2.SecurityGroup{Name: name}
	_, err := e.ec2().AuthorizeSecurityGroup(g, ipPerms)
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
	g := ec2.SecurityGroup{Name: name}
	_, err := e.ec2().RevokeSecurityGroup(g, portsToIPPerms(ports))
	if err != nil {
		return fmt.Errorf("cannot close ports: %v", err)
	}
	return nil
}

func (e *environ) portsInGroup(name string) (ports []instance.Port, err error) {
	g := ec2.SecurityGroup{Name: name}
	resp, err := e.ec2().SecurityGroups([]ec2.SecurityGroup{g}, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Groups) != 1 {
		return nil, fmt.Errorf("expected one security group, got %d", len(resp.Groups))
	}
	for _, p := range resp.Groups[0].IPPerms {
		if len(p.SourceIPs) != 1 {
			log.Warningf("environs/ec2: unexpected IP permission found: %v", p)
			continue
		}
		for i := p.FromPort; i <= p.ToPort; i++ {
			ports = append(ports, instance.Port{
				Protocol: p.Protocol,
				Number:   i,
			})
		}
	}
	state.SortPorts(ports)
	return ports, nil
}

func (e *environ) OpenPorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode for opening ports on environment: %q",
			e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	log.Infof("environs/ec2: opened ports in global group: %v", ports)
	return nil
}

func (e *environ) ClosePorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode for closing ports on environment: %q",
			e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	log.Infof("environs/ec2: closed ports in global group: %v", ports)
	return nil
}

func (e *environ) Ports() ([]instance.Port, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode for retrieving ports from environment: %q",
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
		return fmt.Errorf("invalid firewall mode for opening ports on instance: %q",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	log.Infof("environs/ec2: opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) ClosePorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode for closing ports on instance: %q",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	log.Infof("environs/ec2: closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) Ports(machineId string) ([]instance.Port, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode for retrieving ports from instance: %q",
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
	sourceGroups := []ec2.UserSecurityGroup{{Name: e.jujuGroupName()}}
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
				Protocol:     "tcp",
				FromPort:     0,
				ToPort:       65535,
				SourceGroups: sourceGroups,
			},
			{
				Protocol:     "udp",
				FromPort:     0,
				ToPort:       65535,
				SourceGroups: sourceGroups,
			},
			{
				Protocol:     "icmp",
				FromPort:     -1,
				ToPort:       -1,
				SourceGroups: sourceGroups,
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
		have = newPermSet(info.IPPerms)
		g = info.SecurityGroup
	}
	want := newPermSet(perms)
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
	protocol  string
	fromPort  int
	toPort    int
	groupName string
	ipAddr    string
}

type permSet map[permKey]bool

// newPermSet returns a set of all the permissions in the
// given slice of IPPerms. It ignores the name and owner
// id in source groups, using group ids only.
func newPermSet(ps []ec2.IPPerm) permSet {
	m := make(permSet)
	for _, p := range ps {
		k := permKey{
			protocol: p.Protocol,
			fromPort: p.FromPort,
			toPort:   p.ToPort,
		}
		for _, g := range p.SourceGroups {
			k.groupName = g.Name
			m[k] = true
		}
		k.groupName = ""
		for _, ip := range p.SourceIPs {
			k.ipAddr = ip
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
			ipp.SourceGroups = []ec2.UserSecurityGroup{{Name: p.groupName}}
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
