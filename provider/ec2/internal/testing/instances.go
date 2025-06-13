// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/errors"
)

// Recognized AWS instance states.
var (
	Pending      = types.InstanceState{Code: aws.Int32(0), Name: "pending"}
	Running      = types.InstanceState{Code: aws.Int32(16), Name: "running"}
	ShuttingDown = types.InstanceState{Code: aws.Int32(32), Name: "shutting-down"}
	Terminated   = types.InstanceState{Code: aws.Int32(16), Name: "terminated"}
	Stopped      = types.InstanceState{Code: aws.Int32(16), Name: "stopped"}
)

// Instance holds a fake ec2 instance
type Instance struct {
	seq int
	// first is set to true until the instance has been marshaled
	// into a response at least once.
	first bool
	// UserData holds the data that was passed to the RunInstances request
	// when the instance was started.
	UserData            []byte
	imageId             string
	reservation         *reservation
	instType            types.InstanceType
	availZone           string
	state               types.InstanceState
	subnetId            string
	vpcId               string
	ifaces              []types.NetworkInterface
	blockDeviceMappings []types.InstanceBlockDeviceMapping
	sourceDestCheck     bool
	tags                []types.Tag
	rootDeviceType      types.DeviceType
	rootDeviceName      string

	iamInstanceProfile *types.IamInstanceProfileSpecification
}

// TerminateInstances implements ec2.Client.
func (srv *Server) TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	resp := &ec2.TerminateInstancesOutput{}
	var insts []*Instance
	for _, id := range in.InstanceIds {
		inst := srv.instances[id]
		if inst == nil {
			return nil, apiError("InvalidInstanceID.NotFound", "no such instance id %q", id)
		}
		insts = append(insts, inst)
	}
	for _, inst := range insts {
		// Delete any attached volumes that are "DeleteOnTermination"
		for _, va := range srv.volumeAttachments {
			if aws.ToString(va.InstanceId) != inst.id() || !aws.ToBool(va.DeleteOnTermination) {
				continue
			}
			delete(srv.volumeAttachments, aws.ToString(va.VolumeId))
			delete(srv.volumes, aws.ToString(va.VolumeId))
		}
		resp.TerminatingInstances = append(resp.TerminatingInstances, inst.terminate())
	}
	return resp, nil
}

func (srv *Server) instance(id string) (*Instance, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	inst, found := srv.instances[id]
	if !found {
		return nil, apiError("InvalidInstanceID.NotFound", "instance %s not found", id)
	}
	return inst, nil
}

