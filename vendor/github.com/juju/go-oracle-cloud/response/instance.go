// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

type LaunchPlan struct {
	Relationships []string   `json:"relationships,omitempty"`
	Instances     []Instance `json:"instances"`
}

// AllInstances a slice of all instances in the
// oracle cloud account
type AllInstances struct {
	Result []Instance `json:"result"`
}

// AllInstanceNames a slice of all instance
// names in the oracle cloud account
type AllInstanceNames struct {
	Result []string `json:"result"`
}

// Instance represents an Oracle Compute Cloud Service
// instance is a virtual machine running a specific
// operating system and with CPU and memory resources that you specify.
type Instance struct {
	Domain                          string               `json:"domain"`
	Placement_requirements          []string             `json:"placement_requirements"`
	Ip                              string               `json:"ip"`
	Fingerprint                     string               `json:"fingerprint,omitempty"`
	Site                            string               `json:"site,omitempty"`
	Last_state_change_time          interface{}          `json:"last_state_change_time,omitempty"`
	Error_exception                 interface{}          `json:"error_exception,omitempty"`
	Cluster                         interface{}          `json:"cluster,omitempty"`
	Shape                           string               `json:"shape"`
	Start_requested                 bool                 `json:"start_requested"`
	Vethernets                      interface{}          `json:"vethernets,omitempty"`
	Imagelist                       string               `json:"imagelist,omitempty"`
	Image_format                    string               `json:"image_format"`
	Cluster_uri                     interface{}          `json:"cluster_uri,omitempty"`
	Relationships                   []string             `json:"relationships,omitempty"`
	Target_node                     interface{}          `json:"target_node,omitempty"`
	Availability_domain             interface{}          `json:"availability_domain,omitempty"`
	Networking                      common.Networking    `json:"networking"`
	Seclist_associations            interface{}          `json:"seclist_associations,omitempty"`
	Hostname                        string               `json:"hostname"`
	State                           common.InstanceState `json:"state"`
	Disk_attach                     string               `json:"disk_attach,omitempty"`
	Label                           string               `json:"label,omitempty"`
	Priority                        string               `json:"priority"`
	Platform                        string               `json:"platform"`
	Quota_reservation               interface{}          `json:"quota_reservation,omitempty"`
	Suspend_file                    interface{}          `json:"suspend_file,omitempty"`
	Node                            interface{}          `json:"node,omitempty"`
	Resource_requirements           ResourceRequirments  `json:"resource_requirements"`
	Virtio                          interface{}          `json:"virtio,omitempty"`
	Vnc                             string               `json:"vnc,omitempty"`
	Desired_state                   common.InstanceState `json:"desired_state"`
	Storage_attachments             []Storage            `json:"storage_attachments,omitempty"`
	Start_time                      string               `json:"start_time"`
	Storage_attachment_associations []interface{}        `json:"storage_attachment_associations,omitempty"`
	Quota                           string               `json:"quota"`
	Vnc_key                         interface{}          `json:"vnc_key,omitempty"`
	Numerical_priority              uint64               `json:"numerical_priority"`
	Suspend_requested               bool                 `json:"suspend_requested"`
	Entry                           int                  `json:"entry"`
	Error_reason                    string               `json:"error_reason,omitempty"`
	Nat_associations                interface{}          `json:"nat_associations,omitempty"`
	SSHKeys                         []string             `json:"sshkeys,omitemtpy"`
	Tags                            []string             `json:"tags,omitempty"`
	Resolvers                       interface{}          `json:"resolvers,omitempty"`
	Metrics                         interface{}          `json:"metrics,omitempty"`
	Account                         string               `json:"account"`
	Node_uuid                       interface{}          `json:"node_uuid,omitempty"`
	Name                            string               `json:"name"`
	Vcable_id                       common.VcableID      `json:"vcable_id,omitempty"`
	Higgs                           interface{}          `json:"higgs,omitempty"`
	Hypervisor                      Hypervisor           `json:"hypervisor"`
	Uri                             string               `json:"uri"`
	Console                         interface{}          `json:"console,omitempty"`
	Reverse_dns                     bool                 `json:"reverse_dns"`
	Launch_context                  string               `json:"launch_context"`
	Delete_requested                interface{}          `json:"delete_requested,omitempty"`
	Tracking_id                     interface{}          `json:"tracking_id,omitempty"`
	Hypervisor_type                 interface{}          `json:"hypervisor_type,omitempty"`
	Attributes                      Attributes           `json:"attributes"`
	Boot_order                      []int                `json:"boot_order,omitempty"`
	Last_seen                       interface{}          `json:"last_seen,omitempty"`
}

// Attributes holds a map of attributes that is returned from
// the instance response
// This attributes can have user data scripts or any other
// key, value passed to be executed when the instance will start, inits
type Attributes struct {
	Dns                   map[string]string  `json:"dns"`
	Network               map[string]Network `json:"network"`
	Nimbula_orchestration string             `json:"nimbula_orchestration"`
	Sshkeys               []string           `json:"sshkeys"`
	Userdata              interface{}        `json:"userdata"`
}

// Userdata key value pair
// In order to read elements from it you should use the build in
// function for this type StringValue
type Userdata map[string]interface{}

// StringValue returns a string from a given key in the userdata
func (u Userdata) StringValue(key string) string {
	if u == nil {
		return ""
	}

	val, ok := u[key].(string)
	if !ok {
		return ""
	}

	return val
}

type Network struct {
	// The MAC address of the interface, in hexadecimal format
	// This can contains ipv4 addresses also
	Address []string `json:"address"`

	Dhcp_options []string `json:"dhcp_options,omitempty"`

	// Id you want to associate a static private
	// IP address with the instance, specify
	// an available IP address from the IP
	// address range of the specified ipnetwork.
	Id string `json:"id"`

	// Model is the type of network
	// interface card (NIC).
	// The only allowed value is e1000.
	Model string `json:"model"`

	Vethernet string `json:"vethernet"`

	Vethernet_id string `json:"vethernet_id"`

	Vethernet_type string `josn:"vethernet_type"`

	Instance string `json:"instance,omitmepty"`

	Ipassociations []string `json:"ipassociations,omitempty"`

	Ipattachment string `json:"ipattachment"`

	// Ipnetwork is the name of the IP network
	// that you want to add the instance to
	Ipnetwork string `json:"ipnetwork"`

	// Nnic is the virtual nic
	Vnic string `json:"vnic"`

	// Vnicsets are names of the vNICsets
	// that you want to add this interface to
	Vnicsets []string `json:"vnicsets"`
}

type Dns struct {
	Domain      string `json:"domain"`
	Hostname    string `json:"hostname"`
	Vcable_eth0 string `json:"nimbula_vcable-eth0"`
}

type Storage struct {
	Index               uint64 `json:"index"`
	Storage_volume_name string `json:"storage_volume_name"`
	Name                string `json:"name"`
}

type Hypervisor struct {
	Mode string `json:"mode"`
}

type ResourceRequirments struct {
	Compressed_size   uint64  `json:"compressed_size"`
	Is_root_ssd       bool    `json:"is_root_ssd"`
	Ram               uint64  `json:"ram"`
	Cpus              float64 `json:"cpus"`
	Root_disk_size    uint64  `json:"root_disk_size"`
	Io                uint64  `json:"io"`
	Decompressed_size uint64  `json:"decompressed_size"`
	Gpus              uint64  `json:"gpus"`
	Ssd_data_size     uint64  `json:"ssd_data_size"`
}
