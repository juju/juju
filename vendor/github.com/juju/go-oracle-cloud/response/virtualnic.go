// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// VirtualNic A Virtual NIC, or vNIC, is a virtual network
// interface card that enables an instance to be associated
//  with a network. Instances created after mid-September 2016 using
// Oracle-provided images support eight vNICs, enabling each instance to
// be associated with up to eight networks. Virtual NIC is created when
// you launch an instance using an orchestration and in the network attributes
//  you specify the IP network or shared network that each vNIC of an instance
// should be associated with. For more information, see Instance Attributes
// in Using Oracle Compute Cloud Service (IaaS).
type VirtualNic struct {

	// Name is the name of the vnc
	Name string `json:"name"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	//Description is the description of the object.
	Description *string `json:"description,omitempty"`

	// Tags associated with the object.
	Tags []string `json:"tags"`

	// MacAddress holds the MAC address of this VNIC.
	MacAddress string `json:"macAddress"`

	//TransitFlag is True if the VNIC is of type "transit".
	TransitFlag bool `json:"transitFlag"`
}

// AllVirtualNics type returned by AllVirtualNic()
// holding all the vnc inside the oracle cloud
// account
type AllVirtualNics struct {
	Result []VirtualNic `json:"result"`
}
