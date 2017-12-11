// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	ociCore "github.com/oracle/oci-go-sdk/core"
)

const (
	// DefaultAddressSpace is the subnet to use for the default juju VCN
	// An individual subnet will be created from this class, for each
	// availability domain.
	DefaultAddressSpace = "10.0.0.0/16"

	SubnetPrefixLength = "24"

	VcnNamePrefix         = "juju-vcn"
	SecListNamePrefix     = "juju-seclist"
	SubnetNamePrefix      = "juju-subnet"
	InternetGatewayPrefix = "juju-ig"
	RouteTablePrefix      = "juju-rt"
)

// TODO(gsamfira): Use "local" instead? make configurable?
var DnsLabelTld = "local"

func (e *Environ) vcnName(controllerUUID string) *string {
	name := fmt.Sprintf("%s-%s", VcnNamePrefix, controllerUUID)
	return &name
}

func (e *Environ) getVCNStatus(vcnID *string) (string, error) {
	request := ociCore.GetVcnRequest{
		VcnId: vcnID,
	}

	response, err := e.cli.GetVcn(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("vcn not found: %s", *vcnID)
		} else {
			return "", err
		}
	}
	return string(response.Vcn.LifecycleState), nil
}

func (e *Environ) getVcn(controllerUUID string) (ociCore.Vcn, error) {
	request := ociCore.ListVcnsRequest{
		CompartmentId: e.ecfg().compartmentID(),
	}

	response, err := e.cli.ListVcns(context.Background(), request)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	name := e.vcnName(controllerUUID)

	if len(response.Items) > 0 {
		for _, val := range response.Items {
			// NOTE(gsamfira): Display names are not unique. We only care
			// about VCNs that have been created for this controller.
			// While we do include the controller UUID in the name of
			// the VCN, I believe it is worth doing an extra check.
			if *val.DisplayName != *name {
				continue
			}
			if tag, ok := val.FreeFormTags[tags.JujuController]; ok {
				if tag == controllerUUID {
					return val, nil
				}
			}
		}
	}
	return ociCore.Vcn{}, errors.NotFoundf("no such VCN: %s", *name)
}

func (e *Environ) secListName(controllerUUID string) string {
	return fmt.Sprintf("juju-seclist-%s", controllerUUID)
}

func (e *Environ) ensureVCN(controllerUUID string) (ociCore.Vcn, error) {
	logger.Infof("ensuring that VCN is created")
	if vcn, err := e.getVcn(controllerUUID); err != nil {
		if !errors.IsNotFound(err) {
			return ociCore.Vcn{}, err
		}
	} else {
		return vcn, nil
	}

	name := e.vcnName(controllerUUID)
	logger.Infof("creating new VCN %s", *name)
	addressSpace := DefaultAddressSpace
	vcnDetails := ociCore.CreateVcnDetails{
		CidrBlock:     &addressSpace,
		CompartmentId: e.ecfg().compartmentID(),
		DisplayName:   name,
		DnsLabel:      &DnsLabelTld,
		FreeFormTags: map[string]string{
			tags.JujuController: controllerUUID,
		},
	}
	request := ociCore.CreateVcnRequest{
		CreateVcnDetails: vcnDetails,
	}

	result, err := e.cli.CreateVcn(context.Background(), request)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	logger.Debugf("VCN %s created. Waiting for status: %s", *result.Vcn.Id, string(ociCore.VcnLifecycleStateAvailable))
	err = e.waitForResourceStatus(
		e.getVCNStatus, result.Vcn.Id,
		string(ociCore.VcnLifecycleStateAvailable),
		5*time.Minute)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	vcn := result.Vcn
	return vcn, nil
}

