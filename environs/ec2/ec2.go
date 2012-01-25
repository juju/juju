package ec2

import (
	"fmt"
	"io"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"sync"
)

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

func (e *environ) findBootstrapMachine() (environs.Instance, error) {
	_, err := e.Instances()
	if err != nil {
		return nil, fmt.Errorf("cannot list machines: %v", err)
	}
	return nil, nil
}

func (e *environ) zookeeperAddrs() ([]string, error) {
	return nil, fmt.Errorf("not implemented zookeeper addtrs")
}

func (e *environ) Bootstrap() error {
	addrs, err := e.zookeeperAddrs()
	if err != nil {
		return fmt.Errorf("cannot get zookeeper machine addresses: %v", err)
	}
	if len(addrs) > 0 {
		return fmt.Errorf("environment is already bootstrapped")
	}
	_, err = e.startInstance(0, true)
	return err
}

func (e *environ) StartInstance(machineId int) (environs.Instance, error) {
	if machineId <= 0 {
		panic(fmt.Errorf("StartInstance:invalid machine id: %d", machineId))
	}
	return e.startInstance(machineId, false)
}

// startInstance is the internal version of StartInstance, used by Bootstrap
// as well as via StartInstance itself. If master is true, a bootstrap
// instance will be started.
func (e *environ) startInstance(machineId int, master bool) (environs.Instance, error) {
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

func (e *environ) makeControlBucket() error {
	e.checkBucket.Do(func() {
		b := e.controlBucket()
		// As bucket LIST isn't implemented for the s3test server yet,
		// we try to get an object from the control bucket
		// and determine whether the bucket exists using the resulting
		// error message.
		r, testErr := b.GetReader("testing123")
		if testErr == nil {
			r.Close()
			return
		}
		if testErr, _ := testErr.(*s3.Error); testErr == nil || testErr.Code != "NoSuchBucket" {
			return
		}
		// The bucket doesn't exist, so try to make it.
		e.checkBucketError = b.PutBucket(s3.Private)
	})
	return e.checkBucketError
}

func (e *environ) PutFile(file string, r io.Reader, length int64) error {
	if err := e.makeControlBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := e.controlBucket().PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot read file %q from control bucket: %v", file)
	}
	return nil
}

func (e *environ) GetFile(file string) (io.ReadCloser, error) {
	return e.controlBucket().GetReader(file)
}

func (e *environ) RemoveFile(file string) error {
	return e.controlBucket().Del(file)
}

func (e *environ) controlBucket() *s3.Bucket {
	return e.s3.Bucket(e.config.controlBucket)
}

func (e *environ) Destroy() error {
	// TODO should we ignore error from this or give a warning or what?
	e.controlBucket().DelBucket()

	insts, err := e.Instances()
	if err != nil {
		return err
	}
	return e.StopInstances(insts)
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
	groups, err := e.ec2.SecurityGroups([]ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil)
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
