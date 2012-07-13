package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"sync"
	"time"
)

const zkPort = 2181

var zkPortSuffix = fmt.Sprintf(":%d", zkPort)

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by longAttempt.
var shortAttempt = environs.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var longAttempt = environs.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
}

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var _ environs.EnvironProvider = environProvider{}

type environ struct {
	name string

	// configMutex protects the *Unlocked fields below.
	configMutex           sync.Mutex
	configUnlocked        *providerConfig
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

func (cfg *providerConfig) Open() (environs.Environ, error) {
	log.Printf("environs/ec2: opening environment %q", cfg.name)
	if aws.Regions[cfg.region].EC2Endpoint == "" {
		return nil, fmt.Errorf("no ec2 endpoint found for region %q, opening %q", cfg.region, cfg.name)
	}
	e := new(environ)
	e.SetConfig(cfg)
	return e, nil
}

func (e *environ) SetConfig(cfg environs.EnvironConfig) {
	config := cfg.(*providerConfig)
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	// TODO(dfc) bug #1018207, renaming an environment once it is in use should be forbidden
	e.name = config.name
	e.configUnlocked = config
	e.ec2Unlocked = ec2.New(config.auth, aws.Regions[config.region])
	e.s3Unlocked = s3.New(config.auth, aws.Regions[config.region])

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &storage{
		bucket: e.s3Unlocked.Bucket(config.bucket),
	}
	if config.publicBucket != "" {
		e.publicStorageUnlocked = &storage{
			bucket: e.s3Unlocked.Bucket(config.publicBucket),
		}
	} else {
		e.publicStorageUnlocked = nil
	}
}

func (e *environ) config() *providerConfig {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	return e.configUnlocked
}

func (e *environ) ec2() *ec2.EC2 {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	return e.ec2Unlocked
}

func (e *environ) s3() *s3.S3 {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	return e.s3Unlocked
}

func (e *environ) Name() string {
	return e.config().name
}

func (e *environ) Storage() environs.Storage {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
	return e.storageUnlocked
}

func (e *environ) PublicStorage() environs.StorageReader {
	e.configMutex.Lock()
	defer e.configMutex.Unlock()
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
	if uploadTools {
		err := environs.PutTools(e.Storage())
		if err != nil {
			return fmt.Errorf("cannot upload tools: %v", err)
		}
	}
	inst, err := e.startInstance(0, nil, true)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = e.saveState(&bootstrapState{
		ZookeeperInstances: []string{inst.Id()},
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
	log.Printf("environs/ec2: waiting for zookeeper DNS name(s) of instances %v", st.ZookeeperInstances)
	for a := longAttempt.Start(); len(addrs) == 0 && a.Next(); {
		insts, err := e.Instances(st.ZookeeperInstances)
		if err != nil && err != environs.ErrPartialInstances {
			return nil, err
		}
		for _, inst := range insts {
			if inst == nil {
				continue
			}
			name := inst.(*instance).Instance.DNSName
			if name != "" {
				addrs = append(addrs, name+zkPortSuffix)
			}
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("timed out waiting for zk address from %v", st.ZookeeperInstances)
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

func (e *environ) StartInstance(machineId int, info *state.Info) (environs.Instance, error) {
	log.Printf("environs/ec2: starting machine %d in %q", machineId, e.name)
	return e.startInstance(machineId, info, false)
}

func (e *environ) userData(machineId int, info *state.Info, master bool, toolsURL string) ([]byte, error) {
	cfg := &machineConfig{
		provisioner:        master,
		zookeeper:          master,
		stateInfo:          info,
		instanceIdAccessor: "$(curl http://169.254.169.254/1.0/meta-data/instance-id)",
		providerType:       "ec2",
		toolsURL:           toolsURL,
		machineId:          machineId,
		authorizedKeys:     e.config().authorizedKeys,
	}
	cloudcfg, err := newCloudInit(cfg)
	if err != nil {
		return nil, err
	}
	return cloudcfg.Render()
}

// startInstance is the internal version of StartInstance, used by Bootstrap
// as well as via StartInstance itself. If master is true, a bootstrap
// instance will be started.
func (e *environ) startInstance(machineId int, info *state.Info, master bool) (environs.Instance, error) {
	spec, err := findInstanceSpec(&instanceConstraint{
		series: environs.CurrentSeries,
		arch:   environs.CurrentArch,
		region: e.config().region,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot find image satisfying constraints: %v", err)
	}
	log.Debugf("looking for tools for version %v; instance spec %#v", version.Current, spec)
	toolsURL, err := environs.FindTools(e, version.Current, spec.series, spec.arch)
	if err != nil {
		return nil, fmt.Errorf("cannot find juju tools that would work on the specified instance: %v", err)
	}
	userData, err := e.userData(machineId, info, master, toolsURL)
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
	// to properly observe e.storageUnlocked we need to get it's value while
	// holding e.configMutex. e.Storage() does this for us, then we convert
	// back to the (*storage) to access the private deleteAll() method.
	st := e.Storage().(*storage)
	err = st.deleteAll()
	if err != nil {
		return err
	}
	return nil
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
	var firstErr error
	// If we get a NotFound error, it means that no instances have been
	// terminated even if some exist, so try them one by one, ignoring
	// NotFound errors.
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
	ipPerms := make([]ec2.IPPerm, len(ports))
	for i, p := range ports {
		ipPerms[i] = ec2.IPPerm{
			Protocol:  p.Protocol,
			FromPort:  p.Number,
			ToPort:    p.Number,
			SourceIPs: []string{"0.0.0.0/0"},
		}
	}
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	_, err := inst.e.ec2().AuthorizeSecurityGroup(g, ipPerms)
	if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
		return err
	}
	return nil
}

func (inst *instance) ClosePorts(machineId int, ports []state.Port) error {
	if len(ports) == 0 {
		return nil
	}
	// Revoke permissions for anyone to access the given ports.
	ipPerms := make([]ec2.IPPerm, len(ports))
	for i, p := range ports {
		ipPerms[i] = ec2.IPPerm{
			Protocol:  p.Protocol,
			FromPort:  p.Number,
			ToPort:    p.Number,
			SourceIPs: []string{"0.0.0.0/0"},
		}
	}
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	_, err := inst.e.ec2().RevokeSecurityGroup(g, ipPerms)
	if err != nil {
		return fmt.Errorf("cannot open ports: %v", err)
	}
	return nil
}

func (inst *instance) Ports(machineId int) (ports []state.Port, err error) {
	g := ec2.SecurityGroup{Name: inst.e.machineGroupName(machineId)}
	resp, err := inst.e.ec2().SecurityGroups([]ec2.SecurityGroup{g}, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Groups) != 1 {
		return nil, fmt.Errorf("expected one security group, got %d", len(resp.Groups))
	}
	for _, p := range resp.Groups[0].IPPerms {
		if len(p.SourceIPs) != 1 || p.SourceIPs[0] != "0.0.0.0/0" {
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
	jujuGroup, err := e.ensureGroup(e.groupName(),
		[]ec2.IPPerm{
			{
				Protocol:  "tcp",
				FromPort:  22,
				ToPort:    22,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			// TODO authorize internal traffic
		})
	if err != nil {
		return nil, err
	}
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

	want := newPermSet(perms)
	var have permSet
	if err == nil {
		g = resp.SecurityGroup
	} else {
		resp, err := ec2inst.SecurityGroups(ec2.SecurityGroupNames(name), nil)
		if err != nil {
			return zeroGroup, err
		}
		// It's possible that the old group has the wrong
		// description here, but if it does it's probably due
		// to something deliberately playing games with juju,
		// so we ignore it.
		have = newPermSet(resp.Groups[0].IPPerms)
		g = resp.Groups[0].SecurityGroup
	}
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
// to access the given range of ports. Only one of groupId or ipAddr
// should be non-empty.
type permKey struct {
	protocol string
	fromPort int
	toPort   int
	groupId  string
	ipAddr   string
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
			k.groupId = g.Id
			m[k] = true
		}
		k.groupId = ""
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