// RunInstances implements ec2.Client.
func (srv *Server) RunInstances(ctx context.Context, in *ec2.RunInstancesInput, opts ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	min := aws.ToInt32(in.MinCount)
	max := aws.ToInt32(in.MaxCount)
	if min < 0 || max < 1 {
		return nil, apiError("InvalidParameterValue", "bad values for MinCount or MaxCount")
	}
	if min > max {
		return nil, apiError("InvalidParameterCombination", "MinCount is greater than MaxCount")
	}
	var userData []byte
	if data := aws.ToString(in.UserData); data != "" {
		var err error
		userData, err = b64.DecodeString(data)
		if err != nil {
			return nil, apiError("InvalidParameterValue", "bad UserData value: %v", err)
		}
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	// make sure that form fields are correct before creating the reservation.
	instType := in.InstanceType
	imageId := aws.ToString(in.ImageId)
	availZone := ""
	if in.Placement != nil {
		availZone = aws.ToString(in.Placement.AvailabilityZone)
	}
	if availZone == "" {
		availZone = defaultAvailZone
	}

	var groups []*securityGroup
	if in != nil {
		var err error
		groups, err = srv.groupsByIdOrName(in.SecurityGroupIds, in.SecurityGroups)
		if err != nil {
			return nil, err
		}
	}
	r := srv.newReservation(groups)

	// If the user specifies an explicit subnet id, use it.
	// Otherwise, get a subnet from the default VPC.
	userSubnetId := aws.ToString(in.SubnetId)
	instSubnet := srv.subnets[userSubnetId]
	if instSubnet == nil && userSubnetId != "" {
		return nil, apiError("InvalidSubnetID.NotFound", "subnet %s not found", userSubnetId)
	}
	if userSubnetId == "" {
		instSubnet = srv.getDefaultSubnet()
	}

	ifacesToCreate, limitToOneInstance, err := srv.parseNetworkInterfaces(in.NetworkInterfaces)
	if err != nil {
		return nil, err
	}
	if len(ifacesToCreate) > 0 && userSubnetId != "" {
		return nil, apiError("InvalidParameterCombination", "Network interfaces and an instance-level subnet ID may not be specified on the same request")
	}
	if limitToOneInstance {
		max = 1
	}
	if len(ifacesToCreate) == 0 {
		// No NICs specified, so create a default one to simulate what EC2 does.
		ifacesToCreate = srv.addDefaultNIC(instSubnet)
	}

	resp := &ec2.RunInstancesOutput{}
	resp.ReservationId = aws.String(r.id)
	resp.OwnerId = aws.String(ownerId)

	for i := 0; i < int(max); i++ {
		inst := srv.newInstance(r, instType, imageId, availZone, srv.initialInstanceState)
		// Create any NICs on the instance subnet (if any), and then
		// save the VPC and subnet ids on the instance, as EC2 does.
		inst.ifaces = srv.createNICsOnRun(inst.id(), instSubnet, ifacesToCreate)
		if instSubnet != nil {
			inst.subnetId = aws.ToString(instSubnet.SubnetId)
			inst.vpcId = aws.ToString(instSubnet.VpcId)
		}
		inst.UserData = userData
		inst.blockDeviceMappings = append(inst.blockDeviceMappings,
			srv.createBlockDeviceMappingsOnRun(in.BlockDeviceMappings)...,
		)
		resp.Instances = append(resp.Instances, inst.ec2instance())
	}
	return resp, nil
}

func (srv *Server) AssociateIamInstanceProfile(
	ctx context.Context,
	params *ec2.AssociateIamInstanceProfileInput,
	opts ...func(*ec2.Options),
) (*ec2.AssociateIamInstanceProfileOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	inst, exists := srv.instances[*params.InstanceId]
	if !exists {
		return nil, apiError("InvalidInstanceID.NotFound", "instance %q not found", *params.InstanceId)
	}

	if inst.state.Name != types.InstanceStateNameRunning {
		return nil, apiError("InvalidInstanceStateName.NotRunning", "Instance %q not in a running state", *params.InstanceId)
	}

	inst.iamInstanceProfile = params.IamInstanceProfile

	association := types.IamInstanceProfileAssociation{
		AssociationId: aws.String(fmt.Sprintf("%s-%s", *params.InstanceId, *params.IamInstanceProfile.Name)),
		IamInstanceProfile: &types.IamInstanceProfile{
			Arn: params.IamInstanceProfile.Arn,
			Id:  aws.String("iam-id-1234"),
		},
		InstanceId: params.InstanceId,
		State:      types.IamInstanceProfileAssociationStateAssociated,
	}
	srv.instanceProfileAssociations[*association.AssociationId] = association

	return &ec2.AssociateIamInstanceProfileOutput{
		IamInstanceProfileAssociation: &association,
	}, nil
}

func (srv *Server) DescribeIamInstanceProfileAssociations(
	ctx context.Context,
	params *ec2.DescribeIamInstanceProfileAssociationsInput,
	opts ...func(*ec2.Options),
) (*ec2.DescribeIamInstanceProfileAssociationsOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	associations := []types.IamInstanceProfileAssociation{}
	for _, id := range params.AssociationIds {
		association, exists := srv.instanceProfileAssociations[id]
		if !exists {
			return nil, errors.NotFoundf("instance profile association %s", id)
		}
		associations = append(associations, association)
	}
	return &ec2.DescribeIamInstanceProfileAssociationsOutput{
		IamInstanceProfileAssociations: associations,
	}, nil
}

// DescribeInstances implements ec2.Client.
func (srv *Server) DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	insts := make(map[*Instance]bool)
	for _, id := range in.InstanceIds {
		inst := srv.instances[id]
		if inst == nil {
			return nil, apiError("InvalidInstanceID.NotFound", "instance %q not found", id)
		}
		insts[inst] = true
	}

	var f ec2filter
	if in != nil {
		f = in.Filters
	}
	var reservations []types.Reservation

	for _, r := range srv.reservations {
		var instances []types.Instance
		var groups []types.GroupIdentifier
		for _, g := range r.groups {
			sg := g.ec2SecurityGroup()
			groups = append(groups, types.GroupIdentifier{
				GroupId:   sg.GroupId,
				GroupName: sg.GroupName,
			})
		}
		for _, inst := range r.instances {
			if len(insts) > 0 && !insts[inst] {
				continue
			}
			// make instances in state "shutting-down" to transition
			// to "terminated" first, so we can simulate: shutdown,
			// subsequent refresh of the state with Instances(),
			// terminated.
			if inst.state == ShuttingDown {
				inst.state = Terminated
			}

			ok, err := f.ok(inst)
			if ok {
				instance := inst.ec2instance()
				instance.SecurityGroups = groups
				instances = append(instances, instance)
			} else if err != nil {
				return nil, apiError("InvalidParameterValue", "describe instances: %v", err)
			}
		}
		if len(instances) > 0 {
			reservations = append(reservations, types.Reservation{
				ReservationId: aws.String(r.id),
				OwnerId:       aws.String(ownerId),
				Instances:     instances,
				Groups:        groups,
			})
		}
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: reservations,
	}, nil
}

