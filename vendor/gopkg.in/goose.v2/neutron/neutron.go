// goose/neutron - Go package to interact with OpenStack Network Service (Neutron) API V2.0.
// See documentation at:
// http://developer.openstack.org/api-ref/networking/v2/index.html

package neutron

import (
	"fmt"
	"gopkg.in/goose.v2/client"
	"gopkg.in/goose.v2/errors"
	goosehttp "gopkg.in/goose.v2/http"
	"net/http"
	"net/url"
)

const (
	ApiFloatingIPsV2        = "floatingips"
	ApiNetworksV2           = "networks"
	ApiSubnetsV2            = "subnets"
	ApiSecurityGroupsV2     = "security-groups"
	ApiSecurityGroupRulesV2 = "security-group-rules"
)

// Filter keys for Networks.
// As of the Newton release of OpenStack, Network filter by subnet was not implemented
const (
	FilterRouterExternal = "router:external" // The router:external
	FilterNetwork        = "name"            // The network name.
	FilterProjectId      = "project_id"      // The project id
)

// NetworkV2 contains details about a labeled network
type NetworkV2 struct {
	Id                  string   `json:"id"` // UUID of the resource
	Name                string   // User-provided name for the network range
	SubnetIds           []string `json:"subnets"`         // an array of subnet UUIDs
	External            bool     `json:"router:external"` // is this network connected to an external router
	AvailabilityZones   []string `json:"availability_zones"`
	TenantId            string   `json:"tenant_id"`
	PortSecurityEnabled *bool    `json:"port_security_enabled"`
}

// SubnetV2 contains details about a labeled subnet
type SubnetV2 struct {
	Id              string        `json:"id"`         // UUID of the resource
	NetworkId       string        `json:"network_id"` // UUID of the related network
	Name            string        `json:"name"`       // User-provided name for the subnet
	Cidr            string        `json:"cidr"`       // IP range covered by the subnet
	AllocationPools []interface{} `json:"allocation_pools"`
	TenantId        string        `json:"tenant_id"`
}

// Client provides a means to access the OpenStack Network Service.
type Client struct {
	client client.Client
}

// New creates a new Client.
func New(client client.Client) *Client {
	return &Client{client}
}

// ----------------------------------------------------------------------------
// Filter builds filtering parameters to be used in an OpenStack query which supports
// filtering.  For example:
//
//     filter := NewFilter()
//     filter.Set(neutron.FilterRouterExternal, "true")
//     resp, err := neutron.ListNetworks(filter)
//
// TODO(hml): copied from the nova package.  However it should really be pulled out
// and shared between goose pkgs, but  we don't want to break compatibility or rev
// the package at this time.
//
type Filter struct {
	v url.Values
}

// NewFilter creates a new Filter.
func NewFilter() *Filter {
	return &Filter{make(url.Values)}
}

// Set sets a value in the filter.
func (f *Filter) Set(filter, value string) {
	f.v.Set(filter, value)
}

// ----------------------------------------------------------------------------

// ListNetworksV2 gives details on available networks, zero or one Filters
// accepted, any more will be ignored.
//
// TODO(hml): when this package revs to a new version, make this the same as other
// methods with Filters.  We don't want to break compatibility at this time or rev
// the package at this time.
func (c *Client) ListNetworksV2(filter ...*Filter) ([]NetworkV2, error) {
	var resp struct {
		Networks []NetworkV2 `json:"networks"`
	}
	var params *url.Values
	if len(filter) > 0 {
		params = &filter[0].v
	}
	requestData := goosehttp.RequestData{RespValue: &resp, Params: params}
	err := c.client.SendRequest(client.GET, "network", "v2.0", ApiNetworksV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of networks")
	}
	return resp.Networks, nil
}

// GetNetworkV2 gives details on a specific network
func (c *Client) GetNetworkV2(netID string) (*NetworkV2, error) {
	var resp struct {
		Network NetworkV2 `json:"network"`
	}
	url := fmt.Sprintf("%s/%s", ApiNetworksV2, netID)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get network detail")
	}
	return &resp.Network, nil
}

// ListSubnetsV2 gives details on available subnets
func (c *Client) ListSubnetsV2() ([]SubnetV2, error) {
	var resp struct {
		Subnets []SubnetV2 `json:"subnets"`
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", ApiSubnetsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of subnets")
	}
	return resp.Subnets, nil
}

// GetSubnetV2 gives details on a specific subnet
func (c *Client) GetSubnetV2(subnetID string) (*SubnetV2, error) {
	var resp struct {
		Subnet SubnetV2 `json:"subnet"`
	}
	url := fmt.Sprintf("%s/%s", ApiSubnetsV2, subnetID)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get subnet detail")
	}
	return &resp.Subnet, nil
}

// FloatingIPV2 contains details about a floating ip
type FloatingIPV2 struct {
	// FixedIP holds the private IP address of the machine (when assigned)
	FixedIP           string `json:"fixed_ip_address"`
	Id                string `json:"id"`
	IP                string `json:"floating_ip_address"`
	FloatingNetworkId string `json:"floating_network_id"`
}

