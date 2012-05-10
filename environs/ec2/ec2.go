package ec2

import (
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/state"
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
var shortAttempt = attemptStrategy{
	total: 5 * time.Second,
	delay: 200 * time.Millisecond,
}

var longAttempt = attemptStrategy{
	total: 3 * time.Minute,
	delay: 1 * time.Second,
}

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var _ environs.EnvironProvider = environProvider{}

type environ struct {
	name        string
	config      *providerConfig
	ec2         *ec2.EC2
	s3          *s3.S3
	bucketMutex sync.Mutex
	madeBucket  bool
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
	for a := longAttempt.start(); a.next(); {
		name, err := inst.DNSName()
		if err == nil || err != environs.ErrNoDNSName {
			return name, err
		}
	}
	return "", fmt.Errorf("timed out trying to get DNS address for %v", inst.Id())
}

func (environProvider) Open(name string, config interface{}) (e environs.Environ, err error) {
	log.Printf("environs/ec2: opening environment %q", name)
	cfg := config.(*providerConfig)
	if Regions[cfg.region].EC2Endpoint == "" {
		return nil, fmt.Errorf("no ec2 endpoint found for region %q, opening %q", cfg.region, name)
	}
	return &environ{
		name:   name,
		config: cfg,
		ec2:    ec2.New(cfg.auth, Regions[cfg.region]),
		s3:     s3.New(cfg.auth, Regions[cfg.region]),
	}, nil
}

func (e *environ) Bootstrap(uploadTools bool) error {
	log.Printf("environs/ec2: bootstrapping environment %q", e.name)
	_, err := e.loadState()
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if err != nil && s3ErrorStatusCode(err) != 404 {
		return err
	}

	if uploadTools {
		err := environs.UploadTools(e)
		if err != nil {
			return err
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
		return err
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
	for a := longAttempt.start(); len(addrs) == 0 && a.next(); {
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

func (e *environ) StartInstance(machineId int, info *state.Info) (environs.Instance, error) {
	log.Printf("environs/ec2: starting machine %d in %q", machineId, e.name)
	return e.startInstance(machineId, info, false)
}

func (e *environ) userData(machineId int, info *state.Info, master bool) ([]byte, error) {
	cfg := &machineConfig{
		provisioner:        master,
		zookeeper:          master,
		stateInfo:          info,
		instanceIdAccessor: "$(curl http://169.254.169.254/1.0/meta-data/instance-id)",
		providerType:       "ec2",
		origin:             e.config.origin,
		machineId:          fmt.Sprint(machineId),
	}

	if e.config.authorizedKeys == "" {
		var err error
		cfg.authorizedKeys, err = authorizedKeys(e.config.authorizedKeysPath)
		if err != nil {
			return nil, fmt.Errorf("cannot get ssh authorized keys: %v", err)
		}
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
	image, err := FindInstanceSpec(DefaultInstanceConstraint)
	if err != nil {
		return nil, fmt.Errorf("cannot find image: %v", err)
	}
	userData, err := e.userData(machineId, info, master)
	if err != nil {
		return nil, err
	}
	groups, err := e.setUpGroups(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var instances *ec2.RunInstancesResp

	for a := shortAttempt.start(); a.next(); {
		instances, err = e.ec2.RunInstances(&ec2.RunInstances{
			ImageId:        image.ImageId,
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
	resp, err := e.ec2.Instances(nil, filter)
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
	for a := shortAttempt.start(); a.next(); {
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

func (e *environ) Destroy(insts []environs.Instance) error {
	log.Printf("environs/ec2: destroying environment %q", e.name)
	// Try to find all the instances in the environ's group.
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", e.groupName())
	resp, err := e.ec2.Instances(nil, filter)
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
	err = e.deleteState()
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
	for a := shortAttempt.start(); a.next(); {
		_, err = e.ec2.TerminateInstances(ids)
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
		_, err = e.ec2.TerminateInstances([]string{id})
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
	resp, err := e.ec2.CreateSecurityGroup(name, "juju group")
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return zeroGroup, err
	}

	want := newPermSet(perms)
	var have permSet
	if err == nil {
		g = resp.SecurityGroup
	} else {
		resp, err := e.ec2.SecurityGroups(ec2.SecurityGroupNames(name), nil)
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
		_, err := e.ec2.RevokeSecurityGroup(g, revoke.ipPerms())
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
		_, err := e.ec2.AuthorizeSecurityGroup(g, add.ipPerms())
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
