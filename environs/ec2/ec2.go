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
	names := make([]string, len(insts))
	for i, inst := range insts {
		names[i] = inst.(*instance).InstanceId
	}
	_, err := e.ec2.TerminateInstances(names)
	return err
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
	if err != nil {
		// If the instance doesn't exist, we don't care
		if ec2err, _ := err.(*ec2.Error); ec2err == nil || ec2err.Code == "InvalidInstance.NotFound" {
			return err
		}
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
	jujuGroup := ec2.SecurityGroup{Name: e.groupName()}
	jujuMachineGroup := ec2.SecurityGroup{Name: e.machineGroupName(machineId)}

	f := ec2.NewFilter()
	f.Add("group-name", jujuGroup.Name, jujuMachineGroup.Name)
	groups, err := e.ec2.SecurityGroups(nil, f)
	if err != nil {
		return nil, fmt.Errorf("cannot get security groups: %v", err)
	}

	for _, g := range groups.Groups {
		switch g.Name {
		case jujuGroup.Name:
			jujuGroup = g.SecurityGroup
		case jujuMachineGroup.Name:
			jujuMachineGroup = g.SecurityGroup
		}
	}

	// Create the provider group if doesn't exist.
	if jujuGroup.Id == "" {
		r, err := e.ec2.CreateSecurityGroup(jujuGroup.Name, "juju group for "+e.name)
		if err != nil {
			return nil, fmt.Errorf("cannot create juju security group: %v", err)
		}
		jujuGroup = r.SecurityGroup
	}

	// Create the machine-specific group, but first see if there's
	// one already existing from a previous machine launch;
	// if so, delete it, since it can have the wrong firewall setup
	if jujuMachineGroup.Id != "" {
		_, err := e.ec2.DeleteSecurityGroup(jujuMachineGroup)
		if err != nil {
			return nil, fmt.Errorf("cannot delete old security group %q: %v", jujuMachineGroup.Name, err)
		}
	}
	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	r, err := e.ec2.CreateSecurityGroup(jujuMachineGroup.Name, descr)
	if err != nil {
		return nil, fmt.Errorf("cannot create machine group %q: %v", jujuMachineGroup.Name, err)
	}
	return []ec2.SecurityGroup{jujuGroup, r.SecurityGroup}, nil
}
