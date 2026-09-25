// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_2_0

import (
	"context"

	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/domain/export/types/v4_2_0"
)

// deltas is the engineer-owned implementation of the Deltas interface
// declared in transform.go. When Deltas has methods, add receivers on
// this type or the package will not compile.
type deltas struct{}

var _ Deltas = deltas{}

// NewDeltas returns the engineer-written delta implementation for the
// 4.1.0 -> 4.2.0 transform.
func NewDeltas() Deltas { return deltas{} }

// IpAddress converts v4_1_0 IpAddress rows to v4_2_0. The device_uuid column
// became nullable in 4.2.0 (Kubernetes pod and service addresses no longer
// carry a placeholder link-layer device). Addresses exported from a 4.1.0
// model still reference a real device, so the value is carried through as a
// non-nil pointer; an empty value is defensively mapped to nil so import
// cannot fail the ip_address device foreign key on a zero UUID.
func (d deltas) IpAddress(_ context.Context, src []v4_1_0.IpAddress) ([]v4_2_0.IpAddress, error) {
	result := make([]v4_2_0.IpAddress, len(src))
	for i := range src {
		result[i] = v4_2_0.IpAddress{
			UUID:         src[i].UUID,
			NetNodeUUID:  src[i].NetNodeUUID,
			AddressValue: src[i].AddressValue,
			SubnetUUID:   src[i].SubnetUUID,
			TypeID:       src[i].TypeID,
			ConfigTypeID: src[i].ConfigTypeID,
			OriginID:     src[i].OriginID,
			ScopeID:      src[i].ScopeID,
			IsSecondary:  src[i].IsSecondary,
			IsShadow:     src[i].IsShadow,
		}
		if src[i].DeviceUUID != "" {
			result[i].DeviceUUID = &src[i].DeviceUUID
		}
	}
	return result, nil
}
