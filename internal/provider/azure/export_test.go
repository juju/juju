// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"

	azurenetwork "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

const ComputeAPIVersion = computeAPIVersion
const NetworkAPIVersion = networkAPIVersion

var NetworkTemplateResources = networkTemplateResources
var SubnetProviderIDForFamily = subnetProviderIDForFamily
var StripIPFamilySuffix = stripIPFamilySuffix
var StripAndDeduplicateSubnetIDs = stripAndDeduplicateSubnetIDs

const SecurityGroupName = internalSecurityGroupName
const InternalNetworkName = internalNetworkName
const InternalSubnetName = internalSubnetName
const ControllerSubnetName = controllerSubnetName
const InternalSubnetPrefix = internalSubnetPrefix
const ControllerSubnetPrefix = controllerSubnetPrefix
const InternalSubnetIPv6Prefix = internalSubnetIPv6Prefix
const ControllerSubnetIPv6Prefix = controllerSubnetIPv6Prefix
const VnetIPv6Prefix = vnetIPv6Prefix

func FindSubnetByID(ctx context.Context, env environs.Environ, id network.Id) (*azurenetwork.Subnet, error) {
	azEnv, ok := env.(*azureEnviron)
	if !ok {
		return nil, errors.Errorf("expected *azureEnviron, got %T", env)
	}
	return azEnv.findSubnetByID(ctx, id)
}
