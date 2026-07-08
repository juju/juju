// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/juju/juju/core/network"
)

// normalizeSubnets parses each candidate CIDR, replaces it with its masked
// canonical form, dedupes (on the masked form) and sorts. Invalid CIDRs are
// logged at debug and skipped. Child blocks are not collapsed into supernets.
func normalizeSubnets(ctx context.Context, cidrs []string) []network.SubnetInfo {
	seen := make(map[string]struct{})
	result := make([]network.SubnetInfo, 0, len(cidrs))
	for _, candidate := range cidrs {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(candidate)
		if err != nil {
			logger.Debugf(ctx, "ignoring invalid pod CIDR %q: %v", candidate, err)
			continue
		}
		cidr := ipNet.String()
		if _, exists := seen[cidr]; exists {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, network.SubnetInfo{
			CIDR:       cidr,
			ProviderId: network.Id(cidr),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CIDR < result[j].CIDR
	})
	return result
}

// splitCIDRCandidates splits a raw annotation/ConfigMap value into individual
// CIDR candidates on commas, semicolons and whitespace.
func splitCIDRCandidates(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
}
