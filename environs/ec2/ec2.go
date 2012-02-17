package ec2

import (
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/state"
	"sort"
	"sync"
)

const zkPort = 2181
var zkPortSuffix = fmt.Sprintf(":%d", zkPort)

const maxReqs = 20 // maximum concurrent ec2 requests

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
	if len(insts) == 0 {
		return nil
	}
	ids := make([]string, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*instance).InstanceId
	}
	_, err := e.ec2.TerminateInstances(ids)
	// If the instance is already gone, that's fine with us.
	if err != nil && ec2ErrCode(err) != "InvalidInstanceId.NotFound" {
		return err
	}
	return nil
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
				inst := & r.Instances[k]
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
	hasId := make(map[string]bool)
	for _, r := range resp.Reservations {
		for _, inst := range r.Instances {
			ids = append(ids, inst.InstanceId)
			hasId[inst.InstanceId] = true
		}
	}

	// Then add any instances we've been told about
	// but haven't yet shown up in the instance list.
	for _, inst := range insts {
		id := inst.Id()
		if !hasId[id] {
			ids = append(ids, id)
			hasId[id] = true
		}
	}
	if len(ids) > 0 {
		_, err = e.ec2.TerminateInstances(ids)
	}
	// If the instance doesn't exist, we don't care
	if err != nil && ec2ErrCode(err) != "InvalidInstance.NotFound" {
		return err
	}
	err = e.deleteState()
	if err != nil {
		return err
	}
	return nil
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
	return []ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil
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

// ensureGroup tries to ensure that a security group exists with the given
// name and permissions. If the group does not exist, it will be created
// with the given description. It returns the group.
func (e *environ) ensureGroup(name, descr string, perms []ec2.IPPerm) (g ec2.SecurityGroup, err error) {
	resp, err := e.ec2.CreateSecurityGroup(name, descr)
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return
	}

	if err != nil {
		// The group already exists, so check to see if it already
		// has the required permissions. If it does, we need
		// do nothing. While checking for the required permissions
		// is quite involved, waiting for a group to be able to be
		// deleted can take more than 2 minutes, so it's worth it.
		f := ec2.NewFilter()
		f.Add("group-name", name)
		gresp, err := e.ec2.SecurityGroups(nil, f)
		if err != nil {
			return g, err
		}
		if len(gresp.Groups) != 1 {
			return g, fmt.Errorf("unexpected number of groups found; expected 1 got %d", len(gresp.Groups))
		}
		if samePerms(gresp.Groups[0].IPPerms, perms)  {
			// TODO the description might not match, but do we care?
			return gresp.Groups[0].SecurityGroup, nil
		}

		// Delete the group so that we can recreate it with the correct permissions.
		// TODO we could modify the permissions instead of deleting the group.
		// TODO repeat until the group is not in use
		_, err = e.ec2.DeleteSecurityGroup(ec2.SecurityGroup{Name: name})
		if err != nil && ec2ErrCode(err) != "InvalidGroup.InUse" {
			return g, err
		}
		resp, err = e.ec2.CreateSecurityGroup(name, descr)
		if err != nil {
			return g, err
		}
	}

	g = resp.SecurityGroup
	_, err = e.ec2.AuthorizeSecurityGroup(g, perms)
	if err != nil {
		return g, err
	}
	return g, nil
}

// samePerms returns true if p0 and p1 represent the
// same set of permissions.
// It mutates the contents of p0 and p1 to do the check.
func samePerms(p0, p1 []ec2.IPPerm) bool {
	if len(p0) != len(p1) {
		return false
	}
	canonPerms(p0)
	canonPerms(p1)
	for i, p := range p0 {
		if !eqPerm(p, p1[i]) {
			return false
		}
	}
	return true
}

// eqPerm returns true if the canonicalized permissions
// p0 and p1 are identical (ignoring OwnerId and Id fields
// in source groups).
func eqPerm(p0, p1 ec2.IPPerm) bool {
	same := p0.Protocol == p1.Protocol &&
		p0.FromPort == p1.FromPort &&
		p0.ToPort == p1.ToPort &&
		len(p0.SourceIPs) == len(p1.SourceIPs) &&
		len(p0.SourceGroups) == len(p1.SourceGroups)
	if !same {
		return false
	}
	for i, ip := range p0.SourceIPs {
		if ip != p1.SourceIPs[i] {
			return false
		}
	}
	for i, g := range p0.SourceGroups {
		if g.Name != p1.SourceGroups[i].Name {
			return false
		}
	}
	return true
}

// canonPerms canonicalizes a set of IPPerms by sorting them and the
// slices inside them.
func canonPerms(ps []ec2.IPPerm) {
	// TODO if a permission has a source group owned by a different owner but
	// with the same name, then it will compare equal. The only way of getting
	// our own owner id is by creating a security group and looking at that.
	for _, p := range ps {
		sort.Strings(p.SourceIPs)
		sort.Sort(groupSlice(p.SourceGroups))
	}
	sort.Sort(permSlice(ps))
}

type permSlice []ec2.IPPerm
func (p permSlice) Less(i, j int) bool {
	p0, p1 := p[i], p[j]
	if p0.Protocol != p1.Protocol {
		return p0.Protocol < p1.Protocol
	}
	if p0.FromPort != p1.FromPort {
		return p0.FromPort < p1.FromPort
	}
	for i, ip0 := range p0.SourceIPs {
		if i >= len(p1.SourceIPs) {
			return false
		}
		ip1 := p1.SourceIPs[i]
		if ip0 != ip1 {
			return ip0 < ip1
		}
	}
	if len(p0.SourceIPs) < len(p1.SourceIPs) {
		return true
	}
	for i, g0 := range p0.SourceGroups {
		if i >= len(p1.SourceGroups) {
			return false
		}
		g1 := p1.SourceGroups[i]
		// ignore Id and OwnerId because they will not be set
		// in the perms passed to ensureGroup.
		if g0.Name != g1.Name {
			return g0.Name < g1.Name
		}
	}
	return len(p0.SourceGroups) < len(p1.SourceGroups)
}
func (p permSlice) Len() int {
	return len(p)
}
func (p permSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type groupSlice []ec2.UserSecurityGroup
func (g groupSlice) Less(i, j int) bool {
	return g[i].Name < g[j].Name
}
func (g groupSlice) Len() int {
	return len(g)
}
func (g groupSlice) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}
