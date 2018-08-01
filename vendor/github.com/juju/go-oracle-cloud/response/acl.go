// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// Acl an access control list (ACL) is a collection
// of security rules that can be applied to a vNICset.
// You can use ACLs to control the traffic between instances.
// ACLs determine whether a packet can be forwarded
// to or from a vNIC, based on the criteria specified
// in its security rules. When you create a security rule,
// you specify the ACL that it belongs to. ACLs apply to vNICsets.
// Each vNICset can reference multiple ACLs and each ACL
// can be referenced in multiple vNICsets. When an ACL is
// referenced in a vNICset, every security rule that belongs
// to the ACL applies to every vNIC that is specified
// in the vNICset.
// The default ACL allows traffic to all virtual NICs in an IP network.
type Acl struct {
	// Name is the name of the ACL
	Name string `json:"name"`

	// Description of the ACL.
	Description string `json:"description,omitempty"`
	// EnableFlag enables or disables the ACL.
	// Set to true by default.
	// Set to false when the ACL is disable.
	EnableFlag bool `json:"enableFlag"`

	// Tags strings that you can use to tag the ACL.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Unified Resource Identifier
	Uri string `json:"uri"`
}

// AllAcls holds all acls inside of the account
type AllAcls struct {
	Result []Acl `json:"result,omitempty"`
}