func (e *Environ) getSeclistStatus(resourceID *string) (string, error) {
	request := ociCore.GetSecurityListRequest{
		SecurityListId: resourceID,
	}

	response, err := e.cli.GetSecurityList(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("seclist not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.SecurityList.LifecycleState), nil
}

func (e *Environ) getSecurityList(controllerUUID string, vcnid *string) (ociCore.SecurityList, error) {
	request := ociCore.ListSecurityListsRequest{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnid,
	}
	response, err := e.cli.ListSecurityLists(context.Background(), request)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}
	if len(response.Items) == 0 {
		return ociCore.SecurityList{}, errors.NotFoundf("no security lists found for vcn: %v", *vcnid)
	}
	for _, val := range response.Items {
		if tag, ok := val.FreeFormTags[tags.JujuController]; ok {
			if tag == controllerUUID {
				return val, nil
			}
		}
	}
	return ociCore.SecurityList{}, errors.NotFoundf("no security lists found for vcn: %v", *vcnid)
}

func (e *Environ) ensureSecurityList(controllerUUID string, vcnid *string) (ociCore.SecurityList, error) {
	if seclist, err := e.getSecurityList(controllerUUID, vcnid); err != nil {
		if !errors.IsNotFound(err) {
			return ociCore.SecurityList{}, errors.Trace(err)
		}
	} else {
		return seclist, nil
	}

	prefix := "0.0.0.0/0"
	name := e.secListName(controllerUUID)
	// Hopefully just temporary, open all ingress/egress ports
	details := ociCore.CreateSecurityListDetails{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnid,
		DisplayName:   &name,
		FreeFormTags: map[string]string{
			tags.JujuController: controllerUUID,
		},
		EgressSecurityRules: []ociCore.EgressSecurityRule{
			ociCore.EgressSecurityRule{
				Destination: &prefix,
				Protocol:    &allProtocols,
			},
		},
		IngressSecurityRules: []ociCore.IngressSecurityRule{
			ociCore.IngressSecurityRule{
				Source:   &prefix,
				Protocol: &allProtocols,
			},
		},
	}

	request := ociCore.CreateSecurityListRequest{
		CreateSecurityListDetails: details,
	}

	response, err := e.cli.CreateSecurityList(context.Background(), request)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}

	err = e.waitForResourceStatus(
		e.getSeclistStatus, response.SecurityList.Id,
		string(ociCore.SecurityListLifecycleStateAvailable),
		5*time.Minute)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}
	return response.SecurityList, nil
}

func (e *Environ) allSubnets(controllerUUID string, vcnID *string) (map[string][]ociCore.Subnet, error) {
	request := ociCore.ListSubnetsRequest{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnID,
	}
	response, err := e.cli.ListSubnets(context.Background(), request)
	if err != nil {
		return nil, err
	}

	ret := map[string][]ociCore.Subnet{}
	for _, val := range response.Items {
		if tag, ok := val.FreeFormTags[tags.JujuController]; ok {
			if tag == controllerUUID {
				cidr := *val.CidrBlock
				if valid, err := e.validateCidrBlock(cidr); err != nil || !valid {
					logger.Warningf("failed to validate CIDR block %s: %s", cidr, err)
					continue
				}
				ret[*val.AvailabilityDomain] = append(ret[*val.AvailabilityDomain], val)
			}
		}
	}
	return ret, nil
}

func (e *Environ) validateCidrBlock(cidr string) (bool, error) {
	_, vncIPNet, err := net.ParseCIDR(DefaultAddressSpace)
	if err != nil {
		return false, errors.Trace(err)
	}

	subnetIP, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, errors.Trace(err)
	}
	if vncIPNet.Contains(subnetIP) {
		return true, nil
	}
	return false, nil
}

func (e *Environ) getFreeSubnet(existing map[string]bool) (string, error) {
	ip, _, err := net.ParseCIDR(DefaultAddressSpace)
	if err != nil {
		return "", errors.Trace(err)
	}
	to4 := ip.To4()
	if to4 == nil {
		return "", errors.Errorf("invalid IPv4 address: %s", DefaultAddressSpace)
	}

	for i := 0; i <= 255; i++ {
		to4[2] = byte(i)
		subnet := fmt.Sprintf("%s/%s", to4.String(), SubnetPrefixLength)
		if _, ok := existing[subnet]; ok {
			continue
		}
		existing[subnet] = true
		return subnet, nil
	}
	return "", errors.Errorf("failed to find a free subnet")
}

