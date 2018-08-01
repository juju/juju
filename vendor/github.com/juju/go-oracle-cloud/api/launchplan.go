// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"net"
	"strings"

	"github.com/juju/go-oracle-cloud/response"
)

const (
	PublicNATPool string = "ippool:/oracle/public/ippool"
)

type VEthernet struct {
	Dns      []string `json:"dns,omitempty"`
	Nat      string   `json:"nat,omitempty"`
	SecLists []string `json:"seclists,omitempty"`
	Model    string   `json:"model,omitempty"`
}

type VNic struct {
	Dns       []string `json:"dns,omitempty"`
	Nat       string   `json:"nat,omitempty"`
	SecLists  []string `json:"seclists,omitempty"`
	Model     string   `json:"model,omitempty"`
	IPNetwork string   `json:"ipnetwork,omitempty"`
}

// InstancesParams params for the CreateInstance method
type Instances struct {
	// Shape represents every instance in oracle cloud has a predefined shape
	// in order to create a virtual instance
	// here it can be specify the computing power shape
	Shape string `json:"shape"`

	// Imagelist is the virtual image that will be used
	// in order to init the instance
	Imagelist string `json:"imagelist,omitempty"`

	// Name is the name of the instance
	Name string `json:"name,omitempty"`

	// Label is used when defining relationships in an orchestration.
	Label string `json:"label,omitempty"`

	// SSHKeys that will be installed on the Instance
	SSHKeys []string `json:"sshkeys,omitempty"`

	// Hostname is assigned to the instance
	// on an Oracle Linux instance, this host name
	// is displayed in response to the hostname command.
	// Only relative DNS is supported.
	// The domain name is suffixed to the host name that you specify.
	// The host name must not end with a period. If you don’t specify a host name,
	// then a name is generated automatically.
	// The DNS name of an instance depends on its host name, as follows:
	// If no DNS name is specified in the networking attribute, then
	// the DNS name is set to the host name, and a
	// reverse DNS record (PTR) is created for the host name.
	//
	// If the DNS name specified in the networking attribute matches the host
	// name, then that record also creates a reverse DNS record for the host name.
	//
	// If the dns attribute under networking is set to an empty list ([]),
	// then no DNS records are created even if a host name is specified.
	// The instance still receives its host name through DHCP, and can perform
	// a reverse lookup of its host name. However, no other
	// instance can perform this reverse lookup.
	// Note:
	// If an instance has network interfaces defined only for
	// IP networks and doesn’t have any interface on the shared network,
	// then when hostname is specified, no DNS entries are set.
	// In this case, DNS entries are set by the dns
	// subparameter of the networking attribute.
	Hostname string `json:"hostname,omitempty"`

	// Tags by assigning a human-friendly tag to an instance
	// you can identify the instance easily when you perform
	// an instance listing.
	// These tags aren’t available from within the instance.
	Tags []string `json:"tags,omitempty"`

	// Networking is the defined network for the instance.
	// ethn: The interface that you’re defining. Oracle-provided
	// images with release version 16.3.6 and later support eight vNICs.
	// You can also create private images that support multiple vNICs.
	// If the image you’ve specified supports eight vNICs,
	// then you can specify up to eight network interfaces, from eth0 to eth7.
	// Note:
	// For each interface, you can specify parameters for either the shared network,
	// or for an IP network. You can’t specify parameters for both networks for
	// the same ethn interface.
	// To add this instance to the shared network, you can specify any
	// or all of the following subparameters:
	// seclists: The security lists that you want to add the instance to.
	// nat: Indicates whether a temporary or permanent public IP address
	// should be assigned to the instance.
	// dns: DNS name for this instance. This name is relative to the internal DNS domain.
	// model: The type of network interface card (NIC). The only allowed value is e1000.
	// name_servers: (Optional) The name servers that are sent through DHCP as option 6.
	// You can specify a maximum of eight name server IP addresses per interface.
	// search_domains: (Optional) The search domains that should be sent through
	// DHCP as option 119.  You can enter a maximum of eight search domain zones per interface
	// Networking Attributes for Instances
	//
	// there are several subparameters that you can specify
	// under the ethn parameter in the networking section of instance attributes.
	// the list of subparameters varies depending on whether
	// you’re defining a network interface on a shared network or an IP network.
	Networking map[string]Networker `json:"networking,omitempty"`

	Attributes map[string]interface{} `json:"attributes,omitempty"`

	// Boot_order is the index number of the bootable storage
	// volume that should be used to boot the instance.
	// The only valid value is 1
	// If you set this attribute, you must also specify
	// a bootable storage volume with index number 1 in
	// the volume sub-parameter of storage_attachments
	// When you specify boot_order, you don’t need to specify
	// the imagelist attribute, because the instance is booted using the
	// image on the specified bootable storage volume.
	// If you specify both boot_order and imagelist, the imagelist attribute is ignored
	// Note:
	// You must not use this attribute when you’ve specified a high I/O shape.
	// This is because when a high I/O shape is used, the instance boots
	// from a nonpersistent NVM Express SSD disk and not
	// from a persistent block storage disk
	Boot_order []int `json:"boot_order,omitempty"`

	// Storage_attachments holds the stroge volume and index for creating the instance
	Storage_attachments []StorageAttachment `json:"storage_attachments,omitempty"`

	// Reverse_dns boolean flag that represents:
	// If set to true (default), then reverse DNS records are created.
	// If set to false, no reverse DNS records are created.
	Reverse_dns bool `json:"reverse_dns,omiempty"`
}

