// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
)

// DescribeNetworkInterfaces implements ec2.Client.
func (srv *Server) DescribeNetworkInterfaces(ctx context.Context, in *ec2.DescribeNetworkInterfacesInput, opts ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	var f ec2filter
	idSet := set.NewStrings()
	if in != nil {
		f = in.Filters
		idSet = set.NewStrings(in.NetworkInterfaceIds...)
	}

	resp := &ec2.DescribeNetworkInterfacesOutput{}
	for _, i := range srv.ifaces {
		filterMatch, err := f.ok(i)
		if err != nil {
			return nil, apiError("InvalidParameterValue", "describe ifaces: %v", err)
		}
		if filterMatch && (idSet.Size() == 0 || idSet.Contains(aws.ToString(i.NetworkInterfaceId))) {
			resp.NetworkInterfaces = append(resp.NetworkInterfaces, i.NetworkInterface)
		}
	}
	return resp, nil
}

// AddNetworkInterface inserts a given network interface in the test server, as
// if it were created using the simulated AWS API. the Id field of inIface is
// ignored and replaced by the next ifaceId counter value, prefixed by "interface-".
// The VpcId, AvailabilityZone, Attachment.InstanceId, and SubnetIf fields must
// contain an existing vpc, AZ, instance and subnet (resp.).
// The interface will also be attached to the instance specified
func (srv *Server) AddNetworkInterface(inIface types.NetworkInterface) (types.NetworkInterface, error) {
	zeroIface := types.NetworkInterface{}

	vpcId := aws.ToString(inIface.VpcId)
	availZone := aws.ToString(inIface.AvailabilityZone)
	attachmentInstId := aws.ToString(inIface.Attachment.InstanceId)
	subnetId := aws.ToString(inIface.SubnetId)
	if vpcId == "" {
		return zeroIface, fmt.Errorf("empty VPCId field")
	}
	if availZone == "" {
		return zeroIface, fmt.Errorf("empty AvailZone field")
	}
	if attachmentInstId == "" {
		return zeroIface, fmt.Errorf("empty Attachment InstanceId field")
	}
	if subnetId == "" {
		return zeroIface, fmt.Errorf("empty SubnetId field")
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	if _, found := srv.vpcs[vpcId]; !found {
		return zeroIface, fmt.Errorf("no such VPC %q", vpcId)
	}
	if _, found := srv.zones[availZone]; !found {
		return zeroIface, fmt.Errorf("no such availability zone %q", availZone)
	}
	if _, found := srv.instances[attachmentInstId]; !found {
		return zeroIface, fmt.Errorf("no such instance %q", attachmentInstId)
	}
	if _, found := srv.subnets[subnetId]; !found {
		return zeroIface, fmt.Errorf("no such subnet %q", subnetId)
	}

	added := &iface{inIface}
	added.NetworkInterfaceId = aws.String(fmt.Sprintf("interface-%d", srv.ifaceId.next()))
	srv.ifaces[aws.ToString(added.NetworkInterfaceId)] = added
	srv.instances[attachmentInstId].ifaces = append(srv.instances[attachmentInstId].ifaces, added.NetworkInterface)
	return added.NetworkInterface, nil
}

type iface struct {
	types.NetworkInterface
}

func (i *iface) matchAttr(attr, value string) (ok bool, err error) {
	notImplemented := []string{
		"addresses.association.", "association.", "tag", "requester-",
	}
	switch attr {
	case "availability-zone":
		return aws.ToString(i.AvailabilityZone) == value, nil
	case "network-interface-id":
		return aws.ToString(i.NetworkInterfaceId) == value, nil
	case "status":
		return string(i.Status) == value, nil
	case "subnet-id":
		return aws.ToString(i.SubnetId) == value, nil
	case "vpc-id":
		return aws.ToString(i.VpcId) == value, nil
	case "attachment.attachment-id":
		return aws.ToString(i.Attachment.AttachmentId) == value, nil
	case "attachment.instance-id":
		return aws.ToString(i.Attachment.InstanceId) == value, nil
	case "attachment.instance-owner-id":
		return aws.ToString(i.Attachment.InstanceOwnerId) == value, nil
	case "attachment.device-index":
		devIndex, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		return aws.ToInt32(i.Attachment.DeviceIndex) == int32(devIndex), nil
	case "attachment.status":
		return string(i.Attachment.Status) == value, nil
	case "attachment.attach-time":
		return aws.ToTime(i.Attachment.AttachTime).Format(time.RFC3339) == value, nil
	case "attachment.delete-on-termination":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		// EC2 only filters attached NICs here, as the flag defaults
		// to false for manually created NICs and to true for
		// automatically created ones (during RunInstances)
		if aws.ToString(i.Attachment.AttachmentId) == "" {
			return false, nil
		}
		return aws.ToBool(i.Attachment.DeleteOnTermination) == flag, nil
	case "owner-id":
		return aws.ToString(i.OwnerId) == value, nil
	case "source-dest-check":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return aws.ToBool(i.SourceDestCheck) == flag, nil
	case "description":
		return aws.ToString(i.Description) == value, nil
	case "private-dns-name":
		return aws.ToString(i.PrivateDnsName) == value, nil
	case "mac-address":
		return aws.ToString(i.MacAddress) == value, nil
	case "private-ip-address", "addresses.private-ip-address":
		if aws.ToString(i.PrivateIpAddress) == value {
			return true, nil
		}
		// Look inside the secondary IPs list.
		for _, ip := range i.PrivateIpAddresses {
			if aws.ToString(ip.PrivateIpAddress) == value {
				return true, nil
			}
		}
		return false, nil
	case "addresses.primary":
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		for _, ip := range i.PrivateIpAddresses {
			if aws.ToBool(ip.Primary) == flag {
				return true, nil
			}
		}
		return false, nil
	case "group-id":
		for _, group := range i.Groups {
			if aws.ToString(group.GroupId) == value {
				return true, nil
			}
		}
		return false, nil
	case "group-name":
		for _, group := range i.Groups {
			if aws.ToString(group.GroupName) == value {
				return true, nil
			}
		}
		return false, nil
	default:
		for _, item := range notImplemented {
			if strings.HasPrefix(attr, item) {
				return false, fmt.Errorf("%q filter not implemented", attr)
			}
		}
	}
	return false, fmt.Errorf("unknown attribute %q", attr)
}

func (srv *Server) parseNetworkInterfaces(in []types.InstanceNetworkInterfaceSpecification) ([]types.NetworkInterface, bool, error) {
	ifaces := []types.NetworkInterface{}
	limitToOneInstance := false
	for i, spec := range in {
		for len(ifaces)-1 < i {
			ifaces = append(ifaces, types.NetworkInterface{Attachment: &types.NetworkInterfaceAttachment{}})
		}
		iface := ifaces[i]
		iface.NetworkInterfaceId = spec.NetworkInterfaceId
		limitToOneInstance = spec.NetworkInterfaceId != nil
		iface.Description = spec.Description
		iface.Attachment.DeleteOnTermination = spec.DeleteOnTermination
		iface.Attachment.DeviceIndex = spec.DeviceIndex
		if subnetId := aws.ToString(spec.SubnetId); subnetId != "" {
			if _, ok := srv.subnets[subnetId]; !ok {
				return nil, false, apiError("InvalidSubnetID.NotFound", "no such subnet id %q", subnetId)
			}
			iface.SubnetId = spec.SubnetId
		}
		if spec.PrivateIpAddress != nil {
			privateIP := types.NetworkInterfacePrivateIpAddress{
				PrivateIpAddress: spec.PrivateIpAddress,
				PrivateDnsName:   aws.String(srv.dnsNameFromPrivateIP(aws.ToString(spec.PrivateIpAddress))),
				Primary:          aws.Bool(true),
			}
			iface.PrivateIpAddresses = append(iface.PrivateIpAddresses, privateIP)
			limitToOneInstance = true
		}
		for _, sgId := range spec.Groups {
			g, ok := srv.groups[sgId]
			if !ok {
				return nil, false, apiError("InvalidParameterValue", "no such security group id %q", sgId)
			}
			iface.Groups = append(iface.Groups, types.GroupIdentifier{
				GroupId:   aws.String(sgId),
				GroupName: g.ec2SecurityGroup().GroupName,
			})
		}
		for ipIndex, a := range spec.PrivateIpAddresses {
			for len(iface.PrivateIpAddresses)-1 < ipIndex {
				iface.PrivateIpAddresses = append(iface.PrivateIpAddresses, types.NetworkInterfacePrivateIpAddress{})
			}
			privateIP := iface.PrivateIpAddresses[ipIndex]
			privateIP.PrivateIpAddress = a.PrivateIpAddress
			privateIP.PrivateDnsName = aws.String(srv.dnsNameFromPrivateIP(aws.ToString(a.PrivateIpAddress)))
			privateIP.Primary = a.Primary
			iface.PrivateIpAddresses[ipIndex] = privateIP
		}
		ifaces[i] = iface
	}
	return ifaces, limitToOneInstance, nil
}

// addDefaultNIC requests the creation of a default network interface
// for each instance to launch in RunInstances, using the given
// instance subnet, and it's called when no explicit NICs are given.
// It returns the populated NetworkInterface slice to add to
// RunInstances params.
func (srv *Server) addDefaultNIC(instSubnet *subnet) []types.NetworkInterface {
	if instSubnet == nil {
		// No subnet specified, nothing to do.
		return nil
	}
	instSubnetId := aws.ToString(instSubnet.SubnetId)
	ifID := srv.ifaceId.next()

	netInterface := types.NetworkInterface{
		NetworkInterfaceId: aws.String(fmt.Sprintf("eni-%d", ifID)),
		Description:        aws.String("created by ec2test server"),
		Attachment: &types.NetworkInterfaceAttachment{
			AttachmentId:        aws.String(fmt.Sprintf("eni-attach-%d", srv.attachId.next())),
			DeviceIndex:         aws.Int32(0),
			DeleteOnTermination: aws.Bool(true),
		},
		Ipv6Addresses: []types.NetworkInterfaceIpv6Address{},
		PrivateIpAddresses: []types.NetworkInterfacePrivateIpAddress{{
			Primary: aws.Bool(true),
			// Assign a public shadow IP
			Association: &types.NetworkInterfaceAssociation{
				PublicIp:      aws.String(fmt.Sprintf("73.37.0.%d", ifID+1)),
				PublicDnsName: aws.String(fmt.Sprintf("ec2-73-37-0-%d.compute-1.amazonaws.com", ifID+1)),
				IpOwnerId:     aws.String("amazon"),
			},
		}},
	}

	hasIPAddress := false
	if instSubnet.CidrBlock != nil {
		ip, ipnet, err := net.ParseCIDR(aws.ToString(instSubnet.CidrBlock))
		if err != nil {
			panic(fmt.Sprintf("subnet %q has invalid CIDR: %v", instSubnetId, err.Error()))
		}
		ip[len(ip)-1] = 5
		if !ipnet.Contains(ip) {
			panic(fmt.Sprintf("%q does not contain IP %q", instSubnetId, ip))
		}

		hasIPAddress = true
		netInterface.PrivateIpAddresses[0].PrivateIpAddress = aws.String(ip.String())
		netInterface.PrivateIpAddresses[0].PrivateDnsName = aws.String(srv.dnsNameFromPrivateIP(ip.String()))
	}

	if instSubnet.AssignIpv6AddressOnCreation != nil &&
		*instSubnet.AssignIpv6AddressOnCreation &&
		len(instSubnet.Ipv6CidrBlockAssociationSet) != 0 {
		for i, ipv6Assoc := range instSubnet.Ipv6CidrBlockAssociationSet {
			ip, ipnet, err := net.ParseCIDR(aws.ToString(ipv6Assoc.Ipv6CidrBlock))
			if err != nil {
				panic(fmt.Sprintf("subnet %q has invalid ipv6 CIDR: %v", instSubnetId, err.Error()))
			}

			ip[len(ip)-1] = 5
			if !ipnet.Contains(ip) {
				panic(fmt.Sprintf("%q does not contain IP %q", instSubnetId, ip))
			}

			hasIPAddress = true
			netInterface.Ipv6Addresses = append(netInterface.Ipv6Addresses, types.NetworkInterfaceIpv6Address{
				Ipv6Address:   aws.String(ip.String()),
				IsPrimaryIpv6: aws.Bool(i == 0),
			})
			if i == 0 {
				netInterface.Ipv6Address = aws.String(ip.String())
			}
		}
	}

	if !hasIPAddress {
		panic(fmt.Sprintf("subnet id %q does not have any ip addresses to assign", instSubnetId))
	}

	return []types.NetworkInterface{netInterface}
}

// createNICsOnRun creates and returns any network interfaces
// specified in ifacesToCreate in the server for the given instance id
// and subnet.
func (srv *Server) createNICsOnRun(instId string, instSubnet *subnet, ifacesToCreate []types.NetworkInterface) []types.NetworkInterface {
	if instSubnet == nil {
		// No subnet specified, nothing to do.
		return nil
	}

	var createdNICs []types.NetworkInterface
	for _, ifaceToCreate := range ifacesToCreate {
		nicId := aws.ToString(ifaceToCreate.NetworkInterfaceId)
		macAddress := fmt.Sprintf("20:%02x:60:cb:27:37", srv.ifaceId.get())
		if nicId == "" {
			// Simulate a NIC got created.
			nicId = fmt.Sprintf("eni-%d", srv.ifaceId.next())
			macAddress = fmt.Sprintf("20:%02x:60:cb:27:37", srv.ifaceId.get())
		}
		groups := make([]types.GroupIdentifier, len(ifaceToCreate.Groups))
		for i, sg := range ifaceToCreate.Groups {
			sg := sg
			groups[i] = sg
		}
		// Find the primary private IP address for the NIC
		// inside the PrivateIPs slice.
		primaryIP := ""
		for i, ip := range ifaceToCreate.PrivateIpAddresses {
			if aws.ToBool(ip.Primary) {
				primaryIP = aws.ToString(ip.PrivateIpAddress)
				dnsName := srv.dnsNameFromPrivateIP(primaryIP)
				ifaceToCreate.PrivateIpAddresses[i].PrivateDnsName = aws.String(dnsName)
				break
			}
		}
		attach := types.NetworkInterfaceAttachment{
			AttachmentId:        aws.String(fmt.Sprintf("eni-attach-%d", srv.attachId.next())),
			InstanceId:          aws.String(instId),
			InstanceOwnerId:     aws.String(ownerId),
			DeviceIndex:         ifaceToCreate.Attachment.DeviceIndex,
			Status:              "in-use",
			AttachTime:          aws.Time(time.Now()),
			DeleteOnTermination: aws.Bool(true),
		}
		nic := types.NetworkInterface{
			NetworkInterfaceId: aws.String(nicId),
			SubnetId:           instSubnet.SubnetId,
			VpcId:              instSubnet.VpcId,
			AvailabilityZone:   instSubnet.AvailabilityZone,
			Description:        ifaceToCreate.Description,
			OwnerId:            aws.String(ownerId),
			Status:             "in-use",
			MacAddress:         aws.String(macAddress),
			PrivateIpAddress:   aws.String(primaryIP),
			PrivateDnsName:     aws.String(srv.dnsNameFromPrivateIP(primaryIP)),
			SourceDestCheck:    aws.Bool(true),
			Groups:             groups,
			PrivateIpAddresses: ifaceToCreate.PrivateIpAddresses,
			Attachment:         &attach,
			Ipv6Addresses:      ifaceToCreate.Ipv6Addresses,
			Ipv6Address:        ifaceToCreate.Ipv6Address,
		}

		srv.ifaces[nicId] = &iface{nic}
		createdNICs = append(createdNICs, nic)
	}
	return createdNICs
}

func (srv *Server) dnsNameFromPrivateIP(privateIP string) string {
	return fmt.Sprintf("ip-%s.ec2.internal", strings.Replace(privateIP, ".", "-", -1))
}