func (e *Environ) getSubnetStatus(resourceID *string) (string, error) {
	request := ociCore.GetSubnetRequest{
		SubnetId: resourceID,
	}

	response, err := e.cli.GetSubnet(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("subnet not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.Subnet.LifecycleState), nil
}

func (e *Environ) createSubnet(controllerUUID, ad, cidr string, vcnID *string, seclists []string, routeRableID *string) (ociCore.Subnet, error) {
	displayName := fmt.Sprintf("juju-%s-%s", ad, controllerUUID)
	compartment := e.ecfg().compartmentID()
	// TODO(gsamfira): maybe "local" would be better?
	subnetDetails := ociCore.CreateSubnetDetails{
		AvailabilityDomain: &ad,
		CidrBlock:          &cidr,
		CompartmentId:      compartment,
		VcnId:              vcnID,
		DisplayName:        &displayName,
		RouteTableId:       routeRableID,
		SecurityListIds:    seclists,
		// DnsLabel:           &providerNetwork.DnsLabel,
		FreeFormTags: map[string]string{
			tags.JujuController: controllerUUID,
		},
	}

	request := ociCore.CreateSubnetRequest{
		CreateSubnetDetails: subnetDetails,
	}

	response, err := e.cli.CreateSubnet(context.Background(), request)
	if err != nil {
		return ociCore.Subnet{}, errors.Trace(err)
	}
	err = e.waitForResourceStatus(
		e.getSubnetStatus, response.Subnet.Id,
		string(ociCore.SubnetLifecycleStateAvailable),
		5*time.Minute)
	if err != nil {
		return ociCore.Subnet{}, errors.Trace(err)
	}
	return response.Subnet, nil
}

func (e *Environ) ensureSubnets(vcn ociCore.Vcn, secList ociCore.SecurityList, controllerUUID string, routeTableID *string) (map[string][]ociCore.Subnet, error) {
	az, err := e.AvailabilityZones()
	if err != nil {
		return nil, errors.Trace(err)
	}

	allSubnets, err := e.allSubnets(controllerUUID, vcn.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	existingCidrBlocks := map[string]bool{}
	missing := map[string]bool{}
	// Check that we have one subnet, and only one subnet in each availability domain
	for _, val := range az {
		name := val.Name()
		subnets, ok := allSubnets[name]
		if !ok {
			missing[name] = true
			continue
		}
		for _, val := range subnets {
			cidr := *val.CidrBlock
			existingCidrBlocks[cidr] = true
		}
	}

	if len(missing) > 0 {
		for ad, _ := range missing {
			newIPNet, err := e.getFreeSubnet(existingCidrBlocks)
			if err != nil {
				return nil, errors.Trace(err)
			}
			newSubnet, err := e.createSubnet(controllerUUID, ad, newIPNet, vcn.Id, []string{*secList.Id}, routeTableID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			allSubnets[ad] = []ociCore.Subnet{
				newSubnet,
			}
		}
	}
	return allSubnets, nil
}

// ensureNetworksAndSubnets creates VCNs, security lists and subnets that will
// be used throughout the life-cycle of this juju deployment.
func (e *Environ) ensureNetworksAndSubnets(controllerUUID string) (map[string][]ociCore.Subnet, error) {
	// if we have the subnets field populated, it means we already checked/created
	// the necessary resources. Simply return.
	if e.subnets != nil {
		return e.subnets, nil
	}
	vcn, err := e.ensureVCN(controllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// NOTE(gsamfira): There are some limitations at the moment in regards to
	// security lists:
	// * Security lists can only be applied on subnets
	// * Once subnet is created, you may not attach a new security list to that subnet
	// * there is no way to apply a security list on an instance/VNIC
	// * We cannot create a model level security list, unless we create a new subnet for that model
	// ** that means at least 3 subnets per model, which is something we probably don't want
	// * There is no way to specify the target prefix for an Ingress/Egress rule, thus making
	// instance level firewalling, impossible.
	// For now, we open all ports until we decide how to properly take care of this.
	secList, err := e.ensureSecurityList(controllerUUID, vcn.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ig, err := e.ensureInternetGateway(controllerUUID, vcn.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create a route rule that will set the internet gateway created above
	// as a default gateway
	prefix := "0.0.0.0/0"
	// TODO(gsamfira): create route table
	routeRules := []ociCore.RouteRule{
		ociCore.RouteRule{
			CidrBlock:       &prefix,
			NetworkEntityId: ig.Id,
		},
	}
	routeTable, err := e.ensureRouteTable(controllerUUID, vcn.Id, routeRules)
	if err != nil {
		return nil, err
	}

	subnets, err := e.ensureSubnets(vcn, secList, controllerUUID, routeTable.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(gsamfira): should we use a lock here?
	e.subnets = subnets
	return e.subnets, nil
}

// cleanupNetworksAndSubnets destroys all subnets, VCNs and security lists that have
// been used by this juju deployment. This function should only be called when
// destroying the environment, and only after destroying any resources that may be attached
// to a network.
func (e *Environ) cleanupNetworksAndSubnets(controllerUUID string) error {

	if e.vcn.Id == nil {
		vcn, err := e.getVcn(controllerUUID)
		if err != nil {
			if errors.IsNotFound(err) {
				// no VCN was created, we can just return here
				return nil
			}
		}
		e.vcn = vcn
	}

	if e.subnets == nil {
		allSubnets, err := e.allSubnets(controllerUUID, e.vcn.Id)
		if err != nil {
			return errors.Trace(err)
		}
		e.subnets = allSubnets
	}
	vcnID := e.vcn.Id
	secList, err := e.getSecurityList(controllerUUID, vcnID)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	for _, adSubnets := range e.subnets {
		for _, subnet := range adSubnets {
			if vcnID != nil {
				if *vcnID != *subnet.VcnId {
					return errors.Errorf(
						"Found a subnet with a different vcnID. This should not happen. Vcn: %s, subnet: %s", *vcnID, *subnet.VcnId)
				}
			}
			request := ociCore.DeleteSubnetRequest{
				SubnetId: subnet.Id,
			}
			// we may need to wait for resource to be deleted
			response, err := e.cli.DeleteSubnet(context.Background(), request)
			// Should we attempt to delete all subnets and return an array of errors?
			if err != nil && !e.isNotFound(response.RawResponse) {
				return errors.Trace(err)
			}
			err = e.waitForResourceStatus(
				e.getSubnetStatus, subnet.Id,
				string(ociCore.SubnetLifecycleStateTerminated),
				5*time.Minute)
			if err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	if secList.Id != nil {
		request := ociCore.DeleteSecurityListRequest{
			SecurityListId: secList.Id,
		}
		logger.Infof("deleting security list %s", *secList.Id)
		response, err := e.cli.DeleteSecurityList(context.Background(), request)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return nil
		}
		err = e.waitForResourceStatus(
			e.getSeclistStatus, secList.Id,
			string(ociCore.SubnetLifecycleStateTerminated),
			5*time.Minute)
		if !errors.IsNotFound(err) {
			return err
		}
	}
	if err := e.deleteRouteTable(controllerUUID, vcnID); err != nil {
		return errors.Trace(err)
	}

	if err := e.deleteInternetGateway(vcnID); err != nil {
		return errors.Trace(err)
	}

	requestDeleteVcn := ociCore.DeleteVcnRequest{
		VcnId: vcnID,
	}

	if vcnID != nil {
		logger.Infof("deleting VCN: %s", *vcnID)
		response, err := e.cli.DeleteVcn(context.Background(), requestDeleteVcn)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return err
		}
		err = e.waitForResourceStatus(
			e.getVCNStatus, vcnID,
			string(ociCore.VcnLifecycleStateTerminated),
			5*time.Minute)
		if !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (e *Environ) getInternetGatewayStatus(resourceID *string) (string, error) {
	request := ociCore.GetInternetGatewayRequest{
		IgId: resourceID,
	}

	response, err := e.cli.GetInternetGateway(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("internet gateway not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.InternetGateway.LifecycleState), nil
}

func (e *Environ) getInternetGateway(vcnID *string) (ociCore.InternetGateway, error) {
	request := ociCore.ListInternetGatewaysRequest{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnID,
	}

	response, err := e.cli.ListInternetGateways(context.Background(), request)
	if err != nil {
		return ociCore.InternetGateway{}, nil
	}
	if len(response.Items) == 0 {
		return ociCore.InternetGateway{}, errors.NotFoundf("internet gateway not found")
	}

	return response.Items[0], nil
}

func (e *Environ) internetGatewayName(controllerUUID string) *string {
	name := fmt.Sprintf("%s-%s", InternetGatewayPrefix, controllerUUID)
	return &name
}

func (e *Environ) ensureInternetGateway(controllerUUID string, vcnID *string) (ociCore.InternetGateway, error) {
	if ig, err := e.getInternetGateway(vcnID); err != nil {
		if !errors.IsNotFound(err) {
			return ociCore.InternetGateway{}, errors.Trace(err)
		}
	} else {
		return ig, nil
	}

	enabled := true
	details := ociCore.CreateInternetGatewayDetails{
		VcnId:         vcnID,
		CompartmentId: e.ecfg().compartmentID(),
		IsEnabled:     &enabled,
		DisplayName:   e.internetGatewayName(controllerUUID),
	}

	request := ociCore.CreateInternetGatewayRequest{
		CreateInternetGatewayDetails: details,
	}

	response, err := e.cli.CreateInternetGateway(context.Background(), request)
	if err != nil {
		return ociCore.InternetGateway{}, errors.Trace(err)
	}

	if err := e.waitForResourceStatus(
		e.getInternetGatewayStatus,
		response.InternetGateway.Id,
		string(ociCore.InternetGatewayLifecycleStateAvailable),
		5*time.Minute); err != nil {

		return ociCore.InternetGateway{}, errors.Trace(err)
	}

	return response.InternetGateway, nil
}

func (e *Environ) deleteInternetGateway(vcnID *string) error {
	ig, err := e.getInternetGateway(vcnID)
	if err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		return nil
	}
	terminatingStatus := ociCore.InternetGatewayLifecycleStateTerminating
	terminatedStatus := ociCore.InternetGatewayLifecycleStateTerminated
	if ig.LifecycleState == terminatedStatus {
		return nil
	}

	if ig.LifecycleState != terminatingStatus {

		request := ociCore.DeleteInternetGatewayRequest{
			IgId: ig.Id,
		}

		response, err := e.cli.DeleteInternetGateway(context.Background(), request)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return errors.Trace(err)
		}
	}
	if err := e.waitForResourceStatus(
		e.getInternetGatewayStatus,
		ig.Id,
		string(terminatedStatus),
		5*time.Minute); err != nil {

		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}

	return nil
}

func (e *Environ) getRouteTable(controllerUUID string, vcnID *string) (ociCore.RouteTable, error) {
	request := ociCore.ListRouteTablesRequest{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnID,
	}

	response, err := e.cli.ListRouteTables(context.Background(), request)
	if err != nil {
		return ociCore.RouteTable{}, errors.Trace(err)
	}

	for _, val := range response.Items {
		if tag, ok := val.FreeFormTags[tags.JujuController]; ok {
			if tag == controllerUUID {
				return val, nil
			}
		}
	}
	return ociCore.RouteTable{}, errors.NotFoundf("no route table found")
}

func (e *Environ) routeTableName(controllerUUID string) *string {
	name := fmt.Sprintf("%s-%s", RouteTablePrefix, controllerUUID)
	return &name
}

func (e *Environ) getRouteTableStatus(resourceID *string) (string, error) {
	request := ociCore.GetRouteTableRequest{
		RtId: resourceID,
	}

	response, err := e.cli.GetRouteTable(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("route table not found: %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.RouteTable.LifecycleState), nil
}

func (e *Environ) ensureRouteTable(controllerUUID string, vcnID *string, routeRules []ociCore.RouteRule) (ociCore.RouteTable, error) {
	if rt, err := e.getRouteTable(controllerUUID, vcnID); err != nil {
		if !errors.IsNotFound(err) {
			return ociCore.RouteTable{}, errors.Trace(err)
		}
	} else {
		return rt, nil
	}

	details := ociCore.CreateRouteTableDetails{
		VcnId:         vcnID,
		CompartmentId: e.ecfg().compartmentID(),
		RouteRules:    routeRules,
		DisplayName:   e.routeTableName(controllerUUID),
		FreeFormTags: map[string]string{
			tags.JujuController: controllerUUID,
		},
	}

	request := ociCore.CreateRouteTableRequest{
		CreateRouteTableDetails: details,
	}

	response, err := e.cli.CreateRouteTable(context.Background(), request)
	if err != nil {
		return ociCore.RouteTable{}, errors.Trace(err)
	}

	if err := e.waitForResourceStatus(
		e.getRouteTableStatus,
		response.RouteTable.Id,
		string(ociCore.RouteTableLifecycleStateAvailable),
		5*time.Minute); err != nil {

		return ociCore.RouteTable{}, errors.Trace(err)
	}

	return response.RouteTable, nil
}

func (e *Environ) deleteRouteTable(controllerUUID string, vcnID *string) error {
	rt, err := e.getRouteTable(controllerUUID, vcnID)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if rt.LifecycleState == ociCore.RouteTableLifecycleStateTerminated {
		return nil
	}

	if rt.LifecycleState != ociCore.RouteTableLifecycleStateTerminating {
		request := ociCore.DeleteRouteTableRequest{
			RtId: rt.Id,
		}

		response, err := e.cli.DeleteRouteTable(context.Background(), request)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return errors.Trace(err)
		}
	}

	if err := e.waitForResourceStatus(
		e.getRouteTableStatus,
		rt.Id,
		string(ociCore.RouteTableLifecycleStateTerminated),
		5*time.Minute); err != nil {

		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

// Subnets is defined on the environs.Networking interface.
func (e *Environ) Subnets(id instance.Id, subnets []network.Id) ([]network.SubnetInfo, error) {
	return nil, nil
}

func (e *Environ) SuperSubnets() ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

func (e *Environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	// attachments, err := e.cli.GetInstanceVnicAttachments(instId, e.ecfg().compartmentID())
	// if err != nil {
	// 	return nil, errors.Trace(err)
	// }

	// vnics, err := e.cli.GetInstanceVnics(attachments.Items)
	// if err != nil {
	// 	return nil, errors.Trace(err)
	// }

	// interfaces := []network.InterfaceInfo{}
	return nil, nil
}

func (e *Environ) SupportsSpaces() (bool, error) {
	return false, errors.NotSupportedf("spaces")
}

func (e *Environ) SupportsSpaceDiscovery() (bool, error) {
	return false, errors.NotSupportedf("space discovery")
}

func (e *Environ) Spaces() ([]network.SpaceInfo, error) {
	return nil, nil
}

func (e *Environ) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, nil
}

func (e *Environ) AreSpacesRoutable(space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

func (e *Environ) SupportsContainerAddresses() (bool, error) {
	return false, errors.NotSupportedf("container addresses")
}

func (e *Environ) AllocateContainerAddresses(
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {

	return nil, nil
}

func (e *Environ) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return nil
}

func (e *Environ) SSHAddresses(addresses []network.Address) ([]network.Address, error) {
	return addresses, nil
}