// SetInitialInstanceState sets the state that any new instances will be started in.
func (srv *Server) SetInitialInstanceState(state types.InstanceState) {
	srv.mu.Lock()
	srv.initialInstanceState = state
	srv.mu.Unlock()
}

// NewInstancesVPC creates n new VPC instances in srv with the given
// instance type, image ID, initial state, and security groups,
// belonging to the given vpcId and subnetId. If any group does not
// already exist, it will be created. NewInstancesVPC returns the ids
// of the new instances.
//
// If vpcId and subnetId are both empty, this call is equivalent to
// calling NewInstances.
func (srv *Server) NewInstancesVPC(vpcId, subnetId string, n int, instType types.InstanceType, imageId string, state types.InstanceState, groups []types.GroupIdentifier) ([]string, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	rgroups := make([]*securityGroup, len(groups))
	for i, group := range groups {
		g := srv.group(group)
		if g == nil {
			return nil, apiError("InvalidGroup.NotFound", "no such group %v", g)
		}
		rgroups[i] = g
	}
	r := srv.newReservation(rgroups)

	ids := make([]string, n)
	for i := 0; i < n; i++ {
		inst := srv.newInstance(r, instType, imageId, defaultAvailZone, state)
		inst.vpcId = vpcId
		inst.subnetId = subnetId
		ids[i] = inst.id()
	}
	return ids, nil
}

// NewInstances creates n new instances in srv with the given instance
// type, image ID, initial state, and security groups. If any group
// does not already exist, it will be created. NewInstances returns
// the ids of the new instances.
func (srv *Server) NewInstances(n int, instType types.InstanceType, imageId string, state types.InstanceState, groups []types.GroupIdentifier) ([]string, error) {
	return srv.NewInstancesVPC("", "", n, instType, imageId, state, groups)
}

// Instance returns the instance for the given instance id.
// It returns nil if there is no such instance.
func (srv *Server) Instance(id string) *Instance {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.instances[id]
}

func (inst *Instance) id() string {
	return fmt.Sprintf("i-%d", inst.seq)
}

func (inst *Instance) terminate() (d types.InstanceStateChange) {
	ps := inst.state
	d.PreviousState = &ps
	inst.state = ShuttingDown
	cs := inst.state
	d.CurrentState = &cs
	d.InstanceId = aws.String(inst.id())
	return d
}

