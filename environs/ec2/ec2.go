package ec2

import (
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/environs"
)

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var _ environs.EnvironProvider = environProvider{}

type environ struct {
	name   string
	config *providerConfig
	ec2    *ec2.EC2
}

var _ environs.Environ = (*environ)(nil)

type instance struct {
	*ec2.Instance
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
	return &environ{
		name:   name,
		config: cfg,
		ec2:    ec2.New(cfg.auth, Regions[cfg.region]),
	}, nil
}

func (e *environ) StartInstance(machineId int) (environs.Instance, error) {
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

func (e *environ) Instances() ([]environs.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")

	resp, err := e.ec2.Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	var insts []environs.Instance
	for i := range resp.Reservations {
		r := &resp.Reservations[i]
		for j := range r.Instances {
			insts = append(insts, &instance{&r.Instances[j]})
		}
	}
	return insts, nil
}

func (e *environ) Destroy() error {
	insts, err := e.Instances()
	if err != nil {
		return err
	}
	return e.StopInstances(insts)
}

func (e *environ) machineGroupName(machineId int) string {
	return fmt.Sprintf("%s-%s", e.groupName(), machineId)
}

func (e *environ) groupName() string {
	return "juju-" + e.name
}

// setUpGroups ensures that the juju group is in the machine launch groups.
//
// Machines launched by juju are tagged with a group so they
// can be distinguished from other machines that might be running
// on an EC2 account. This group can be specified explicitly or
// implicitly defined by the environment name. In addition, a
// specific machine security group is created for each machine,
// so that its firewall rules can be configured per machine.
//
// setUpGroups returns a slice of the group names used.
func (e *environ) setUpGroups(machineId int) ([]string, error) {
	groups, err := e.ec2.SecurityGroups(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get security groups: %v", err)
	}
	jujuGroupName := e.groupName()
	jujuMachineGroupName := e.machineGroupName(machineId)

	hasJujuGroup := false
	hasJujuMachineGroup := false

	for _, g := range groups.SecurityGroups {
		switch g.GroupName {
		case jujuGroupName:
			hasJujuGroup = true
		case jujuMachineGroupName:
			hasJujuMachineGroup = true
		}
	}

	// Create the provider group if doesn't exist.
	if !hasJujuGroup {
		_, err := e.ec2.CreateSecurityGroup(jujuGroupName, "juju group for "+e.name)
		if err != nil {
			return nil, fmt.Errorf("cannot create juju security group: %v", err)
		}
		// we need to get the group to pick up the owner id for auth.
		groups, err := e.ec2.SecurityGroups([]string{jujuGroupName}, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot re-get security groups: %v", err)
		}
		if len(groups.SecurityGroups) != 1 {
			return nil, fmt.Errorf("expected 1 match for juju security group, got %d", len(groups.SecurityGroups))
		}
	}

	// Create the machine-specific group, but first see if there's
	// one already existing from a previous machine launch;
	// if so, delete it, since it can have the wrong firewall setup
	if hasJujuMachineGroup {
		_, err := e.ec2.DeleteSecurityGroup(jujuMachineGroupName)
		if err != nil {
			return nil, fmt.Errorf("cannot delete old security group %q: %v", jujuMachineGroupName, err)
		}
	}
	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	_, err = e.ec2.CreateSecurityGroup(jujuMachineGroupName, descr)

	return []string{jujuGroupName, jujuMachineGroupName}, nil
}
