package ec2

import (
	"errors"
	"fmt"
	"launchpad.net/goamz/ec2"
	"launchpad.net/goamz/s3"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/state"
	"strings"
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
	if inst.DNSName() == "" {
		noDNS := errors.New("no dns addr")
		longAttempt.do(
			func(err error) bool {
				return err == noDNS || hasCode("InvalidInstance.NotFound")(err)
			},
			func() error {
				insts, err := e.Instances([]string{inst.Id()})
				if err != nil {
					return err
				}
				inst = insts[0]
				if inst.DNSName() == "" {
					return noDNS
				}
				return nil
			},
		)
		if err != nil {
			if err == noDNS {
				return nil, fmt.Errorf("timed out trying to get bootstrap instance DNS address", inst.Id())
			}
			return nil, fmt.Errorf("cannot find just-started bootstrap instance %q: %v", inst.Id(), err)
		}
	}

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
	insts, err := e.Instances(st.ZookeeperInstances)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, len(insts))
	for i, inst := range insts {
		addr := inst.DNSName()
		if addr == "" {
			return nil, fmt.Errorf("zookeeper instance %q does not yet have a DNS address", inst.Id())
		}
		addrs[i] = addr + zkPortSuffix
	}
	return &state.Info{addrs}, nil
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
	instances, err := e.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        image.ImageId,
		MinCount:       1,
		MaxCount:       1,
		UserData:       userData,
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
	err := shortAttempt.do(hasCode("InvalidInstanceID.NotFound"), func() error {
		_, err := e.ec2.TerminateInstances(ids)
		return err
	})
	// If the instance is already gone, that's fine with us.
	if err != nil && !hasCode("InvalidInstanceId.NotFound")(err) {
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
			var resp *ec2.InstancesResp
			err := shortAttempt.do(hasCode("InvalidInstanceID.NotFound"), func() error {
				var err error
				resp, err = e.ec2.Instances([]string{inst.Id()}, nil)
				return err
			})
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
	// make a series of requests to cope with eventual consistency.
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
							insts[i] = &instance{&inst}
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

	// Then add any instances we've been told aboutbut haven't yet shown
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
	if err != nil && !hasCode("InvalidInstance.NotFound")(err) {
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
	err := longAttempt.do(hasCode("InvalidGroup.InUse"), func() error {
		_, err := e.ec2.DeleteSecurityGroup(g)
		return err
	})
	if err == nil || hasCode("InvalidGroup.NotFound")(err) {
		return nil
	}
	return fmt.Errorf("cannot delete juju security group: %v", err)
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
			g := g
			p.do(func() error {
				return e.delGroup(g.SecurityGroup)
			})
		}
	}

	return p.wait()
}

// machineGroupName returns the name of the security group
// that will be assigned to an instance with the given machine id.
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
	jujuGroup := ec2.SecurityGroup{Name: e.groupName()}
	jujuMachineGroup := ec2.SecurityGroup{Name: e.machineGroupName(machineId)}

	// Create the provider group.
	_, err := e.ec2.CreateSecurityGroup(jujuGroup.Name, "juju group for "+e.name)
	// If the group already exists, we don't mind.
	if err != nil && !hasCode("InvalidGroup.Duplicate")(err) {
		return nil, fmt.Errorf("cannot create juju security group: %v", err)
	}
	err = shortAttempt.do(hasCode("InvalidGroup.NotFound"), func() error {
		_, err := e.ec2.AuthorizeSecurityGroup(jujuGroup, []ec2.IPPerm{
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
		return err
	})
	// If the permission has already been granted we don't mind.
	// TODO is it a problem if the group has more permissions than we want?
	if err != nil && !hasCode("InvalidPermission.Duplicate")(err) {
		return nil, fmt.Errorf("cannot authorize security group: %v", err)
	}

	// Create the machine-specific group, but first try to delete it, since it
	// can have the wrong firewall setup
	err = shortAttempt.do(hasCode("InvalidGroup.InUse"), func() error {
		_, err := e.ec2.DeleteSecurityGroup(jujuMachineGroup)
		return err
	})
	if err != nil && !hasCode("InvalidGroup.NotFound")(err) {
		return nil, fmt.Errorf("cannot delete old security group %q: %v", jujuMachineGroup.Name, err)
	}

	descr := fmt.Sprintf("juju group for %s machine %d", e.name, machineId)
	err = shortAttempt.do(hasCode("InvalidGroup.Duplicate"), func() error {
		_, err := e.ec2.CreateSecurityGroup(jujuMachineGroup.Name, descr)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("cannot create machine group %q: %v", jujuMachineGroup.Name, err)
	}

	return []ec2.SecurityGroup{jujuGroup, jujuMachineGroup}, nil
}
