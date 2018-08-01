// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// SecRule is a security rule defines network access over
// a specified protocol between instances in
// two security lists, or from a set of external
// hosts (an IP list) to instances in a security list
// Security rules tie the security list,
// security IP list, and security application
// entities together.
type SecRule struct {

	// Action is the security rule
	Action common.SecRuleAction `json:"action"`

	// Application is the application securiy name
	Application string `json:"application"`

	// Description is the description of the security rule
	Description string `json:"description,omitempty"`

	// Disabled flag indicates whether the security rule
	// is enabled (set to false) or disabled (true).
	// The default setting is false
	Disabled bool `json:"disabled"`

	// Dst_is_ip name of the destination
	// security list or security IP list
	// You cannot specify any of the security
	// IP lists in the /oracle/public container
	// as a destination in a secrule.
	Dst_is_ip string `json:"dst_is_ip"`

	//  Dst_list
	// Note: You can specify a security IP list
	// as the destination in a secrule,
	// provided src_list is a security list that
	// has DENY as its outbound policy.
	// You cannot specify any of the security
	// IP lists in the /oracle/public container
	// as a destination in a secrule.
	Dst_list string `json:"dst_list"`

	// Name is the
	Name string `json:"name"`

	// Is the id of the sec rule
	Id string `json:"id"`

	Src_is_ip string `json:"src_is_ip"`

	// Src_list name of the source security list or security IP list
	// You must use the prefix seclist:
	// or seciplist: to identify the list type
	Src_list string `json:"src_list"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllSecRules contains a slice of all security rules in
// the oracle cloud account
type AllSecRules struct {
	Result []SecRule `json:"result,omitempty"`
}
