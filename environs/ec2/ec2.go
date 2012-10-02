package ec2

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/http"
	"strings"
	"sync"
	"time"
)

const mgoPort = 37017

var mgoPortSuffix = fmt.Sprintf(":%d", mgoPort)

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by longAttempt.
var shortAttempt = trivial.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var longAttempt = trivial.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
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
	storageUnlocked       *storage
	publicStorageUnlocked *storage // optional.
}

var _ environs.Environ = (*environ)(nil)

type instance struct {
	e *environ
	*ec2.Instance
}

func (inst *instance) String() string {
	return inst.Id()
}

var _ environs.Instance = (*instance)(nil)

func (inst *instance) Id() string {
	return inst.InstanceId
}

func (inst *instance) DNSName() (string, error) {
	if inst.Instance.DNSName != "" {
		return inst.Instance.DNSName, nil
	}
	// Fetch the instance information again, in case
	// the DNS information has become available.
	insts, err := inst.e.Instances([]string{inst.Id()})
	if err != nil {
		return "", err
	}
	freshInst := insts[0].(*instance).Instance
	if freshInst.DNSName == "" {
		return "", environs.ErrNoDNSName
	}
	inst.Instance.DNSName = freshInst.DNSName
	return freshInst.DNSName, nil
}

func (inst *instance) WaitDNSName() (string, error) {
	for a := longAttempt.Start(); a.Next(); {
		name, err := inst.DNSName()
		if err == nil || err != environs.ErrNoDNSName {
			return name, err
		}
	}
	return "", fmt.Errorf("timed out trying to get DNS address for %v", inst.Id())
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Printf("environs/ec2: opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
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
	e.name = ecfg.Name()
	e.ecfgUnlocked = ecfg

	auth := aws.Auth{ecfg.accessKey(), ecfg.secretKey()}
	region := aws.Regions[ecfg.region()]
	e.ec2Unlocked = ec2.New(auth, region)
	e.s3Unlocked = s3.New(auth, region)

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &storage{
		bucket: e.s3Unlocked.Bucket(ecfg.controlBucket()),
	}
	if ecfg.publicBucket() != "" {
		e.publicStorageUnlocked = &storage{
			bucket: e.s3Unlocked.Bucket(ecfg.publicBucket()),
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

func (e *environ) Bootstrap(uploadTools bool) error {
	log.Printf("environs/ec2: bootstrapping environment %q", e.name)
	_, err := e.loadState()
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if _, notFound := err.(*environs.NotFoundError); !notFound {
		return fmt.Errorf("cannot query old bootstrap state: %v", err)
	}
	var tools *state.Tools
	if uploadTools {
		tools, err = environs.PutTools(e.Storage(), nil)
		if err != nil {
			return fmt.Errorf("cannot upload tools: %v", err)
		}
	} else {
		flags := environs.HighestVersion | environs.CompatVersion
		tools, err = environs.FindTools(e, version.Current, flags)
		if err != nil {
			return fmt.Errorf("cannot find tools: %v", err)
		}
	}
	config, err := environs.BootstrapConfig(providerInstance, e.Config(), tools)
	if err != nil {
		return fmt.Errorf("unable to determine inital configuration: %v", err)
	}
	inst, err := e.startInstance(0, nil, tools, true, config)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = e.saveState(&bootstrapState{
		StateInstances: []string{inst.Id()},
	})
	if err != nil {
		// ignore error on StopInstance because the previous error is
		// more important.
		e.StopInstances([]environs.Instance{inst})
		return fmt.Errorf("cannot save state: %v", err)
	}
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to use S3 to make sure that only one succeeds.
	// Perhaps consider using SimpleDB for state storage
	// which would enable that possibility.

	return nil
}

func (e *environ) StateInfo() (*state.Info, error) {
	st, err := e.loadState()
	if err != nil {
		return nil, err
	}
	var addrs []string
	// Wait for the DNS names of any of the instances
	// to become available.
	log.Printf("environs/ec2: waiting for DNS name(s) of state server instances %v", st.StateInstances)
	for a := longAttempt.Start(); len(addrs) == 0 && a.Next(); {
		insts, err := e.Instances(st.StateInstances)
		if err != nil && err != environs.ErrPartialInstances {
			return nil, err
		}
		for _, inst := range insts {
			if inst == nil {
				continue
			}
			name := inst.(*instance).Instance.DNSName
			if name != "" {
				addrs = append(addrs, name+mgoPortSuffix)
			}
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("timed out waiting for mgo address from %v", st.StateInstances)
	}
	return &state.Info{
		Addrs:  addrs,
		UseSSH: true,
	}, nil
}

// AssignmentPolicy for EC2 is to deploy units only on machines without other
// units already assigned, and to launch new machines as required.
func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	return state.AssignUnused
}

func (e *environ) StartInstance(machineId int, info *state.Info, tools *state.Tools) (environs.Instance, error) {
	return e.startInstance(machineId, info, tools, false, nil)
}

func (e *environ) userData(machineId int, info *state.Info, tools *state.Tools, master bool, config *config.Config) ([]byte, error) {
	cfg := &cloudinit.MachineConfig{
		StateServer:        master,
		StateInfo:          info,
		InstanceIdAccessor: "$(curl http://169.254.169.254/1.0/meta-data/instance-id)",
		ProviderType:       "ec2",
		DataDir:            "/var/lib/juju",
		Tools:              tools,
		MachineId:          machineId,
		AuthorizedKeys:     e.ecfg().AuthorizedKeys(),
		Config:             config,
	}
	cloudcfg, err := cloudinit.New(cfg)
	if err != nil {
		return nil, err
	}
	return cloudcfg.Render()
}

// startInstance is the internal version of StartInstance, used by Bootstrap
// as well as via StartInstance itself. If master is true, a bootstrap
// instance will be started.
func (e *environ) startInstance(machineId int, info *state.Info, tools *state.Tools, master bool, config *config.Config) (environs.Instance, error) {
	if tools == nil {
		var err error
		flags := environs.HighestVersion | environs.CompatVersion
		tools, err = environs.FindTools(e, version.Current, flags)
		if err != nil {
			return nil, err
		}
	}
	log.Printf("environs/ec2: starting machine %d in %q running tools version %q from %q", machineId, e.name, tools.Binary, tools.URL)
	spec, err := findInstanceSpec(&instanceConstraint{
		series: tools.Series,
		arch:   tools.Arch,
		region: e.ecfg().region(),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot find image satisfying constraints: %v", err)
	}
	// TODO quick sanity check that we can access the tools URL?
	userData, err := e.userData(machineId, info, tools, master, config)
	if err != nil {
		return nil, fmt.Errorf("cannot make user data: %v", err)
	}
	groups, err := e.setUpGroups(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var instances *ec2.RunInstancesResp

	for a := shortAttempt.Start(); a.Next(); {
		instances, err = e.ec2().RunInstances(&ec2.RunInstances{
			ImageId:        spec.imageId,
			MinCount:       1,
			MaxCount:       1,
			UserData:       userData,
			InstanceType:   "m1.small",
			SecurityGroups: groups,
		})
		if err == nil || ec2ErrCode(err) != "InvalidGroup.NotFound" {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instances.Instances) != 1 {
		return nil, fmt.Errorf("expected 1 started instance, got %d", len(instances.Instances))
	}
	inst := &instance{e, &instances.Instances[0]}
	log.Printf("environs/ec2: started instance %q", inst.Id())
	return inst, nil
}

func (e *environ) StopInstances(insts []environs.Instance) error {
	ids := make([]string, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*instance).InstanceId
	}
	return e.terminateInstances(ids)
}

// gatherInstances tries to get information on each instance
// id whose corresponding insts slot is nil.
// It returns environs.ErrPartialInstances if the insts
// slice has not been completely filled.
func (e *environ) gatherInstances(ids []string, insts []environs.Instance) error {
	var need []string
	for i, inst := range insts {
		if inst == nil {
			need = append(need, ids[i])
		}
	}
	if len(need) == 0 {
		return nil
	}
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", e.groupName())
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
				if r.Instances[k].InstanceId == id {
					inst := r.Instances[k]
					insts[i] = &instance{e, &inst}
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

func (e *environ) Instances(ids []string) ([]environs.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]environs.Instance, len(ids))
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

func (e *environ) AllInstances() ([]environs.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", e.groupName())
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	var insts []environs.Instance
	for _, r := range resp.Reservations {
		for i := range r.Instances {
			inst := r.Instances[i]
			insts = append(insts, &instance{e, &inst})
		}
	}
	return insts, nil
}

func (e *environ) Destroy(insts []environs.Instance) error {
	log.Printf("environs/ec2: destroying environment %q", e.name)
	// Try to find all the instances in the environ's group.
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", e.groupName())
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	var ids []string
	found := make(map[string]bool)
	for _, r := range resp.Reservations {
		for _, inst := range r.Instances {
			ids = append(ids, inst.InstanceId)
			found[inst.InstanceId] = true
		}
	}

	// Then add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range insts {
		id := inst.(*instance).InstanceId
		if !found[id] {
			ids = append(ids, id)
			found[id] = true
		}
	}
	err = e.terminateInstances(ids)
	if err != nil {
		return err
	}

	// To properly observe e.storageUnlocked we need to get its value while
	// holding e.ecfgMutex. e.Storage() does this for us, then we convert
	// back to the (*storage) to access the private deleteAll() method.
	st := e.Storage().(*storage)
	err = st.deleteAll()
	if err != nil {
		return err
	}
	return nil
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (e *environ) terminateInstances(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var err error
	ec2inst := e.ec2()
	for a := shortAttempt.Start(); a.Next(); {
		_, err = ec2inst.TerminateInstances(ids)
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
		_, err = ec2inst.TerminateInstances([]string{id})
		if ec2ErrCode(err) == "InvalidInstanceID.NotFound" {
			err = nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (e *environ) machineGroupName(machineId int) string {
	return fmt.Sprintf("%s-%d", e.groupName(), machineId)
}

func (e *environ) groupName() string {
	return "juju-" + e.name
}

func (inst *instance) OpenPorts(machineId int, ports []state.Port) error {
	if len(ports) == 0 {
		return nil
	}
	// Give permissions for anyone to access the given ports.
	// TODO(mue) Choose group depending on inst.e.Config().FirewallMode().
	ipPerms := portsToIPPerms(ports)
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	_, err := inst.e.ec2().AuthorizeSecurityGroup(g, ipPerms)
	if err != nil && ec2ErrCode(err) == "InvalidPermission.Duplicate" {
		if len(ports) == 1 {
			return nil
		}
		// If there's more than one port and we get a duplicate error,
		// then we go through authorizing each port individually,
		// otherwise the ports that were *not* duplicates will have
		// been ignored
		for i := range ipPerms {
			_, err := inst.e.ec2().AuthorizeSecurityGroup(g, ipPerms[i:i+1])
			if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
				return fmt.Errorf("cannot open port %v: %v", ipPerms[i], err)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot open ports: %v", err)
	}
	log.Printf("environs/ec2: opened ports in security group %s: %v", g.Name, ports)
	return nil
}

func (inst *instance) ClosePorts(machineId int, ports []state.Port) error {
	if len(ports) == 0 {
		return nil
	}
	// Revoke permissions for anyone to access the given ports.
	// Note that ec2 allows the revocation of permissions that aren't
	// granted, so this is naturally idempotent.
	// TODO(mue) Choose group depending on inst.e.Config().FirewallMode().
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	_, err := inst.e.ec2().RevokeSecurityGroup(g, portsToIPPerms(ports))
	if err != nil {
		return fmt.Errorf("cannot close ports: %v", err)
	}
	log.Printf("environs/ec2: closed ports in security group %s: %v", g.Name, ports)
	return nil
}

func portsToIPPerms(ports []state.Port) []ec2.IPPerm {
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

func (inst *instance) Ports(machineId int) (ports []state.Port, err error) {
	// TODO(mue) Choose group depending on inst.e.Config().FirewallMode().
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	resp, err := inst.e.ec2().SecurityGroups([]ec2.SecurityGroup{g}, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Groups) != 1 {
		return nil, fmt.Errorf("expected one security group, got %d", len(resp.Groups))
	}
	for _, p := range resp.Groups[0].IPPerms {
		if len(p.SourceIPs) != 1 {
			log.Printf("environs/ec2: unexpected IP permission found: %v", p)
			continue
		}
		for i := p.FromPort; i <= p.ToPort; i++ {
			ports = append(ports, state.Port{
				Protocol: p.Protocol,
				Number:   i,
			})
		}
	}
	state.SortPorts(ports)
	return ports, nil
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
// 
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same EC2 account.  In
// addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(machineId int) ([]ec2.SecurityGroup, error) {
	sourceGroups := []ec2.UserSecurityGroup{{Name: e.groupName()}}
	jujuGroup, err := e.ensureGroup(e.groupName(),
		[]ec2.IPPerm{
			{
				Protocol:  "tcp",
				FromPort:  22,
				ToPort:    22,
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
	// TODO(mue) Ensure machine group only if e.Config().FirewallMode()
	// is config.FwDefault.
	jujuMachineGroup, err := e.ensureGroup(e.machineGroupName(machineId), nil)
	if err != nil {
		return nil, err
	}
	return []ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil
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
	defer trivial.ErrorContextf(&err, "cannot get %q", uri)
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
