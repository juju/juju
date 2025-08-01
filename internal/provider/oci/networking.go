// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/tags"
)

const (
	// DefaultAddressSpace is the subnet to use for the default juju VCN
	// An individual subnet will be created from this class, for each
	// availability domain.
	DefaultAddressSpace = "10.0.0.0/16"
	AllowAllPrefix      = "0.0.0.0/0"

	SubnetPrefixLength = "24"

	VcnNamePrefix         = "juju-vcn"
	SecListNamePrefix     = "juju-seclist"
	InternetGatewayPrefix = "juju-ig"
	RouteTablePrefix      = "juju-rt"
)

var (
	allProtocols = "all"

	resourcePollTimeout = 5 * time.Minute
)

func (e *Environ) vcnName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", VcnNamePrefix, controllerUUID, modelUUID)
}

func (e *Environ) getVCNStatus(vcnID *string) (string, error) {
	request := ociCore.GetVcnRequest{
		VcnId: vcnID,
	}

	response, err := e.Networking.GetVcn(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("vcn %q", *vcnID)
		} else {
			return "", err
		}
	}
	return string(response.Vcn.LifecycleState), nil
}

func (e *Environ) allVCNs(controllerUUID, modelUUID string) ([]ociCore.Vcn, error) {
	vcns, err := e.Networking.ListVcns(context.Background(), e.ecfg().compartmentID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ret := []ociCore.Vcn{}
	if len(vcns) > 0 {
		for _, val := range vcns {
			tag, ok := val.FreeformTags[tags.JujuController]
			if !ok || tag != controllerUUID {
				continue
			}
			if modelUUID != "" {
				tag, ok = val.FreeformTags[tags.JujuModel]
				if !ok || tag != modelUUID {
					continue
				}
			}
			ret = append(ret, val)
		}
	}
	return ret, nil
}

func (e *Environ) getVCN(modelUUID string) (ociCore.Vcn, error) {
	vcns, err := e.allVCNs(e.controllerUUID, modelUUID)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	if len(vcns) > 1 {
		return ociCore.Vcn{}, errors.Errorf("expected 1 VCN, got: %d", len(vcns))
	}

	if len(vcns) == 0 {
		return ociCore.Vcn{}, errors.NotFoundf("vcn")
	}
	return vcns[0], nil
}

func (e *Environ) secListName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", SecListNamePrefix, controllerUUID, modelUUID)
}

func (e *Environ) ensureVCN(ctx context.Context, modelUUID string) (ociCore.Vcn, error) {
	if vcn, err := e.getVCN(modelUUID); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return ociCore.Vcn{}, errors.Trace(err)
		}
	} else {
		return vcn, nil
	}

	name := e.vcnName(e.controllerUUID, modelUUID)
	logger.Debugf(ctx, "creating new VCN %s", name)

	vcnDetails := ociCore.CreateVcnDetails{
		CidrBlock:     e.ecfg().addressSpace(),
		CompartmentId: e.ecfg().compartmentID(),
		DisplayName:   &name,
		FreeformTags: map[string]string{
			tags.JujuController: e.controllerUUID,
			tags.JujuModel:      modelUUID,
		},
	}
	request := ociCore.CreateVcnRequest{
		CreateVcnDetails: vcnDetails,
	}

	result, err := e.Networking.CreateVcn(context.Background(), request)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	logger.Debugf(ctx, "VCN %s created. Waiting for status: %s", *result.Vcn.Id, string(ociCore.VcnLifecycleStateAvailable))

	err = e.waitForResourceStatus(
		e.getVCNStatus, result.Vcn.Id,
		string(ociCore.VcnLifecycleStateAvailable),
		resourcePollTimeout)
	if err != nil {
		return ociCore.Vcn{}, errors.Trace(err)
	}
	vcn := result.Vcn
	return vcn, nil
}

func (e *Environ) getSecurityListStatus(resourceID *string) (string, error) {
	request := ociCore.GetSecurityListRequest{
		SecurityListId: resourceID,
	}

	response, err := e.Firewall.GetSecurityList(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("security list: %q", *resourceID)
		} else {
			return "", errors.Trace(err)
		}
	}
	return string(response.SecurityList.LifecycleState), nil
}

