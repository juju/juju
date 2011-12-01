package ec2

import (
	"errors"
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"launchpad.net/juju/go/juju"
	"os"
	"strings"
)

var notFound = errors.New("not found")

type conn struct {
	name   string
	config providerConfig
	ec2    *ec2.EC2
}

type Instance struct {
	*ec2.Instance
}

func (m *Instance) DNSName() string {
	return m.Instance.DNSName
}

func (m *Instance) Id() string {
	return m.InstanceId
}

func (m *Instance) PrivateDNSName() string {
	return m.Instance.PrivateDNSName
}

func (provider) Open(name string, config interface{}) (e juju.Environ, err error) {
	if strings.Index(name, "-") != -1 {
		return nil, errors.New("invalid environment name")
	}
	cfg := config.(*providerConfig)
	if cfg.accessKey == "" {
		cfg.accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if cfg.secretKey == "" {
		cfg.secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if cfg.accessKey == "" {
		return nil, errors.New("cannot find ec2 access key in environment")
	}
	if cfg.secretKey == "" {
		return nil, errors.New("cannot find ec2 secret access key in environment")
	}
	auth := aws.Auth{
		AccessKey: cfg.accessKey,
		SecretKey: cfg.secretKey,
	}
	return &conn{
		name:   name,
		ec2:    ec2.New(auth, cfg.region),
		config: *cfg,
	}, err
}

// Bootstrap implements juju.Environ.Bootstrap
func (c *conn) Bootstrap() error {
	return nil
}

// Destroy implements juju.Environ.Destroy
func (c *conn) Destroy() error {
	ms, err := c.Instances()
	if err == notFound {
		return errors.New("not bootstrapped")
	}
	if err != nil {
		return err
	}
	return c.StopInstances(ms)
}

// StartInstance implements juju.Environ.StartInstance
func (c *conn) StartInstance(machineId int) (juju.Instance, error) {
	image, err := c.FindImageSpec(DefaultImageConstraint)
	if err != nil {
		return nil, fmt.Errorf("cannot find image: %v", err)
	}
	groups, err := c.setUpGroups(machineId)
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	instances, err := c.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        image.ImageId,
		MinCount:       1,
		MaxCount:       1,
		SecurityGroups: groups,
		UserData:       nil,
		InstanceType:   c.config.defaultInstanceType,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	}
	if len(instances.Instances) != 1 {
		return nil, fmt.Errorf("expected 1 started instance, got %d", len(instances.Instances))
	}
	return &Instance{&instances.Instances[0]}, nil
}

// StopInstances implements juju.Environ.StopInstances
func (c *conn) StopInstances(ms []juju.Instance) error {
	if len(ms) == 0 {
		return nil
	}
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.(*Instance).InstanceId
	}
	_, err := c.ec2.TerminateInstances(names)
	return err
}

// groupName returns the name of the EC2 group which all
// machines in this juju environment will belong to.
func (c *conn) groupName() string {
	return "juju-" + c.name
}

// machineGroupName returns the name of the EC2 group which
// a particular machine will be uniquely assigned to.
func (c *conn) machineGroupName(machineId int) string {
	return fmt.Sprintf("%s-%d", c.groupName(), machineId)
}

// setUpGroups ensures that the juju group is in the machine launch groups.
//
// Instances launched by juju are tagged with a group so they
// can be distinguished from other machines that might be running
// on an EC2 account. This group is defined by the environment name.
// In addition, a specific machine security group is created for each machine,
// so that its firewall rules can be configured per machine.
//
// setUpGroups returns a slice of the group names used.
func (c *conn) setUpGroups(machineId int) ([]string, error) {
	groups, err := c.ec2.SecurityGroups(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get security groups: %v", err)
	}
	jujuGroupName := c.groupName()
	jujuInstanceGroupName := c.machineGroupName(machineId)

	hasJujuGroup := false
	hasJujuInstanceGroup := false

	for _, g := range groups.SecurityGroups {
		switch g.GroupName {
		case jujuGroupName:
			hasJujuGroup = true
		case jujuInstanceGroupName:
			hasJujuInstanceGroup = true
		}
	}

	// Create the provider group if doesn't exist.
	if !hasJujuGroup {
		_, err := c.ec2.CreateSecurityGroup(jujuGroupName, "juju group for "+c.name)
		if err != nil {
			return nil, fmt.Errorf("cannot create juju security group: %v", err)
		}
		// we need to get the group to pick up the owner id for auth.
		groups, err := c.ec2.SecurityGroups([]string{jujuGroupName}, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot re-get security groups: %v", err)
		}
		if len(groups.SecurityGroups) != 1 {
			return nil, fmt.Errorf("expected 1 match for juju security group, got %d", len(groups.SecurityGroups))
		}

		accessGroups := []ec2.SecurityGroupId{{
			OwnerId:   groups.SecurityGroups[0].OwnerId,
			GroupName: jujuGroupName,
		}}

		// Authorize SSH and access for all protocols for internal traffic.
		perms := []ec2.IPPerm{{
			Protocol:  "tcp",
			FromPort:  22,
			ToPort:    22,
			SourceIPs: []string{"0.0.0.0/0"},
		}, {
			Protocol:     "tcp",
			FromPort:     0,
			ToPort:       65535,
			SourceGroups: accessGroups,
		}, {
			Protocol:     "udp",
			FromPort:     0,
			ToPort:       65535,
			SourceGroups: accessGroups,
		}, {
			Protocol:     "icmp",
			FromPort:     -1,
			ToPort:       -1,
			SourceGroups: accessGroups,
		}}
		_, err = c.ec2.AuthorizeSecurityGroup(jujuGroupName, perms)
		if err != nil {
			return nil, fmt.Errorf("cannot authorize internal ZK traffic: %v", err)
		}
	}

	// Create the machine-specific group, but first see if there's
	// one already existing from a previous machine launch;
	// if so, delete it, since it can have the wrong firewall setup
	if hasJujuInstanceGroup {
		_, err := c.ec2.DeleteSecurityGroup(jujuInstanceGroupName)
		if err != nil {
			return nil, fmt.Errorf("cannot delete old security group %q: %v", jujuInstanceGroupName, err)
		}
	}
	descr := fmt.Sprintf("juju group for %s machine %s", c.name, machineId)
	_, err = c.ec2.CreateSecurityGroup(jujuInstanceGroupName, descr)

	return []string{jujuGroupName, jujuInstanceGroupName}, nil
}

func (c *conn) Instances() ([]juju.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	filter.Add("group-name", c.groupName())
	resp, err := c.ec2.Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	var m []juju.Instance
	for i := range resp.Reservations {
		r := &resp.Reservations[i]
		for j := range r.Instances {
			m = append(m, &Instance{&r.Instances[j]})
		}
	}
	return m, nil
}
