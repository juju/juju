// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

// IPv4 describes properties of IPv4 address
type IPv4 struct {
	Conf string    `json:"conf,omitempty"`
	IP   *Resource `json:"ip,omitempty"`
}

// RuntimeNetworkStat describes runtime I/O statistic for network interface card at runtime
type RuntimeNetworkStat struct {
	BytesRecv   int64 `json:"bytes_recv"`
	BytesSent   int64 `json:"bytes_sent"`
	PacketsRecv int64 `json:"packets_recv"`
	PacketsSent int64 `json:"packets_sent"`
}

// RuntimeNetwork describes properties of network interface card at runtime
type RuntimeNetwork struct {
	InterfaceType string             `json:"interface_type,omitempty"`
	IO            RuntimeNetworkStat `json:"io,omitempty"`
	IPv4          *Resource          `json:"ip_v4,omitempty"`
}

// NIC describes properties of network interface card
type NIC struct {
	IPv4    *IPv4           `json:"ip_v4_conf,omitempty"`
	Model   string          `json:"model,omitempty"`
	MAC     string          `json:"mac,omitempty"`
	VLAN    *Resource       `json:"vlan,omitempty"`
	Runtime *RuntimeNetwork `json:"runtime,omitempty"`
}
