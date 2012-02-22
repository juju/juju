package ec2

import (
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/state"
	"sync"
)

const zkPort = 2181

var zkPortSuffix = fmt.Sprintf(":%d", zkPort)

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var _ environs.EnvironProvider = environProvider{}

type environ struct {
	name             string
	config           *providerConfig
	ec2              *ec2.EC2
	s3               *s3.S3
	checkBucket      sync.Once
	checkBucketError error
}

var _ environs.Environ = (*environ)(nil)

type instance struct {
	*ec2.Instance
}

func (inst *instance) String() string {
	return inst.Id()
}

var _ environs.Instance = (*instance)(nil)

func (inst *instance) Id() string {
	return inst.InstanceId
}

func (inst *instance) DNSName() string {
	return inst.Instance.DNSName
}

func (environProvider) Open(name string, config interface{}) (e environs.Environ, err error) {
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

func (e *environ) Bootstrap() error {
	_, err := e.loadState()
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if s3err, _ := err.(*s3.Error); s3err != nil && s3err.StatusCode != 404 {
		return err
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
	resp, err := e.ec2.Instances(st.ZookeeperInstances, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot list instances: %v", err)
	}
	var insts []environs.Instance
	for i := range resp.Reservations {
		r := &resp.Reservations[i]
		for j := range r.Instances {
			insts = append(insts, &instance{&r.Instances[j]})
		}
	}

	addrs := make([]string, len(insts))
	for i, inst := range insts {
		// TODO make this poll until the DNSName becomes available.
		addr := inst.DNSName()
		if addr == "" {
			return nil, fmt.Errorf("zookeeper instance %q does not yet have a DNS address", inst.Id())
		}
		addrs[i] = addr + zkPortSuffix
	}
	return &state.Info{Addrs: addrs}, nil
}

func (e *environ) StartInstance(machineId int, info *state.Info) (environs.Instance, error) {
	return e.startInstance(machineId, info, false)
}

// startInstance is the internal version of StartInstance, used by Bootstrap
// as well as via StartInstance itself. If master is true, a bootstrap
// instance will be started.
func (e *environ) startInstance(machineId int, _ *state.Info, master bool) (environs.Instance, error) {
	image, err := FindImageSpec(DefaultImageConstraint)
	if err != nil {
		return nil, fmt.Errorf("cannot find image: %v", err)
	}
	groups, err := e.setUpGroups(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	instances, err := e.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        image.ImageId,
		MinCount:       1,
		MaxCount:       1,
		UserData:       nil,
		InstanceType:   "m1.small",
		SecurityGroups: groups,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instances.Instances) != 1 {
		return nil, fmt.Errorf("expected 1 started instance, got %d", len(instances.Instances))
	}
	return &instance{&instances.Instances[0]}, nil
}

func (e *environ) StopInstances(insts []environs.Instance) error {
	ids := make([]string, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*instance).InstanceId
	}
	return e.terminateInstances(ids)
}

func (e *environ) Instances(ids []string) ([]environs.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]environs.Instance, len(ids))

	// TODO make a series of requests to cope with eventual consistency.
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", e.groupName())
	filter.Add("instance-id", ids...)
	resp, err := e.ec2.Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	// For each requested id, add it to the returned instances
	// if we find it in the response.
	n := 0
	for i, id := range ids {
		if insts[i] != nil {
			continue
		}
		for j := range resp.Reservations {
			r := &resp.Reservations[j]
			for k := range r.Instances {
				inst := &r.Instances[k]
				if inst.InstanceId == id {
					insts[i] = &instance{inst}
					n++
				}
			}
		}
	}
	if n == 0 {
		return nil, environs.ErrMissingInstance
	}
	if n < len(ids) {
		return insts, environs.ErrMissingInstance
	}
	return insts, err
}

func (e *environ) Destroy(insts []environs.Instance) error {
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

	// Then add any instances we've been told about
	// but haven't yet shown up in the instance list.
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
	_, err := e.ec2.TerminateInstances(ids)
	if err == nil || ec2ErrCode(err) != "InvalidInstanceID.NotFound" {
		return err
	}
	var firstErr error
	// If we get a NotFound error, it means that no instances have been
	// terminated, so try them one by one, ignoring NotFound errors.
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
	jujuGroup, err := e.ensureGroup(e.groupName(), "juju group for "+e.name,
		[]ec2.IPPerm{
			// TODO delete this authorization when we can do
			// the zookeeper ssh tunnelling.
			{
				Protocol:  "tcp",
				FromPort:  zkPort,
				ToPort:    zkPort,
				SourceIPs: []string{"0.0.0.0/0"},
			},
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
	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	jujuMachineGroup, err := e.ensureGroup(e.machineGroupName(machineId), descr, nil)
	if err != nil {
		return nil, err
	}
	return []ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil
}

// zg holds the zero security group.
var zg ec2.SecurityGroup

// ensureGroup tries to ensure that a security group exists with the given
// name and permissions. If the group already exists, its permissions
// will be changed accordingly. If the group does not exist, it will be created
// with the given description. It returns the group.
func (e *environ) ensureGroup(name, descr string, perms []ec2.IPPerm) (g ec2.SecurityGroup, err error) {
	resp, err := e.ec2.CreateSecurityGroup(name, descr)
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return zg, err
	}

	want := newPermSet(perms)
	var have permSet
	if err == nil {
		g = resp.SecurityGroup
	} else {
		resp, err := e.ec2.SecurityGroups(ec2.SecurityGroupNames(name), nil)
		if err != nil {
			return zg, err
		}
		// TODO do we mind if the old group has the wrong description?
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
			return zg, fmt.Errorf("cannot revoke security group: %v", err)
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
			return zg, fmt.Errorf("cannot authorize securityGroup: %v", err)
		}
	}
	return g, nil
}

// ipPerm represents a permission for a group or an ip address range
// to access the given range of ports. Only one of groupId or ipAddr
// should be non-empty.
type ipPerm struct {
	protocol string
	fromPort int
	toPort   int
	groupId  string
	ipAddr   string
}

type permSet map[ipPerm]bool

// newPermSet returns a set of all the permissions in the
// given slice of IPPerms. It ignores the name and owner
// id in source groups, using group ids only.
func newPermSet(ps []ec2.IPPerm) permSet {
	m := make(permSet)
	for _, p := range ps {
		ipp := ipPerm{
			protocol: p.Protocol,
			fromPort: p.FromPort,
			toPort:   p.ToPort,
		}
		for _, g := range p.SourceGroups {
			ipp.groupId = g.Id
			m[ipp] = true
		}
		ipp.groupId = ""
		for _, ip := range p.SourceIPs {
			ipp.ipAddr = ip
			m[ipp] = true
		}
	}
	return m
}

// ipPerms returns the given set of permissions
// as a slice of IPPerms.
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
