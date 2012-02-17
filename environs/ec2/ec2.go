package ec2

import (
	"errors"
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

var shortAttempt = attempt{
	burstTotal: 5e9,
	burstDelay: 0.2e9,
}

var longAttempt = attempt{
	burstTotal: 5e9,
	burstDelay: 0.2e9,
	longTotal:  3 * 60e9,
	longDelay:  5e9,
}

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
	// The DNS address for an instance takes a while to arrive.
	noDNS := errors.New("no dns addr")
	err := longAttempt.do(
		func(err error) bool {
			return err == noDNS || err == environs.ErrMissingInstance
		},
		func() error {
			insts, err := inst.e.Instances([]string{inst.Id()})
			if err != nil {
				return err
			}
			freshInst := insts[0].(*instance).Instance
			if freshInst.DNSName == "" {
				return noDNS
			}
			inst.Instance.DNSName = freshInst.DNSName
			return nil
		},
	)
	if err != nil {
		if err == noDNS {
			return "", fmt.Errorf("timed out trying to get DNS address", inst.Id())
		}
		return "", fmt.Errorf("cannot find instance %q: %v", inst.Id(), err)
	}
	return inst.Instance.DNSName, nil
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
	insts, err := e.Instances(st.ZookeeperInstances)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, len(insts))
	for i, inst := range insts {
		addr, err := inst.DNSName()
		if err != nil {
			return nil, fmt.Errorf("cannot get zookeeper instance DNS address: %v", err)
		}
		addrs[i] = addr + zkPortSuffix
	}
	return &state.Info{Addrs: addrs}, nil
}


func (e *environ) StartInstance(machineId int, info *state.Info) (environs.Instance, error) {
	return e.startInstance(machineId, info, false)
}

func (e *environ) userData(machineId int, info *state.Info, master bool) ([]byte, error) {
	cfg := &machineConfig{
		provisioner:        master,
		zookeeper:          master,
		stateInfo:          info,
		instanceIdAccessor: "$(curl http://169.254.169.254/1.0/meta-data/instance-id)",
		providerType:       "ec2",
		origin:             jujuOrigin{originBranch, "lp:jujubranch"},
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
	image, err := FindImageSpec(DefaultImageConstraint)
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
	err = shortAttempt.do(hasCode("InvalidGroup.NotFound"), func() error {
		var err error
		instances, err = e.ec2.RunInstances(&ec2.RunInstances{
			ImageId:        image.ImageId,
			MinCount:       1,
			MaxCount:       1,
			UserData:       userData,
			InstanceType:   "m1.small",
			SecurityGroups: groups,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instances.Instances) != 1 {
		return nil, fmt.Errorf("expected 1 started instance, got %d", len(instances.Instances))
	}
	return &instance{e, &instances.Instances[0]}, nil
}

func (e *environ) StopInstances(insts []environs.Instance) error {
	if len(insts) == 0 {
		return nil
	}
	ids := make([]string, len(insts))
	for i, inst := range insts {
		ids[i] = inst.(*instance).InstanceId
	}
	err := shortAttempt.do(hasCode("InvalidInstanceID.NotFound"), func() error {
		_, err := e.ec2.TerminateInstances(ids)
		return err
	})
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
	// Make a series of requests to cope with eventual consistency.
	// each request should add more instances to the requested
	// set.
	n := 0
	err := shortAttempt.do(
		func(err error) bool {
			return err == environs.ErrMissingInstance
		},
		func() error {
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
				return environs.ErrMissingInstance
			}
			return nil
		},
	)
	if n == 0 {
		return nil, err
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

	// Then add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range insts {
		id := inst.Id()
		if !hasId[id] {
			ids = append(ids, id)
			hasId[id] = true
		}
	}
	if len(ids) > 0 {
		err = shortAttempt.do(hasCode("InvalidInstance.NotFound"), func() error {
			_, err := e.ec2.TerminateInstances(ids)
			return err
		})
	}
	// If the instance is still not found after waiting around,
	// then it probably really doesn't exist, and we don't care
	// about that.
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

// groupName returns the name of the security group
// that will be assigned to all machines started by the environ.
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
		return nil, fmt.Errorf("cannot ensure juju group: %v", err)
	}

	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	jujuMachineGroup, err := e.ensureGroup(e.machineGroupName(machineId), descr, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot ensure machine group: %v", err)
	}
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

var zg = ec2.SecurityGroup{}

// ensureGroup tries to ensure that a security group exists with the given
// name and permissions. If the group does not exist, it will be created
// with the given description. It returns the group.
func (e *environ) ensureGroup(name, descr string, perms []ec2.IPPerm) (ec2.SecurityGroup, error) {
	resp, err := e.ec2.CreateSecurityGroup(name, descr)
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return zg, err
	}

	var g ec2.SecurityGroup
	if err == nil {
		g = resp.SecurityGroup
	} else {
		var ok bool
		ok, g, err = e.existingGroupOk(name, descr, perms)
		if err != nil {
			return zg, err
		}
		if ok {
			return g, nil
		}
		g, err = e.recreateGroup(name, descr)
		if err != nil {
			return zg, err
		}
	}

	if perms != nil {
		err := shortAttempt.do(hasCode("InvalidGroup.NotFound"), func() error {
			_, err := e.ec2.AuthorizeSecurityGroup(g, perms)
			return err
		})
		if err != nil {
			return zg, err
		}
	}
	return g, nil
}

// We know that a group with the name we want already exists, so
// existingGroupOk checks to see if it already has exactly the required
// permissions. If it does, it returns ok==true and the group.
// While checking for the required permissions is quite involved, waiting to
// be able to delete a group can take more than 2 minutes, so it's worth it.
func (e *environ) existingGroupOk(name, descr string, perms []ec2.IPPerm) (ok bool, g ec2.SecurityGroup, err error) {
	var gresp *ec2.SecurityGroupsResp
	err = shortAttempt.do(hasCode("InvalidGroup.NotFound"), func() error {
		var err error
		gresp, err = e.ec2.SecurityGroups(ec2.SecurityGroupNames(name), nil)
		// TODO remove the below when the ec2test bug is fixed.
		if len(gresp.Groups) == 0 {
			err = &ec2.Error{Code: "InvalidGroup.NotFound"}
		}
		return err
	})
	if err != nil {
		return false, zg, err
	}
	if len(gresp.Groups) != 1 {
		return false, zg, fmt.Errorf("unexpected number of groups found; expected 1 got %d", len(gresp.Groups))
	}
	if samePerms(gresp.Groups[0].IPPerms, perms)  {
		// TODO the description might not match, but do we care?
		return true, gresp.Groups[0].SecurityGroup, nil
	}
	return false, zg, nil
}

// recreateGroup deletes the security group with the given name
// and then creates it again so that it can be given the desired attributes.
func (e *environ) recreateGroup(name, descr string) (ec2.SecurityGroup, error) {
	// TODO we could modify the permissions instead of deleting the group.
	err := longAttempt.do(hasCode("InvalidGroup.InUse"), func() error {
		_, err := e.ec2.DeleteSecurityGroup(ec2.SecurityGroup{Name: name})
		return err
	})
	if err != nil {
		return zg, fmt.Errorf("cannot delete old group %q: %v", name, err)
	}
	var resp *ec2.CreateSecurityGroupResp
	err = shortAttempt.do(hasCode("InvalidGroup.Duplicate"), func() error {
		var err error
		resp, err = e.ec2.CreateSecurityGroup(name, descr)
		return err
	})
	if err != nil {
		return zg, fmt.Errorf("cannot create group %q: %v", name, err)
	}
	return resp.SecurityGroup, nil
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
