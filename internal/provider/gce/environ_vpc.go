// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	stdcontext "context"
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/gce/internal/google"
)

const (
	// errorVPCNotRecommended indicates a user-specified VPC is unlikely to be
	// suitable for hosting a Juju controller instance and/or exposed workloads,
	// due to not satisfying the minimum requirements described in
	// validateVPC()'s doc comment. Users can still force Juju to use such a
	// VPC by passing 'vpc-id-force=true' setting.
	errorVPCNotRecommended = errors.ConstError("vpc not recommended")

	// errorVPCNotUsable indicates a user-specified VPC cannot be used either
	// because it is missing or because it contains no ready subnets.
	errorVPCNotUsable = errors.ConstError("vpc not usable")
)

var (
	vpcNotUsableForBootstrapErrorPrefix = `
Juju cannot use the given vpc-id for bootstrapping a controller
instance. Please, double check the given VPC ID is correct, and that
the VPC contains at least one subnet.

Error details`[1:]

	vpcNotRecommendedErrorPrefix = `
The given vpc-id does not meet one or more of the following minimum
Juju requirements:

1. VPC should have one subnet in the "READY" state
   or have "AutoCreateSubnetworks" set to True.

Error details`[1:]

	vpcNotUsableForModelErrorPrefix = `
Juju cannot use the given vpc-id for the model being added.
Please double check the given VPC ID is correct, and that
the VPC contains at least one ready subnet or has
"AutoCreateSubnetworks" set to True.

Error details`[1:]

	cannotValidateVPCErrorPrefix = `
Juju could not verify whether the given vpc-id meets the minimum Juju
connectivity requirements. Please, double check the VPC ID is correct,
you have a working connection to the Internet, your GCE credentials are
sufficient to access VPC features.

Error details`[1:]

	vpcNotRecommendedButForcedWarning = `
WARNING! The specified vpc-id does not satisfy the minimum Juju requirements,
but will be used anyway because vpc-id-force=true is also specified.

`[1:]
)

func validateBootstrapVPC(ctx environs.BootstrapContext, conn ComputeService, region, vpcID string, force bool) error {
	if vpcID == google.NetworkDefaultName {
		ctx.Infof("Using GCE default VPC in region %q", region)
	}

	err := validateVPC(ctx.Context(), conn, region, vpcID, true)
	switch {
	case errors.Is(err, errorVPCNotUsable):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForBootstrapErrorPrefix)
	case errors.Is(err, errorVPCNotRecommended):
		// VPC does not meet minimum validation criteria.
		if !force {
			return errors.Annotatef(err, vpcNotRecommendedErrorPrefix, vpcID)
		}
		ctx.Infof(vpcNotRecommendedButForcedWarning)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(err, cannotValidateVPCErrorPrefix)
	}

	ctx.Infof("Using VPC %q in region %q", vpcID, region)

	return nil
}

func autoCreateSubnets(n *computepb.Network) bool {
	return n.AutoCreateSubnetworks == nil || *n.AutoCreateSubnetworks
}

