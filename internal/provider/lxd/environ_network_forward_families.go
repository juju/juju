// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"maps"
	"net"
	"slices"
	"strings"

	"github.com/canonical/lxd/shared"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
)

type ovnForwardFamily struct {
	version int
	subnet  *net.IPNet
}

// ovnForwardFamilies selects families with both a guest subnet and an external
// allocation range. Capacity and quota checks remain authoritative in LXD.
func ovnForwardFamilies(ctx context.Context, srv Server, networks map[string]api.Network) (map[string][]ovnForwardFamily, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := srv.GetConnectionInfo()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving LXD connection info")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectName := info.Project
	if projectName == "" {
		projectName = api.ProjectDefaultName
	}
	project, _, err := srv.GetProject(projectName)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving LXD project %q", projectName)
	}
	// Projects without their own networks inherit the default project's
	// networks. LXD allocates forwards using that network's project settings.
	if projectName != api.ProjectDefaultName && !shared.IsTrue(project.Config["features.networks"]) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		project, _, err = srv.GetProject(api.ProjectDefaultName)
		if err != nil {
			return nil, errors.Annotate(err, "retrieving default LXD project")
		}
	}

	families := make(map[string][]ovnForwardFamily)
	uplinkSubnets := make(map[string][]*net.IPNet)
	for _, name := range slices.Sorted(maps.Keys(networks)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		network := networks[name]
		uplink := network.Config["network"]
		if uplink == "" {
			return nil, errors.Errorf("OVN network %q has no uplink", name)
		}
		externalSubnets, ok := uplinkSubnets[uplink]
		if !ok {
			externalSubnets, err = ovnExternalSubnets(ctx, srv, uplink, project)
			if err != nil {
				return nil, errors.Annotatef(err, "retrieving external allocation ranges for OVN network %q", name)
			}
			uplinkSubnets[uplink] = externalSubnets
		}
		for _, version := range []int{4, 6} {
			key := "ipv4.address"
			if version == 6 {
				key = "ipv6.address"
			}
			value := network.Config[key]
			if value == "" || value == "none" {
				continue
			}
			_, subnet, err := net.ParseCIDR(value)
			if err != nil || (subnet.IP.To4() != nil) != (version == 4) {
				return nil, errors.Errorf("invalid %s %q on OVN network %q", key, value, name)
			}
			for _, external := range externalSubnets {
				if (external.IP.To4() != nil) == (version == 4) {
					families[name] = append(families[name], ovnForwardFamily{version: version, subnet: subnet})
					break
				}
			}
		}
		if len(families[name]) == 0 {
			return nil, errors.Errorf("OVN network %q has no address family with a permitted external allocation range", name)
		}
	}
	return families, ctx.Err()
}

func ovnExternalSubnets(ctx context.Context, srv Server, uplink string, project *api.Project) ([]*net.IPNet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var ranges []string
	if restrictions := project.Config["restricted.networks.subnets"]; shared.IsTrue(project.Config["restricted"]) && restrictions != "" {
		// Match LXD's allocator: restricted project subnets replace the
		// uplink routes as the source of candidate allocation ranges.
		for entry := range strings.SplitSeq(restrictions, ",") {
			name, subnet, ok := strings.Cut(strings.TrimSpace(entry), ":")
			if !ok {
				return nil, errors.Errorf("invalid project subnet restriction %q", entry)
			}
			if name == uplink {
				ranges = append(ranges, subnet)
			}
		}
	} else {
		// Uplinks live in the default project even for project-local OVN
		// networks. The lookup must not change the instance client's project.
		network, _, err := srv.GetNetworkInProject(uplink, api.ProjectDefaultName)
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving uplink network %q", uplink)
		}
		for _, key := range []string{"ipv4.routes", "ipv6.routes"} {
			ranges = append(ranges, strings.Split(network.Config[key], ",")...)
		}
	}
	var subnets []*net.IPNet
	for _, value := range ranges {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, subnet, err := net.ParseCIDR(value)
		if err != nil {
			return nil, errors.Annotatef(err, "parsing external allocation range %q", value)
		}
		subnets = append(subnets, subnet)
	}
	return subnets, nil
}
