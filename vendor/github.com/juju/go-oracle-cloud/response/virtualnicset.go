package response

// VnicSet is a A Virtual NIC Set, or vNICset, is a collection
// of one or more vNICs. You must specify a vNICset when you
// create a route. When a vNICset containing multiple vNICs is
// used in a route, Equal Cost Multipath (ECMP) anycast routing
// is implemented. Traffic routed by that route is load balanced
// across all the vNICs in the vNICset. Using vNICsets with
// multiple vNICs also ensures high availability for traffic
// across the specified vNICs
type VnicSet struct {

	// AppliedAcls is a list of ACLs applied to the VNICs in the set.
	AppliedAcls []string `json:"appliedAcls,omitempty"`

	// Description of the object
	Description *string `json:"description,omitempty"`

	// Name is the name of the vnic set
	Name string `json:"name"`

	// Tags associated with the object.
	Tags []string `json:"tags"`

	// List of VNICs associated with this VNIC set
	Vnics []string `json:"vnics"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllVnicSets type for holding all virtual nic sets
// in the oracle cloud account
type AllVnicSets struct {
	// Result is a slice of all vnic sets
	Result []VnicSet `json:"result,omitempty"`
}