// validateVPC requires both arguments to be set and validates that vpcID refers
// to an existing GCE VPC (default or non-default).
// Returns an error satisfying isVPCNotUsableError() when the VPC with the given
// vpcID cannot be found, or when the VPC exists but contains no subnets.
// Returns an error satisfying isVPCNotRecommendedError() in the following
// cases:
//
//  1. The VPC does not have a subnet whose state is "READY".
//  2. ... possibly more requirements will be added if needed.
//
// With the vpc-id-force config setting set to true, the provider can ignore a
// vpcNotRecommendedError. A vpcNotUsableError cannot be ignored, while
// unexpected API responses and errors could be retried.
func validateVPC(ctx stdcontext.Context, conn ComputeService, region, vpcID string, bootstrap bool) error {
	if vpcID == "" || conn == nil {
		return errors.Errorf("invalid arguments: empty VPC ID or nil client")
	}

	vpc, err := getVPCByID(ctx, conn, vpcID)
	if err != nil {
		return errors.Trace(err)
	}
	// The network is unusable if there's no subnets and it's not
	// been configured to auto create them.
	if !autoCreateSubnets(vpc) && len(vpc.Subnetworks) == 0 {
		return fmt.Errorf("VPC does not auto create subnets and has no subnet%w", errors.Hide(errorVPCNotUsable))
	}
	if bootstrap {
		// When bootstrapping we need to ensure there's ssh access to the network
		// being used or else it will fail.
		canSSH, err := haveSSHAccess(ctx, conn, vpc.GetSelfLink())
		if err != nil {
			return errors.Trace(err)
		}
		if !canSSH {
			return fmt.Errorf("VPC does not allow ssh access%w", errors.Hide(errorVPCNotUsable))
		}
	}

	if !autoCreateSubnets(vpc) {
		// Ensure there's at least one available subnet.
		subnets, err := conn.Subnetworks(ctx, region, vpc.Subnetworks...)
		if err != nil {
			return errors.Trace(err)
		}
		availableSubnet, err := findFirstAvailableSubnet(subnets)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Infof(
			"found subnet %q (%s) suitable for a Juju controller instance",
			availableSubnet.GetName(), availableSubnet.GetIpCidrRange(),
		)
	}

	logger.Infof("VPC %q is suitable for Juju controllers and expose-able workloads", vpcID)
	return nil
}

// haveSSHAccess returns true if there's a firewall rule which allows ssh access
// to the specified network. This is needed for bootstrap.
// We don't do any checks for source CDIR range as the only foolproof check
// would be ingress to all; we assume that if there's a rule in place for ssh,
// it has been configured to suit the project's requirements.
func haveSSHAccess(ctx stdcontext.Context, conn ComputeService, networkURL string) (bool, error) {
	firewalls, err := conn.NetworkFirewalls(ctx, networkURL)
	if err != nil {
		return false, errors.Trace(err)
	}
	haveSSH := false
done:
	for _, fw := range firewalls {
		rules := fw.GetAllowed()
		for _, rule := range rules {
			if rule.GetIPProtocol() != "tcp" {
				continue
			}
			ports := set.NewStrings(rule.GetPorts()...)
			if ports.Contains("22") {
				haveSSH = true
				break done
			}
		}
	}
	return haveSSH, nil
}

func getVPCByID(ctx stdcontext.Context, conn ComputeService, vpcID string) (*computepb.Network, error) {
	network, err := conn.Network(ctx, vpcID)
	if errors.Is(err, errors.NotFound) {
		return nil, fmt.Errorf("VPC %q %w%w", vpcID, errors.NotFound, errors.Hide(errorVPCNotUsable))
	} else if err != nil {
		return nil, errors.Annotatef(err, "unexpected AWS response getting VPC %q", vpcID)
	}
	return network, nil
}

func findFirstAvailableSubnet(subnets []*computepb.Subnetwork) (*computepb.Subnetwork, error) {
	for _, subnet := range subnets {
		logger.Debugf("found subnet %q with state %q", subnet.GetName(), subnet.GetState())
		if state := subnet.GetState(); state != "" && state != string(google.NetworkStatusReady) {
			continue
		}
		return subnet, nil
	}
	return nil, fmt.Errorf("VPC contains no available subnets%w", errors.Hide(errorVPCNotRecommended))
}

func validateModelVPC(ctx context.ProviderCallContext, conn ComputeService, region, modelName, vpcID string) error {
	err := validateVPC(ctx, conn, region, vpcID, false)
	switch {
	case errors.Is(err, errorVPCNotUsable):
		// VPC missing or has no subnets at all.
		return errors.Annotate(err, vpcNotUsableForModelErrorPrefix)
	case errors.Is(err, errorVPCNotRecommended):
		// VPC does not meet minimum validation criteria, but that's less
		// important for hosted models, as the controller is already accessible.
		logger.Infof(
			"Juju will use, but does not recommend using VPC %q: %v",
			vpcID, err.Error(),
		)
	case err != nil:
		// Anything else unexpected while validating the VPC.
		return errors.Annotate(google.HandleCredentialError(errors.Trace(err), ctx), cannotValidateVPCErrorPrefix)
	}
	logger.Infof("Using VPC %q for model %q", vpcID, modelName)

	return nil
}