// StorageAttachment represents a storage to which the instance
// wants to attach
type StorageAttachment struct {
	// Volume is the three-part name (/Compute-identity_domain/user/object_name)
	// of the storage volume that you want to attach to the instance.
	// Note that volumes attached to an instance at launch time can't be detached.
	Volume string `json:"volume"`
	// Index is the index number for the volume.
	// The allowed range is 1 to 10. If you want to use a storage volume as the boot disk for an
	// instance, you must specify the index number for that volume as 1.
	// The index determines the device name by which the volume is exposed to the instance.
	// Index 0 is allocated to a nonpersistent boot disk, /dev/xvda.
	// An attachment with index 1 is exposed to the instance as /dev/xvdb, an attachment
	// with index 2 is exposed as /dev/xvdc, and so on.
	Index int `json:"index"`
}

// Networker type used for providing
// different implementations in the
// NewNetowrking creation method
type Networker interface {
	// Validate will validate the implementation
	Validate() (err error)
}

// Nat is the type that will be used to
// specify the ip reservation that you want
// to associate with this interface.
type Nat string

const (
	//NatReservatioNType IP reservation
	// that you want to associate with this interface
	NatReservationType string = "network/v1/ipreservation:"
	// To associate a temporary IP address with the
	// instance for use during the lifetime of
	// the instance, specify ippool:/oracle/public/ippool.
	NatPoolType string = "ippool:"
)

// NewNat returns a new nat complinat with the oracle iaas networking api
func NewNat(name string, nattype string) Nat {
	return Nat(nattype + name)
}

// Validate will check if the nat is valid or not
func (n Nat) Validate() (err error) {
	if n == "" {
		return errors.New(
			"go-oracle-cloud: Empty nat",
		)
	}

	prefix := "network/v1/ipreservation:"

	name := string(n)
	if !strings.HasPrefix(name, prefix) {
		return errors.New(
			"go-oracle-cloud: Invalid nat provided",
		)
	}

	return nil
}

// SharedNetwork type subparameters for a
// network instance shared network
type SharedNetwork struct {

	// seclits are the security lists that
	// you want to add the instance to.
	Seclists []string `json:"seclists,omitempty"`

	// Nat indicates whether a temporary or
	// permanent public IP address should
	// be assigned to the instance
	// To associate a temporary IP address with the
	// instance for use during the lifetime of the
	// instance, specify ippool:/oracle/public/ippool.
	// To associate a persistent IP address,
	// specify ipreservation:ipreservation_name,
	// where ipreservation_name is the three-part name
	// of an existing IP reservation in the
	// /Compute-identity_domain/user/object_name format.
	// if nat is not specified, then no public IP
	// address is associated with your instance
	// when it is created. If required,
	// you can associate an IP address with
	// the instance after the instance has been created
	Nat []Nat `json:"nat,omitempty"`

	// Dns name for this instance.
	// This name is relative to the internal DNS domain
	Dns []string `json:"dns,omitempty"`

	// Model is the type of network
	// interface card (NIC). The only allowed value is e1000
	Model string `json:"model,omitempty"`

	// Name_servers are the name servers that are sent
	// through DHCP as option 6.  You can specify a
	// maximum of eight name server IP addresses per interface
	Name_servers []string `json:"name_servers,omitempty"`

	// Search_domains are the search domains that should be
	// sent through DHCP as option 119.
	// You can enter a maximum of eight search
	// domain zones per interface
	Search_domains []string `json:"search_domains,omitempty"`
}