// ListFloatingIPsV2 lists floating IP addresses associated with the tenant or account.
// Zero or one Filters accepted, any more will be ignored.
//
// TODO(hml): when this package revs to a new version, make this the same as other
// methods with Filters.  We don't want to break compatibility at this time or rev
// the package at this time.
func (c *Client) ListFloatingIPsV2(filter ...*Filter) ([]FloatingIPV2, error) {
	var resp struct {
		FloatingIPV2s []FloatingIPV2 `json:"floatingips"`
	}
	var params *url.Values
	if len(filter) > 0 {
		params = &filter[0].v
	}
	requestData := goosehttp.RequestData{RespValue: &resp, Params: params}
	err := c.client.SendRequest(client.GET, "network", "v2.0", ApiFloatingIPsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to list floating ips")
	}
	return resp.FloatingIPV2s, nil
}

// GetFloatingIPV2 lists details of the floating IP address associated with specified id.
func (c *Client) GetFloatingIPV2(ipId string) (*FloatingIPV2, error) {
	var resp struct {
		FloatingIPV2 FloatingIPV2 `json:"floatingip"`
	}

	url := fmt.Sprintf("%s/%s", ApiFloatingIPsV2, ipId)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get floating ip %s details", ipId)
	}
	return &resp.FloatingIPV2, nil
}

// AllocateFloatingIPV2 allocates a new floating IP address in the given external network.
func (c *Client) AllocateFloatingIPV2(floatingNetworkId string) (*FloatingIPV2, error) {
	var req struct {
		FloatingIPV2 struct {
			FloatingNetworkId string `json:"floating_network_id"`
		} `json:"floatingip"`
	}
	req.FloatingIPV2.FloatingNetworkId = floatingNetworkId
	var resp struct {
		FloatingIPV2 FloatingIPV2 `json:"floatingip"`
	}
	requestData := goosehttp.RequestData{
		ReqValue:       req,
		RespValue:      &resp,
		ExpectedStatus: []int{http.StatusCreated},
	}
	err := c.client.SendRequest(client.POST, "network", "v2.0", ApiFloatingIPsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to allocate a floating ip")
	}
	return &resp.FloatingIPV2, nil
}

// DeleteFloatingIPV2 deallocates the floating IP address associated with the specified id.
func (c *Client) DeleteFloatingIPV2(ipId string) error {
	url := fmt.Sprintf("%s/%s", ApiFloatingIPsV2, ipId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := c.client.SendRequest(client.DELETE, "network", "v2.0", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete floating ip %s details", ipId)
	}
	return err
}

// SecurityGroupRuleV2 describes a rule of a security group. There are 2
// basic rule types: ingress and egress rules (see RuleInfo struct).
type SecurityGroupRuleV2 struct {
	PortRangeMax   *int    `json:"port_range_max"` // Can be nil
	PortRangeMin   *int    `json:"port_range_min"` // Can be nil
	IPProtocol     *string `json:"protocol"`       // Can be nil, must be defined if PortRange is used
	ParentGroupId  string  `json:"security_group_id"`
	RemoteIPPrefix string  `json:"remote_ip_prefix"`
	RemoteGroupID  string  `json:"remote_group_id"`
	EthernetType   string  `json:"ethertype"`
	Direction      string  `json:"direction"` // Required
	Id             string  `json:",omitempty"`
	TenantId       string  `json:"tenant_id,omitempty"`
}

// SecurityGroupV2 describes a single security group in OpenStack.
type SecurityGroupV2 struct {
	Rules       []SecurityGroupRuleV2 `json:"security_group_rules"`
	TenantId    string                `json:"tenant_id"`
	Id          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
}

// ListSecurityGroupsV2 lists IDs, names, and other details for all security groups.
func (c *Client) ListSecurityGroupsV2() ([]SecurityGroupV2, error) {
	var resp struct {
		Groups []SecurityGroupV2 `json:"security_groups"`
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", ApiSecurityGroupsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to list security groups")
	}
	return resp.Groups, nil
}

// SecurityGroupByNameV2 returns the named security group.
// OpenStack now supports filtering with API calls.
// More than one Security Group may be returned, as names are not unique
// e.g. name=default
func (c *Client) SecurityGroupByNameV2(name string) ([]SecurityGroupV2, error) {
	var resp struct {
		Groups []SecurityGroupV2 `json:"security_groups"`
	}
	url := fmt.Sprintf("%s?name=%s", ApiSecurityGroupsV2, url.QueryEscape(name))
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, err
	}
	if len(resp.Groups) == 0 {
		return nil, errors.Newf(err, "failed to find security group with name: %s", name)
	}
	return resp.Groups, nil
}