// jujuSecurityLists returns the security lists for the input VCN
// that were created by juju.
func (e *Environ) jujuSecurityLists(vcnId *string) ([]ociCore.SecurityList, error) {
	var ret []ociCore.SecurityList

	lists, err := e.Firewall.ListSecurityLists(context.Background(), e.ecfg().compartmentID(), vcnId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(lists) == 0 {
		return ret, errors.NotFoundf("security lists for vcn: %q", *vcnId)
	}
	for _, val := range lists {
		if !strings.HasPrefix(*val.DisplayName, SecListNamePrefix) {
			continue
		}
		ret = append(ret, val)
	}
	return ret, nil
}

func (e *Environ) getSecurityList(vcnId *string) (ociCore.SecurityList, error) {
	seclist, err := e.jujuSecurityLists(vcnId)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}

	if len(seclist) > 1 {
		return ociCore.SecurityList{}, errors.Errorf("expected 1 security list, got %d", len(seclist))
	}

	if len(seclist) == 0 {
		return ociCore.SecurityList{}, errors.NotFoundf("security lists for vcn: %q", *vcnId)
	}

	return seclist[0], nil
}

func (e *Environ) ensureSecurityList(ctx context.Context, modelUUID string, vcnid *string) (ociCore.SecurityList, error) {
	if seclist, err := e.getSecurityList(vcnid); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return ociCore.SecurityList{}, errors.Trace(err)
		}
	} else {
		return seclist, nil
	}

	name := e.secListName(e.controllerUUID, modelUUID)
	logger.Debugf(ctx, "creating new security list %s", name)

	// Hopefully just temporary, open all ingress/egress ports
	prefix := AllowAllPrefix
	details := ociCore.CreateSecurityListDetails{
		CompartmentId: e.ecfg().compartmentID(),
		VcnId:         vcnid,
		DisplayName:   &name,
		FreeformTags: map[string]string{
			tags.JujuController: e.controllerUUID,
			tags.JujuMachine:    modelUUID,
		},
		EgressSecurityRules: []ociCore.EgressSecurityRule{
			{
				Destination: &prefix,
				Protocol:    &allProtocols,
			},
		},
		IngressSecurityRules: []ociCore.IngressSecurityRule{
			{
				Source:   &prefix,
				Protocol: &allProtocols,
			},
		},
	}

	request := ociCore.CreateSecurityListRequest{
		CreateSecurityListDetails: details,
	}

	response, err := e.Firewall.CreateSecurityList(context.Background(), request)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}
	logger.Debugf(ctx, "security list %s created. Waiting for status: %s",
		*response.SecurityList.Id, string(ociCore.SecurityListLifecycleStateAvailable))

	err = e.waitForResourceStatus(
		e.getSecurityListStatus, response.SecurityList.Id,
		string(ociCore.SecurityListLifecycleStateAvailable),
		resourcePollTimeout)
	if err != nil {
		return ociCore.SecurityList{}, errors.Trace(err)
	}
	return response.SecurityList, nil
}

func (e *Environ) allSubnets(
	ctx context.Context,
	controllerUUID, modelUUID string,
	vcnID *string,
) (map[string][]ociCore.Subnet, error) {
	subnets, err := e.Networking.ListSubnets(context.Background(), e.ecfg().compartmentID(), vcnID)
	if err != nil {
		return nil, err
	}

	ret := map[string][]ociCore.Subnet{}
	for _, val := range subnets {
		tag, ok := val.FreeformTags[tags.JujuController]
		if !ok || tag != controllerUUID {
			continue
		}
		if modelUUID != "" {
			tag, ok = val.FreeformTags[tags.JujuModel]
			if !ok || tag != modelUUID {
				continue
			}
		}
		cidr := *val.CidrBlock
		if valid, err := e.validateCidrBlock(cidr); err != nil || !valid {
			logger.Warningf(ctx, "failed to validate CIDR block %s: %s", cidr, err)
			continue
		}
		ret[*val.AvailabilityDomain] = append(ret[*val.AvailabilityDomain], val)
	}
	return ret, nil
}