// Validate will check if the shared network that is created is valid
func (s SharedNetwork) Validate() (err error) {

	if len(s.Seclists) > 5 {
		return errors.New(
			"go-oracle-cloud: You can not provide more than 5 security lists",
		)
	}

	for _, val := range s.Nat {
		if err = val.Validate(); err != nil {
			return err
		}
	}

	if s.Model != "" && s.Model != "e1000" {
		return errors.New(
			"go-oracle-cloud: The only allowed value of the model is e1000",
		)
	}

	if len(s.Name_servers) > 8 {
		return errors.New(
			"go-oracle-cloud: You can specify a maximum of eight name server IP addresses per interface",
		)
	}

	if len(s.Search_domains) > 8 {
		return errors.New(
			"go-oracle-cloud:  You can enter a maximum of eight search domain zones per interface",
		)
	}

	return nil
}

// IpNetwork type subparameters for a Network Interface on an IP Network
type IPNetwork struct {

	// Ipnetwork is the name of the IP network
	// that you want to add the instance to
	Ipnetwork string `json:"ipnetwork,omitempty"`

	// IP If you want to associate a static private
	// IP address with the instance, specify an available
	// IP address from the IP address range
	// of the specified ipnetwork
	IP string `json:"ip,omitempty"`

	//  The MAC address of the interface, in hexadecimal format
	Address string `json:"address,omitempty"`

	// The three-part name of an IP
	// reservation that you want to
	// associate with this interface
	Nat []Nat `json:"nat,omitempty"`

	// Vnic is the name of the virtual nic you want to associate the
	// instance with
	Vnic string `json:"vnic,omitempty"`

	// Vnicsets a list of vNics that you want to add to  the instance
	Vnicsets []string `json:"vnicsets,omitempty"`

	// DNS a record names for the instance
	Dns []string `json:"dns,omitempty"`

	// Name_servers are the name servers that are sent
	// through DHCP as option 6.  You can specify a
	// maximum of eight name server IP addresses per interface
	Name_servers []string `json:"name_servers,omitempty"`

	// Search_domains are the search domains that should be
	// sent through DHCP as option 119.
	// You can enter a maximum of eight search
	// domain zones per interface
	Search_domains []string `json:"search_domains,omitempty"`
}

// Validate validates if the ipnetwork implementation is valid
func (i IPNetwork) Validate() (err error) {

	if i.Ipnetwork == "" {
		return errors.New(
			"go-oracle-cloud: Empty IPNetwork name",
		)
	}

	if i.Address != "" {
		if _, err = net.ParseMAC(i.Address); err != nil {
			return errors.New(
				"go-oracle-cloud: Invalid mac address supplied",
			)
		}
	}

	if i.IP != "" {
		if ip := net.ParseIP(i.IP); ip == nil {
			return errors.New(
				"go-oracle-cloud: Invalid ip address specified",
			)
		}
	}

	for _, val := range i.Nat {
		if err = val.Validate(); err != nil {
			return err
		}
	}

	if len(i.Name_servers) > 8 {
		return errors.New(
			"go-oracle-cloud: You can specify a maximum of eight name server IP addresses per interface",
		)
	}

	if len(i.Search_domains) > 8 {
		return errors.New(
			"go-oracle-cloud:  You can enter a maximum of eight search domain zones per interface",
		)
	}

	return nil
}

// InstanceParams used to feed the CreateInstance function
type InstanceParams struct {
	Relationships []string    `json:"relationships,omitempty"`
	Instances     []Instances `json:"instances"`
}

func (i InstanceParams) validate() (err error) {

	if i.Instances == nil || len(i.Instances) == 0 {
		return errors.New(
			"go-oracle-cloud: Empty instances in instance params",
		)
	}

	for _, val := range i.Instances {
		if val.Name == "" {
			return errors.New(
				"go-oracle-cloud: Empty instance name",
			)

		}
		if val.Imagelist == "" {
			return errors.New(
				"go-oracle-cloud: Empty instance image list",
			)
		}
		if val.Label == "" {
			return errors.New(
				"go-oracle-cloud: Empty instance label",
			)
		}

		for _, val := range val.Networking {
			if err = val.Validate(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Client) CreateInstance(params InstanceParams) (resp response.LaunchPlan, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err := params.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["launchplan"]

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