func (inst *Instance) ec2instance() types.Instance {
	id := inst.id()
	// The first time the instance is returned, its DNSName
	// and block device mappings will be empty. The client
	// should then refresh the instance.
	var dnsName string
	var blockDeviceMappings []types.InstanceBlockDeviceMapping
	if inst.first {
		inst.first = false
	} else {
		dnsName = fmt.Sprintf("%s.testing.invalid", id)
		blockDeviceMappings = inst.blockDeviceMappings
	}
	return types.Instance{
		InstanceId:          aws.String(id),
		InstanceType:        inst.instType,
		ImageId:             aws.String(inst.imageId),
		PublicDnsName:       aws.String(dnsName),
		PrivateDnsName:      aws.String(fmt.Sprintf("%s.internal.invalid", id)),
		PublicIpAddress:     aws.String(fmt.Sprintf("8.0.0.%d", inst.seq%256)),
		PrivateIpAddress:    aws.String(fmt.Sprintf("127.0.0.%d", inst.seq%256)),
		State:               &inst.state,
		Placement:           &types.Placement{AvailabilityZone: aws.String(inst.availZone)},
		VpcId:               aws.String(inst.vpcId),
		SubnetId:            aws.String(inst.subnetId),
		BlockDeviceMappings: blockDeviceMappings,
		SourceDestCheck:     aws.Bool(inst.sourceDestCheck),
		Tags:                inst.tags,
		RootDeviceType:      inst.rootDeviceType,
		RootDeviceName:      aws.String(inst.rootDeviceName),
	}
}

func (inst *Instance) matchAttr(attr, value string) (ok bool, err error) {
	if strings.HasPrefix(attr, "tag:") && len(attr) > 4 {
		filterTag := attr[4:]
		return matchTag(inst.tags, filterTag, value), nil
	}
	switch attr {
	case "architecture":
		return value == "amd64", nil
	case "availability-zone":
		return value == inst.availZone, nil
	case "instance-id":
		return inst.id() == value, nil
	case "subnet-id":
		return inst.subnetId == value, nil
	case "vpc-id":
		return inst.vpcId == value, nil
	case "instance.group-id", "group-id":
		for _, g := range inst.reservation.groups {
			if g.id == value {
				return true, nil
			}
		}
		return false, nil
	case "instance.group-name", "group-name":
		for _, g := range inst.reservation.groups {
			if g.name == value {
				return true, nil
			}
		}
		return false, nil
	case "image-id":
		return value == inst.imageId, nil
	case "instance-state-code":
		code, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return code&0xff == int(aws.ToInt32(inst.state.Code)), nil
	case "instance-state-name":
		return value == string(inst.state.Name), nil
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) newInstance(r *reservation, instType types.InstanceType, imageId string, availZone string, state types.InstanceState) *Instance {
	inst := &Instance{
		seq:             srv.maxId.next(),
		first:           true,
		instType:        instType,
		imageId:         imageId,
		availZone:       availZone,
		state:           state,
		reservation:     r,
		sourceDestCheck: true,
	}
	id := inst.id()
	srv.instances[id] = inst
	r.instances[id] = inst

	if srv.createRootDisks {
		// create a root disk for the instance
		inst.rootDeviceType = "ebs"
		inst.rootDeviceName = "/dev/sda1"
		volume := srv.newVolume("magnetic", 8)
		volume.AvailabilityZone = aws.String(availZone)
		volume.State = "in-use"
		volumeAttachment := &volumeAttachment{}
		volumeAttachment.InstanceId = aws.String(inst.id())
		volumeAttachment.State = "attached"
		volumeAttachment.DeleteOnTermination = aws.Bool(true)
		volumeAttachment.Device = aws.String(inst.rootDeviceName)
		srv.volumeAttachments[aws.ToString(volume.VolumeId)] = volumeAttachment
		inst.blockDeviceMappings = []types.InstanceBlockDeviceMapping{{
			DeviceName: aws.String(inst.rootDeviceName),
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId:            volume.VolumeId,
				AttachTime:          aws.Time(time.Now()),
				Status:              "attached",
				DeleteOnTermination: volumeAttachment.DeleteOnTermination,
			},
		}}
	}

	return inst
}