func (e *Environ) validateCidrBlock(cidr string) (bool, error) {
	addressSpace := e.ecfg().addressSpace()
	_, vncIPNet, err := net.ParseCIDR(*addressSpace)
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
	addressSpace := e.ecfg().addressSpace()
	ip, _, err := net.ParseCIDR(*addressSpace)
	if err != nil {
		return "", errors.Trace(err)
	}
	to4 := ip.To4()
	if to4 == nil {
		return "", errors.Errorf("invalid IPv4 address: %s", *addressSpace)
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

	response, err := e.Networking.GetSubnet(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("subnet %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.Subnet.LifecycleState), nil
}

func (e *Environ) createSubnet(
	modelUUID, ad, cidr string, vcnID *string, seclists []string, routeTableId *string,
) (ociCore.Subnet, error) {
	displayName := fmt.Sprintf("juju-%s-%s-%s", ad, e.controllerUUID, modelUUID)
	compartment := e.ecfg().compartmentID()
	// TODO(gsamfira): maybe "local" would be better?
	subnetDetails := ociCore.CreateSubnetDetails{
		AvailabilityDomain: &ad,
		CidrBlock:          &cidr,
		CompartmentId:      compartment,
		VcnId:              vcnID,
		DisplayName:        &displayName,
		RouteTableId:       routeTableId,
		SecurityListIds:    seclists,
		FreeformTags: map[string]string{
			tags.JujuController: e.controllerUUID,
			tags.JujuModel:      modelUUID,
		},
	}

	request := ociCore.CreateSubnetRequest{
		CreateSubnetDetails: subnetDetails,
	}

	response, err := e.Networking.CreateSubnet(context.Background(), request)
	if err != nil {
		return ociCore.Subnet{}, errors.Trace(err)
	}
	err = e.waitForResourceStatus(
		e.getSubnetStatus, response.Subnet.Id,
		string(ociCore.SubnetLifecycleStateAvailable),
		resourcePollTimeout)
	if err != nil {
		return ociCore.Subnet{}, errors.Trace(err)
	}
	return response.Subnet, nil
}

func (e *Environ) ensureSubnets(
	ctx context.Context,
	vcn ociCore.Vcn,
	secList ociCore.SecurityList,
	modelUUID string,
	routeTableId *string,
) (map[string][]ociCore.Subnet, error) {
	az, err := e.AvailabilityZones(ctx)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	allSubnets, err := e.allSubnets(ctx, e.controllerUUID, modelUUID, vcn.Id)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	existingCidrBlocks := map[string]bool{}
	missing := map[string]bool{}
	// Check that we have one subnet in each availability domain
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
		for ad := range missing {
			newIPNet, err := e.getFreeSubnet(existingCidrBlocks)
			if err != nil {
				return nil, e.HandleCredentialError(ctx, err)
			}
			newSubnet, err := e.createSubnet(
				modelUUID, ad, newIPNet, vcn.Id, []string{*secList.Id}, routeTableId)
			if err != nil {
				return nil, e.HandleCredentialError(ctx, err)
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
func (e *Environ) ensureNetworksAndSubnets(
	ctx context.Context, modelUUID string,
) (map[string][]ociCore.Subnet, error) {
	// if we have the subnets field populated, it means we already checked/created
	// the necessary resources. Simply return.
	if e.subnets != nil {
		return e.subnets, nil
	}
	vcn, err := e.ensureVCN(ctx, modelUUID)
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
	secList, err := e.ensureSecurityList(ctx, modelUUID, vcn.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ig, err := e.ensureInternetGateway(ctx, modelUUID, vcn.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create a default route through the gateway created above
	// as a default gateway
	prefix := AllowAllPrefix
	routeRules := []ociCore.RouteRule{
		{
			Destination:     &prefix,
			DestinationType: ociCore.RouteRuleDestinationTypeCidrBlock,
			NetworkEntityId: ig.Id,
		},
	}
	routeTable, err := e.ensureRouteTable(ctx, modelUUID, vcn.Id, routeRules)
	if err != nil {
		return nil, errors.Trace(err)
	}

	subnets, err := e.ensureSubnets(ctx, vcn, secList, modelUUID, routeTable.Id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(gsamfira): should we use a lock here?
	e.subnets = subnets
	return e.subnets, nil
}

func (e *Environ) removeSubnets(subnets map[string][]ociCore.Subnet) error {
	errorMessages := []string{}
	for _, adSubnets := range subnets {
		for _, subnet := range adSubnets {
			request := ociCore.DeleteSubnetRequest{
				SubnetId: subnet.Id,
			}
			// we may need to wait for resource to be deleted
			response, err := e.Networking.DeleteSubnet(context.Background(), request)
			if err != nil && !e.isNotFound(response.RawResponse) {
				errorMessages = append(errorMessages, err.Error())
				continue
			}
			err = e.waitForResourceStatus(
				e.getSubnetStatus, subnet.Id,
				string(ociCore.SubnetLifecycleStateTerminated),
				resourcePollTimeout)
			if err != nil && !errors.Is(err, errors.NotFound) {
				errorMessages = append(errorMessages, err.Error())
				continue
			}
		}
	}
	if len(errorMessages) > 0 {
		return errors.Errorf("the following errors occurred while cleaning up subnets: %q",
			strings.Join(errorMessages, "\n"))
	}
	return nil
}

func (e *Environ) removeSecurityLists(ctx context.Context, secLists []ociCore.SecurityList) error {
	for _, secList := range secLists {
		if secList.Id == nil {
			return nil
		}
		request := ociCore.DeleteSecurityListRequest{
			SecurityListId: secList.Id,
		}
		logger.Debugf(ctx, "deleting security list %s", *secList.Id)
		response, err := e.Firewall.DeleteSecurityList(context.Background(), request)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return nil
		}
		err = e.waitForResourceStatus(
			e.getSecurityListStatus, secList.Id,
			string(ociCore.SecurityListLifecycleStateTerminated),
			resourcePollTimeout)
		if !errors.Is(err, errors.NotFound) {
			return err
		}
	}
	return nil
}

func (e *Environ) removeVCN(ctx context.Context, vcn ociCore.Vcn) error {
	if vcn.Id == nil {
		return nil
	}
	requestDeleteVcn := ociCore.DeleteVcnRequest{
		VcnId: vcn.Id,
	}

	logger.Infof(ctx, "deleting VCN: %s", *vcn.Id)
	response, err := e.Networking.DeleteVcn(context.Background(), requestDeleteVcn)
	if err != nil && !e.isNotFound(response.RawResponse) {
		return err
	}
	err = e.waitForResourceStatus(
		e.getVCNStatus, vcn.Id,
		string(ociCore.VcnLifecycleStateTerminated),
		resourcePollTimeout)
	if !errors.Is(err, errors.NotFound) {
		return err
	}
	return nil
}

// cleanupNetworksAndSubnets destroys all subnets, VCNs and security lists that have
// been used by this juju deployment. This function should only be called when
// destroying the environment, and only after destroying any resources that may be attached
// to a network.
func (e *Environ) cleanupNetworksAndSubnets(ctx context.Context, controllerUUID, modelUUID string) error {
	vcns, err := e.allVCNs(controllerUUID, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	if len(vcns) == 0 {
		return nil
	}

	for _, vcn := range vcns {
		allSubnets, err := e.allSubnets(ctx, controllerUUID, modelUUID, vcn.Id)
		if err != nil {
			return errors.Trace(err)
		}

		if err := e.removeSubnets(allSubnets); err != nil {
			return errors.Trace(err)
		}

		secLists, err := e.jujuSecurityLists(vcn.Id)
		if err != nil {
			return errors.Trace(err)
		}
		if err := e.removeSecurityLists(ctx, secLists); err != nil {
			return errors.Trace(err)
		}

		if err := e.deleteRouteTable(controllerUUID, modelUUID, vcn.Id); err != nil {
			return errors.Trace(err)
		}

		if err := e.deleteInternetGateway(vcn.Id); err != nil {
			return errors.Trace(err)
		}

		if err := e.removeVCN(ctx, vcn); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *Environ) getInternetGatewayStatus(resourceID *string) (string, error) {
	request := ociCore.GetInternetGatewayRequest{
		IgId: resourceID,
	}

	response, err := e.Networking.GetInternetGateway(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("internet gateway %s", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.InternetGateway.LifecycleState), nil
}

func (e *Environ) getInternetGateway(vcnID *string) (ociCore.InternetGateway, error) {
	if vcnID == nil {
		return ociCore.InternetGateway{}, errors.Errorf("vcnID may not be nil")
	}

	internetGateways, err := e.Networking.ListInternetGateways(context.Background(), e.ecfg().compartmentID(), vcnID)
	if err != nil {
		return ociCore.InternetGateway{}, errors.Trace(err)
	}
	if len(internetGateways) == 0 {
		return ociCore.InternetGateway{}, errors.NotFoundf("internet gateways for vcn %q", *vcnID)
	}

	return internetGateways[0], nil
}

func (e *Environ) internetGatewayName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", InternetGatewayPrefix, controllerUUID, modelUUID)
}

func (e *Environ) ensureInternetGateway(ctx context.Context, modelUUID string, vcnID *string) (ociCore.InternetGateway, error) {
	if ig, err := e.getInternetGateway(vcnID); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return ociCore.InternetGateway{}, errors.Trace(err)
		}
	} else {
		return ig, nil
	}

	name := e.internetGatewayName(e.controllerUUID, modelUUID)
	logger.Debugf(ctx, "creating new internet gateway %s", name)

	enabled := true
	details := ociCore.CreateInternetGatewayDetails{
		VcnId:         vcnID,
		CompartmentId: e.ecfg().compartmentID(),
		IsEnabled:     &enabled,
		DisplayName:   &name,
	}

	request := ociCore.CreateInternetGatewayRequest{
		CreateInternetGatewayDetails: details,
	}

	response, err := e.Networking.CreateInternetGateway(context.Background(), request)
	if err != nil {
		return ociCore.InternetGateway{}, errors.Trace(err)
	}

	if err := e.waitForResourceStatus(
		e.getInternetGatewayStatus,
		response.InternetGateway.Id,
		string(ociCore.InternetGatewayLifecycleStateAvailable),
		resourcePollTimeout); err != nil {

		return ociCore.InternetGateway{}, errors.Trace(err)
	}

	return response.InternetGateway, nil
}

func (e *Environ) deleteInternetGateway(vcnID *string) error {
	ig, err := e.getInternetGateway(vcnID)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
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

		response, err := e.Networking.DeleteInternetGateway(context.Background(), request)
		if err != nil && !e.isNotFound(response.RawResponse) {
			return errors.Trace(err)
		}
	}
	if err := e.waitForResourceStatus(
		e.getInternetGatewayStatus,
		ig.Id,
		string(terminatedStatus),
		resourcePollTimeout); err != nil {

		if !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
	}

	return nil
}

// jujuRouteTables returns the route tables for the input VCN
// that were created by juju.
func (e *Environ) jujuRouteTables(vcnId *string) ([]ociCore.RouteTable, error) {
	var ret []ociCore.RouteTable
	if vcnId == nil {
		return ret, errors.Errorf("vcnId may not be nil")
	}

	routeTables, err := e.Networking.ListRouteTables(context.Background(), e.ecfg().compartmentID(), vcnId)
	if err != nil {
		return ret, errors.Trace(err)
	}

	for _, val := range routeTables {
		if !strings.HasPrefix(*val.DisplayName, RouteTablePrefix) {
			continue
		}
		ret = append(ret, val)
	}
	return ret, nil
}

func (e *Environ) getRouteTable(vcnId *string) (ociCore.RouteTable, error) {
	routeTables, err := e.jujuRouteTables(vcnId)
	if err != nil {
		return ociCore.RouteTable{}, errors.Trace(err)
	}

	if len(routeTables) > 1 {
		return ociCore.RouteTable{}, errors.Errorf("expected 1 route table, got %d", len(routeTables))
	}

	if len(routeTables) == 0 {
		return ociCore.RouteTable{}, errors.NotFoundf("route table for VCN %q", *vcnId)
	}

	return routeTables[0], nil
}

func (e *Environ) routeTableName(controllerUUID, modelUUID string) string {
	return fmt.Sprintf("%s-%s-%s", RouteTablePrefix, controllerUUID, modelUUID)
}

func (e *Environ) getRouteTableStatus(resourceID *string) (string, error) {
	if resourceID == nil {
		return "", errors.Errorf("resourceID may not be nil")
	}
	request := ociCore.GetRouteTableRequest{
		RtId: resourceID,
	}

	response, err := e.Networking.GetRouteTable(context.Background(), request)
	if err != nil {
		if e.isNotFound(response.RawResponse) {
			return "", errors.NotFoundf("route table %q", *resourceID)
		} else {
			return "", err
		}
	}
	return string(response.RouteTable.LifecycleState), nil
}

func (e *Environ) ensureRouteTable(
	ctx context.Context,
	modelUUID string, vcnId *string, routeRules []ociCore.RouteRule,
) (ociCore.RouteTable, error) {
	if rt, err := e.getRouteTable(vcnId); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return ociCore.RouteTable{}, errors.Trace(err)
		}
	} else {
		return rt, nil
	}

	name := e.routeTableName(e.controllerUUID, modelUUID)
	logger.Debugf(ctx, "creating new route table %s", name)

	details := ociCore.CreateRouteTableDetails{
		VcnId:         vcnId,
		CompartmentId: e.ecfg().compartmentID(),
		RouteRules:    routeRules,
		DisplayName:   &name,
		FreeformTags: map[string]string{
			tags.JujuController: e.controllerUUID,
			tags.JujuModel:      modelUUID,
		},
	}

	request := ociCore.CreateRouteTableRequest{
		CreateRouteTableDetails: details,
	}

	response, err := e.Networking.CreateRouteTable(context.Background(), request)
	if err != nil {
		return ociCore.RouteTable{}, errors.Trace(err)
	}
	logger.Debugf(ctx, "route table %s created. Waiting for status: %s",
		*response.RouteTable.Id, string(ociCore.RouteTableLifecycleStateAvailable))

	if err := e.waitForResourceStatus(
		e.getRouteTableStatus,
		response.RouteTable.Id,
		string(ociCore.RouteTableLifecycleStateAvailable),
		resourcePollTimeout,
	); err != nil {
		return ociCore.RouteTable{}, errors.Trace(err)
	}

	return response.RouteTable, nil
}

func (e *Environ) deleteRouteTable(controllerUUID, modelUUID string, vcnId *string) error {
	rts, err := e.jujuRouteTables(vcnId)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return err
		}
		return nil
	}

	for _, rt := range rts {
		if rt.LifecycleState == ociCore.RouteTableLifecycleStateTerminated {
			return nil
		}

		if rt.LifecycleState != ociCore.RouteTableLifecycleStateTerminating {
			request := ociCore.DeleteRouteTableRequest{
				RtId: rt.Id,
			}

			response, err := e.Networking.DeleteRouteTable(context.Background(), request)
			if err != nil && !e.isNotFound(response.RawResponse) {
				return errors.Trace(err)
			}
		}

		if err := e.waitForResourceStatus(
			e.getRouteTableStatus,
			rt.Id,
			string(ociCore.RouteTableLifecycleStateTerminated),
			resourcePollTimeout); err != nil {

			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (e *Environ) allSubnetsAsMap(modelUUID string) (map[string]ociCore.Subnet, error) {
	vcns, err := e.Networking.ListVcns(context.Background(), e.ecfg().compartmentID())
	if err != nil {
		return map[string]ociCore.Subnet{}, errors.Trace(err)
	}

	result := map[string]ociCore.Subnet{}
	for _, vcn := range vcns {
		if modelUUID != "" {
			tag, ok := vcn.FreeformTags[tags.JujuModel]
			if !ok || tag != modelUUID {
				continue
			}
		}
		subnets, err := e.Networking.ListSubnets(context.Background(), e.ecfg().compartmentID(), vcn.Id)
		if err != nil {
			return map[string]ociCore.Subnet{}, err
		}
		for _, subnet := range subnets {
			if subnet.Id == nil {
				continue
			}
			result[*subnet.Id] = subnet
		}
	}
	return result, nil
}

// Subnets is defined on the environs.Networking interface.
// This implementation also creates the subnets if none exist.
func (e *Environ) Subnets(
	ctx context.Context, subnets []network.Id,
) ([]network.SubnetInfo, error) {
	var results []network.SubnetInfo
	subIdSet := set.NewStrings()
	for _, subId := range subnets {
		subIdSet.Add(string(subId))
	}

	allSubnets, err := e.allSubnetsAsMap(e.Config().UUID())
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	// If no subnets exist for this model, create them.
	if len(allSubnets) == 0 {
		subnetMap, err := e.ensureNetworksAndSubnets(ctx, e.Config().UUID())
		if err != nil {
			return nil, errors.Trace(err)
		}
		allSubnets = make(map[string]ociCore.Subnet, len(subnetMap))
		for _, subnets := range subnetMap {
			for _, subnet := range subnets {
				allSubnets[*subnet.Id] = subnet
			}
		}
	}

	hasSubnetList := false
	if len(subIdSet) > 0 {
		hasSubnetList = true
	}
	for subnetId, subnet := range allSubnets {
		if hasSubnetList {
			if !subIdSet.Contains(subnetId) {
				continue
			} else {
				subIdSet.Remove(subnetId)
			}
		}
		if info, err := makeSubnetInfo(subnet); err == nil {
			results = append(results, info)
		}
	}
	if hasSubnetList && !subIdSet.IsEmpty() {
		return nil, errors.Errorf("failed to find the following subnet ids: %v", subIdSet.Values())
	}

	// Sort the list of subnets to ensure consistency in what we display
	// to the user
	sort.Slice(results, func(i, j int) bool {
		return results[i].ProviderId < results[j].ProviderId
	})

	return results, nil
}

func makeSubnetInfo(subnet ociCore.Subnet) (network.SubnetInfo, error) {
	if subnet.CidrBlock == nil {
		return network.SubnetInfo{}, errors.Errorf("nil cidr block in subnet")
	}
	_, _, err := net.ParseCIDR(*subnet.CidrBlock)
	if err != nil {
		return network.SubnetInfo{}, errors.Annotatef(err, "skipping subnet %q, invalid CIDR", *subnet.CidrBlock)
	}

	info := network.SubnetInfo{
		CIDR:              *subnet.CidrBlock,
		ProviderId:        network.Id(*subnet.Id),
		AvailabilityZones: []string{*subnet.AvailabilityDomain},
	}
	return info, nil
}

func (e *Environ) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	var (
		infos = make([]network.InterfaceInfos, len(ids))
		err   error
	)

	for idx, id := range ids {
		if infos[idx], err = e.networkInterfacesForInstance(ctx, id); err != nil {
			return nil, err
		}
	}

	return infos, nil
}

func (e *Environ) networkInterfacesForInstance(ctx context.Context, instId instance.Id) (network.InterfaceInfos, error) {
	oInst, err := e.getOCIInstance(ctx, instId)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	info := network.InterfaceInfos{}
	vnics, err := oInst.getVnics()
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	subnets, err := e.allSubnetsAsMap(e.Config().UUID())
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	for _, iface := range vnics {
		if iface.Vnic.Id == nil || iface.Vnic.MacAddress == nil || iface.Vnic.SubnetId == nil {
			continue
		}
		subnet, ok := subnets[*iface.Vnic.SubnetId]
		if !ok || subnet.CidrBlock == nil {
			continue
		}
		// Provider does not support interface names.
		nic := network.InterfaceInfo{
			DeviceIndex: iface.Idx,
			ProviderId:  network.Id(*iface.Vnic.Id),
			MACAddress:  *iface.Vnic.MacAddress,
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress(
					*iface.Vnic.PrivateIp,
					network.WithScope(network.ScopeCloudLocal),
					network.WithCIDR(*subnet.CidrBlock),
				).AsProviderAddress(),
			},
			InterfaceType:    network.EthernetDevice,
			ProviderSubnetId: network.Id(*iface.Vnic.SubnetId),
			Origin:           network.OriginProvider,
		}
		if iface.Vnic.PublicIp != nil {
			nic.ShadowAddresses = append(nic.ShadowAddresses,
				network.NewMachineAddress(
					*iface.Vnic.PublicIp,
					network.WithScope(network.ScopePublic),
				).AsProviderAddress(),
			)
		}
		info = append(info, nic)
	}
	return info, nil
}

func (e *Environ) SupportsSpaces() (bool, error) {
	return false, nil
}
