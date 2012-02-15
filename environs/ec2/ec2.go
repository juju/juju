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

func (e *environ) Bootstrap() (*state.Info, error) {
	_, err := e.loadState()
	if err == nil {
		return nil, fmt.Errorf("environment is already bootstrapped")
	}
	if s3err, _ := err.(*s3.Error); s3err != nil && s3err.StatusCode != 404 {
		return nil, err
	}
	inst, err := e.startInstance(0, nil, true)
	if err != nil {
		return nil, fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = e.saveState(&bootstrapState{
		ZookeeperInstances: []string{inst.Id()},
	})
	if err != nil {
		// ignore error on StopInstance because the previous error is
		// more important.
		e.StopInstances([]environs.Instance{inst})
		return nil, err
	}
	// TODO wait for the DNS name of the instance to appear.
	// This will happen in a later CL.

	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to use S3 to make sure that only one succeeds.
	// Perhaps consider using SimpleDB for state storage
	// which would enable that possibility.
	return &state.Info{[]string{inst.DNSName() + zkPortSuffix}}, nil
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
func (e *environ) startInstance(machineId int, info *state.Info, master bool) (environs.Instance, error) {
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
	if err != nil && !hasCode(err, "InvalidInstanceId.NotFound") {
		return err
	}

	// Delete the machine group for each instance.
	// We could get the instance data for each instance
	// in one request before doing the group deletion,
	// but then we would have to cope with a possibly partial result.
	prefix := e.groupName() + "-"
	p := newParallel(maxReqs)
	for _, inst := range insts {
		inst := inst
		p.do(func() error {
			resp, err := e.ec2.Instances([]string{inst.Id()}, nil)
			if err != nil {
				return fmt.Errorf("cannot get info about instance %q: %v", inst.Id(), err)
			}
			if len(resp.Reservations) != 1 {
				return fmt.Errorf("unexpected number of instances found, expected 1 got %d", len(resp.Reservations))
			}
			for _, g := range resp.Reservations[0].SecurityGroups {
				if strings.HasPrefix(g.Name, prefix) {
					return e.delGroup(g)
				}
			}
			return nil
		})
	}
	return p.wait()
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
	if err != nil && !hasCode(err, "InvalidInstance.NotFound") {
		return err
	}
	err = e.deleteState()
	if err != nil {
		return err
	}
	err = e.deleteSecurityGroups()
	if err != nil {
		return err
	}
	return nil
}

// delGroup deletes a security group, retrying if it is in use
// (something that will happen for quite a while after an
// environment has been destroyed)
func (e *environ) delGroup(g ec2.SecurityGroup) error {
	_, err := e.ec2.DeleteSecurityGroup(g)
	if err == nil || hasCode("InvalidGroup.NotFound")(err) {
		return nil
	}
	return fmt.Errorf("cannot delete juju security group %q: %v", g.Name, err)
}

// deleteSecurityGroups deletes all the security groups
// associated with the environ.
func (e *environ) deleteSecurityGroups() error {
	// destroy security groups in parallel as we can have
	// many of them.
	p := newParallel(maxReqs)

	p.do(func() error {
		return e.delGroup(ec2.SecurityGroup{Name: e.groupName()})
	})

	resp, err := e.ec2.SecurityGroups(nil, nil)
	if err != nil {
		return fmt.Errorf("cannot list security groups: %v", err)
	}

	prefix := e.groupName() + "-"
	for _, g := range resp.Groups {
		if strings.HasPrefix(g.Name, prefix) {
			p.do(func() error {
				return e.delGroup(g.SecurityGroup)
			})
		}
	}

	return p.wait()
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
	jujuGroup := ec2.SecurityGroup{Name: e.groupName()}
	jujuMachineGroup := ec2.SecurityGroup{Name: e.machineGroupName(machineId)}

	// Create the provider group.
	_, err := e.ec2.CreateSecurityGroup(jujuGroup.Name, "juju group for "+e.name)
	// If the group already exists, we don't mind.
	if err != nil && !hasCode(err, "InvalidGroup.Duplicate") {
		return nil, fmt.Errorf("cannot create juju security group: %v", err)
	}
	_, err = e.ec2.AuthorizeSecurityGroup(jujuGroup, []ec2.IPPerm{
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
	// If the permission has already been granted we don't mind.
	// TODO is it a problem if the group has more permissions than we want?
	if err != nil && !hasCode(err, "InvalidPermission.Duplicate") {
		return nil, fmt.Errorf("cannot authorize security group: %v", err)
	}

	// Create the machine-specific group, but first try to delete it, since it
	// can have the wrong firewall setup
	_, err = e.ec2.DeleteSecurityGroup(jujuMachineGroup)
	if err != nil && !hasCode(err, "InvalidGroup.NotFound") {
		return nil, fmt.Errorf("cannot delete old security group %q: %v", jujuMachineGroup.Name, err)
	}

	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	_, err = e.ec2.CreateSecurityGroup(jujuMachineGroup.Name, descr)
	if err != nil {
		return nil, fmt.Errorf("cannot create machine group %q: %v", jujuMachineGroup.Name, err)
	}

	return []ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil
}

// hasCode true if the provided error has the given ec2 error code.
func hasCode(err error, code string) bool {
	ec2err, _ := err.(*ec2.Error)
	return ec2err != nil && ec2err.Code == code
}

// hasCode true if the provided error has the given ec2 error code.
func hasCode(err error, code string) bool {
	ec2err, _ := err.(*ec2.Error)
	return ec2err != nil && ec2err.Code == code
}