// CreateSecurityGroupV2 creates a new security group.
func (c *Client) CreateSecurityGroupV2(name, description string) (*SecurityGroupV2, error) {
	var req struct {
		SecurityGroupV2 struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"security_group"`
	}
	req.SecurityGroupV2.Name = name
	req.SecurityGroupV2.Description = description

	var resp struct {
		SecurityGroup SecurityGroupV2 `json:"security_group"`
	}
	requestData := goosehttp.RequestData{
		ReqValue:       req,
		RespValue:      &resp,
		ExpectedStatus: []int{http.StatusCreated},
	}
	err := c.client.SendRequest(client.POST, "network", "v2.0", ApiSecurityGroupsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to create a security group with name: %s", name)
	}
	return &resp.SecurityGroup, nil
}

// DeleteSecurityGroupV2 deletes the specified security group.
func (c *Client) DeleteSecurityGroupV2(groupId string) error {
	url := fmt.Sprintf("%s/%s", ApiSecurityGroupsV2, groupId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := c.client.SendRequest(client.DELETE, "network", "v2.0", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete security group with id: %s", groupId)
	}
	return err
}

// UpdateSecurityGroupV2 updates the name and description of the given group.
func (c *Client) UpdateSecurityGroupV2(groupId, name, description string) (*SecurityGroupV2, error) {
	var req struct {
		SecurityGroupV2 struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"security_group"`
	}
	req.SecurityGroupV2.Name = name
	req.SecurityGroupV2.Description = description
	var resp struct {
		SecurityGroup SecurityGroupV2 `json:"security_group"`
	}
	url := fmt.Sprintf("%s/%s", ApiSecurityGroupsV2, groupId)
	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusOK}}
	err := c.client.SendRequest(client.PUT, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to update security group with Id %s to name: %s", groupId, name)
	}
	return &resp.SecurityGroup, nil
}

// RuleInfoV2 allows the callers of CreateSecurityGroupRuleV2() to
// create 2 types of security group rules: ingress rules and egress
// rules. Security Groups are applied on neutron ports.
//
// Each tenant/project has a default security group with a rule
// which allows intercommunication among hosts associated with the
// default security group.  As a result, all egress traffic and
// intercommunication in the default group are allowed and all ingress
// from outside of the default group is dropped by default (in the
// default security group).
//
// If no ingress rule is defined, all inbound traffic is dropped.
// If no egress rule is defined, all outbound traffic is dropped.
//
// For more information:
// http://docs.openstack.org/developer/neutron/devref/security_group_api.html
// https://wiki.openstack.org/wiki/Neutron/SecurityGroups
// Neutron source: https://github.com/openstack/neutron.git
type RuleInfoV2 struct {
	// Ingress or egress, which is the direction in which the metering
	// rule is applied. Required.
	Direction string `json:"direction"`

	// IPProtocol is optional, and if specified must be "tcp", "udp" or
	// "icmp" (in the case of icmp, both PortRangeMax and PortRangeMin should
	// be blank).
	IPProtocol string `json:"protocol,omitempty"`

	// The maximum port number in the range that is matched by the
	// security group rule. The port_range_min attribute constrains
	// the port_range_max attribute. If the protocol is ICMP, this
	// value must be an ICMP type.
	PortRangeMax int `json:"port_range_max,omitempty"`

	// The minimum port number in the range that is matched by the
	// security group rule. If the protocol is TCP or UDP, this value
	// must be less than or equal to the port_range_max attribute value.
	// If the protocol is ICMP, this value must be an ICMP type.
	PortRangeMin int `json:"port_range_min,omitempty"`

	EthernetType string `json:"ethertype,omitempty"`

	// Cidr for ICMP
	RemoteIPPrefix string `json:"remote_ip_prefix"`

	// ParentGroupId is always required and specifies the group to which
	// the rule is added.
	ParentGroupId string `json:"security_group_id"`
	RemoteGroupId string `json:"remote_group_id,omitempty"`
}

// CreateSecurityGroupRuleV2 creates a security group rule. It can either be an
// ingress rule or group rule (see the description of SecurityGroupRuleV2).
func (c *Client) CreateSecurityGroupRuleV2(ruleInfo RuleInfoV2) (*SecurityGroupRuleV2, error) {
	var req struct {
		SecurityGroupRule RuleInfoV2 `json:"security_group_rule"`
	}
	req.SecurityGroupRule = ruleInfo

	var resp struct {
		SecurityGroupRule SecurityGroupRuleV2 `json:"security_group_rule"`
	}

	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusCreated}}
	err := c.client.SendRequest(client.POST, "network", "v2.0", ApiSecurityGroupRulesV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to create a rule for the security group with id: %v", ruleInfo.ParentGroupId)
	}
	return &resp.SecurityGroupRule, nil
}

// DeleteSecurityGroupRuleV2 deletes the specified security group rule.
func (c *Client) DeleteSecurityGroupRuleV2(ruleId string) error {
	url := fmt.Sprintf("%s/%s", ApiSecurityGroupRulesV2, ruleId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := c.client.SendRequest(client.DELETE, "network", "v2.0", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete security group rule with id: %s", ruleId)
	}
	return err
}
