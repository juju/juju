// goose/nova - Go package to interact with OpenStack Compute (Nova) API.
// See http://docs.openstack.org/api/openstack-compute/2/content/.

package nova

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	"gopkg.in/goose.v1/client"
	"gopkg.in/goose.v1/errors"
	goosehttp "gopkg.in/goose.v1/http"
)

// API URL parts.
const (
	apiFlavors            = "flavors"
	apiFlavorsDetail      = "flavors/detail"
	apiServers            = "servers"
	apiServersDetail      = "servers/detail"
	apiSecurityGroups     = "os-security-groups"
	apiSecurityGroupRules = "os-security-group-rules"
	apiFloatingIPs        = "os-floating-ips"
	apiAvailabilityZone   = "os-availability-zone"
	apiVolumeAttachments  = "os-volume_attachments"
)

// Server status values.
const (
	StatusActive        = "ACTIVE"          // The server is active.
	StatusBuild         = "BUILD"           // The server has not finished the original build process.
	StatusBuildSpawning = "BUILD(spawning)" // The server has not finished the original build process but networking works (HP Cloud specific)
	StatusDeleted       = "DELETED"         // The server is deleted.
	StatusError         = "ERROR"           // The server is in error.
	StatusHardReboot    = "HARD_REBOOT"     // The server is hard rebooting.
	StatusPassword      = "PASSWORD"        // The password is being reset on the server.
	StatusReboot        = "REBOOT"          // The server is in a soft reboot state.
	StatusRebuild       = "REBUILD"         // The server is currently being rebuilt from an image.
	StatusRescue        = "RESCUE"          // The server is in rescue mode.
	StatusResize        = "RESIZE"          // Server is performing the differential copy of data that changed during its initial copy.
	StatusShutoff       = "SHUTOFF"         // The virtual machine (VM) was powered down by the user, but not through the OpenStack Compute API.
	StatusSuspended     = "SUSPENDED"       // The server is suspended, either by request or necessity.
	StatusUnknown       = "UNKNOWN"         // The state of the server is unknown. Contact your cloud provider.
	StatusVerifyResize  = "VERIFY_RESIZE"   // System is awaiting confirmation that the server is operational after a move or resize.
)

// Filter keys.
const (
	FilterStatus       = "status"        // The server status. See Server Status Values.
	FilterImage        = "image"         // The image reference specified as an ID or full URL.
	FilterFlavor       = "flavor"        // The flavor reference specified as an ID or full URL.
	FilterServer       = "name"          // The server name.
	FilterMarker       = "marker"        // The ID of the last item in the previous list.
	FilterLimit        = "limit"         // The page size.
	FilterChangesSince = "changes-since" // The changes-since time. The list contains servers that have been deleted since the changes-since time.
)

// Client provides a means to access the OpenStack Compute Service.
type Client struct {
	client client.Client
}

// New creates a new Client.
func New(client client.Client) *Client {
	return &Client{client}
}

// ----------------------------------------------------------------------------
// Filtering helper.
//
// Filter builds filtering parameters to be used in an OpenStack query which supports
// filtering.  For example:
//
//     filter := NewFilter()
//     filter.Set(nova.FilterServer, "server_name")
//     filter.Set(nova.FilterStatus, nova.StatusBuild)
//     resp, err := nova.ListServers(filter)
//
type Filter struct {
	v url.Values
}

// NewFilter creates a new Filter.
func NewFilter() *Filter {
	return &Filter{make(url.Values)}
}

func (f *Filter) Set(filter, value string) {
	f.v.Set(filter, value)
}

// Link describes a link to a flavor or server.
type Link struct {
	Href string
	Rel  string
	Type string
}

// Entity describe a basic information about a flavor or server.
type Entity struct {
	Id    string `json:"-"`
	UUID  string `json:"uuid"`
	Links []Link `json:"links"`
	Name  string `json:"name"`
}

func stringValue(item interface{}, attr string) string {
	return reflect.ValueOf(item).FieldByName(attr).String()
}

// Allow Entity slices to be sorted by named attribute.
type EntitySortBy struct {
	Attr     string
	Entities []Entity
}

func (e EntitySortBy) Len() int {
	return len(e.Entities)
}

func (e EntitySortBy) Less(i, j int) bool {
	return stringValue(e.Entities[i], e.Attr) < stringValue(e.Entities[j], e.Attr)
}

func (e EntitySortBy) Swap(i, j int) {
	e.Entities[i], e.Entities[j] = e.Entities[j], e.Entities[i]
}

// ListFlavours lists IDs, names, and links for available flavors.
func (c *Client) ListFlavors() ([]Entity, error) {
	var resp struct {
		Flavors []Entity
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiFlavors, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of flavours")
	}
	return resp.Flavors, nil
}

// FlavorDetail describes detailed information about a flavor.
type FlavorDetail struct {
	Name  string
	RAM   int    // Available RAM, in MB
	VCPUs int    // Number of virtual CPU (cores)
	Disk  int    // Available root partition space, in GB
	Id    string `json:"-"`
	Links []Link
}

// Allow FlavorDetail slices to be sorted by named attribute.
type FlavorDetailSortBy struct {
	Attr          string
	FlavorDetails []FlavorDetail
}

func (e FlavorDetailSortBy) Len() int {
	return len(e.FlavorDetails)
}

func (e FlavorDetailSortBy) Less(i, j int) bool {
	return stringValue(e.FlavorDetails[i], e.Attr) < stringValue(e.FlavorDetails[j], e.Attr)
}

func (e FlavorDetailSortBy) Swap(i, j int) {
	e.FlavorDetails[i], e.FlavorDetails[j] = e.FlavorDetails[j], e.FlavorDetails[i]
}

// ListFlavorsDetail lists all details for available flavors.
func (c *Client) ListFlavorsDetail() ([]FlavorDetail, error) {
	var resp struct {
		Flavors []FlavorDetail
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiFlavorsDetail, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of flavour details")
	}
	return resp.Flavors, nil
}

// ListServers lists IDs, names, and links for all servers.
func (c *Client) ListServers(filter *Filter) ([]Entity, error) {
	var resp struct {
		Servers []Entity
	}
	var params *url.Values
	if filter != nil {
		params = &filter.v
	}
	requestData := goosehttp.RequestData{RespValue: &resp, Params: params, ExpectedStatus: []int{http.StatusOK}}
	err := c.client.SendRequest(client.GET, "compute", apiServers, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of servers")
	}
	return resp.Servers, nil
}

// IPAddress describes a single IPv4/6 address of a server.
type IPAddress struct {
	Version int    `json:"version"`
	Address string `json:"addr"`
}

// ServerDetail describes a server in more detail.
// See: http://docs.openstack.org/api/openstack-compute/2/content/Extensions-d1e1444.html#ServersCBSJ
type ServerDetail struct {
	// AddressIPv4 and AddressIPv6 hold the first public IPv4 or IPv6
	// address of the server, or "" if no floating IP is assigned.
	AddressIPv4 string
	AddressIPv6 string

	// Addresses holds the list of all IP addresses assigned to this
	// server, grouped by "network" name ("public", "private" or a
	// custom name).
	Addresses map[string][]IPAddress

	// Created holds the creation timestamp of the server
	// in RFC3339 format.
	Created string

	Flavor   Entity
	HostId   string
	Id       string `json:"-"`
	UUID     string
	Image    Entity
	Links    []Link
	Name     string
	Metadata map[string]string

	// HP Cloud returns security groups in server details.
	Groups []Entity `json:"security_groups"`

	// Progress holds the completion percentage of
	// the current operation
	Progress int

	// Status holds the current status of the server,
	// one of the Status* constants.
	Status string

	TenantId string `json:"tenant_id"`

	// Updated holds the timestamp of the last update
	// to the server in RFC3339 format.
	Updated string

	UserId string `json:"user_id"`

	AvailabilityZone string `json:"OS-EXT-AZ:availability_zone"`
}

// ListServersDetail lists all details for available servers.
func (c *Client) ListServersDetail(filter *Filter) ([]ServerDetail, error) {
	var resp struct {
		Servers []ServerDetail
	}
	var params *url.Values
	if filter != nil {
		params = &filter.v
	}
	requestData := goosehttp.RequestData{RespValue: &resp, Params: params}
	err := c.client.SendRequest(client.GET, "compute", apiServersDetail, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of server details")
	}
	return resp.Servers, nil
}

// GetServer lists details for the specified server.
func (c *Client) GetServer(serverId string) (*ServerDetail, error) {
	var resp struct {
		Server ServerDetail
	}
	url := fmt.Sprintf("%s/%s", apiServers, serverId)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get details for serverId: %s", serverId)
	}
	return &resp.Server, nil
}

// DeleteServer terminates the specified server.
func (c *Client) DeleteServer(serverId string) error {
	var resp struct {
		Server ServerDetail
	}
	url := fmt.Sprintf("%s/%s", apiServers, serverId)
	requestData := goosehttp.RequestData{RespValue: &resp, ExpectedStatus: []int{http.StatusNoContent}}
	err := c.client.SendRequest(client.DELETE, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete server with serverId: %s", serverId)
	}
	return err
}

type SecurityGroupName struct {
	Name string `json:"name"`
}

// ServerNetworks sets what networks a server should be connected to on boot.
// - FixedIp may be supplied only when NetworkId is also given.
// - PortId may be supplied only if neither NetworkId or FixedIp is set.
type ServerNetworks struct {
	NetworkId string `json:"uuid,omitempty"`
	FixedIp   string `json:"fixed_ip,omitempty"`
	PortId    string `json:"port,omitempty"`
}

// RunServerOpts defines required and optional arguments for RunServer().
type RunServerOpts struct {
	Name               string              `json:"name"`                        // Required
	FlavorId           string              `json:"flavorRef"`                   // Required
	ImageId            string              `json:"imageRef"`                    // Required
	UserData           []byte              `json:"user_data"`                   // Optional
	SecurityGroupNames []SecurityGroupName `json:"security_groups"`             // Optional
	Networks           []ServerNetworks    `json:"networks"`                    // Optional
	AvailabilityZone   string              `json:"availability_zone,omitempty"` // Optional
	Metadata           map[string]string   `json:"metadata,omitempty"`          // Optional
	ConfigDrive        bool                `json:"config_drive,omitempty"`      // Optional
}

// RunServer creates a new server, based on the given RunServerOpts.
func (c *Client) RunServer(opts RunServerOpts) (*Entity, error) {
	var req struct {
		Server RunServerOpts `json:"server"`
	}
	req.Server = opts
	// opts.UserData gets serialized to base64-encoded string automatically
	var resp struct {
		Server Entity `json:"server"`
	}
	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.POST, "compute", apiServers, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to run a server with %#v", opts)
	}
	return &resp.Server, nil
}

type serverUpdateNameOpts struct {
	Name string `json:"name"`
}

// UpdateServerName updates the name of the given server.
func (c *Client) UpdateServerName(serverID, name string) (*Entity, error) {
	var req struct {
		Server serverUpdateNameOpts `json:"server"`
	}
	var resp struct {
		Server Entity `json:"server"`
	}
	req.Server = serverUpdateNameOpts{Name: name}
	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusOK}}
	url := fmt.Sprintf("%s/%s", apiServers, serverID)
	err := c.client.SendRequest(client.PUT, "compute", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to update server name to %q", name)
	}
	return &resp.Server, nil
}

// SecurityGroupRef refers to an existing named security group
type SecurityGroupRef struct {
	TenantId string `json:"tenant_id"`
	Name     string `json:"name"`
}

// SecurityGroupRule describes a rule of a security group. There are 2
// basic rule types: ingress and group rules (see RuleInfo struct).
type SecurityGroupRule struct {
	FromPort      *int              `json:"from_port"`   // Can be nil
	IPProtocol    *string           `json:"ip_protocol"` // Can be nil
	ToPort        *int              `json:"to_port"`     // Can be nil
	ParentGroupId string            `json:"-"`
	IPRange       map[string]string `json:"ip_range"` // Can be empty
	Id            string            `json:"-"`
	Group         SecurityGroupRef
}

// SecurityGroup describes a single security group in OpenStack.
type SecurityGroup struct {
	Rules       []SecurityGroupRule
	TenantId    string `json:"tenant_id"`
	Id          string `json:"-"`
	Name        string
	Description string
}

// ListSecurityGroups lists IDs, names, and other details for all security groups.
func (c *Client) ListSecurityGroups() ([]SecurityGroup, error) {
	var resp struct {
		Groups []SecurityGroup `json:"security_groups"`
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiSecurityGroups, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to list security groups")
	}
	return resp.Groups, nil
}

// GetSecurityGroupByName returns the named security group.
// Note: due to lack of filtering support when querying security groups, this is not an efficient implementation
// but it's all we can do for now.
func (c *Client) SecurityGroupByName(name string) (*SecurityGroup, error) {
	// OpenStack does not support group filtering, so we need to load them all and manually search by name.
	groups, err := c.ListSecurityGroups()
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.Name == name {
			return &group, nil
		}
	}
	return nil, errors.NewNotFoundf(nil, "", "Security group %s not found.", name)
}

// GetServerSecurityGroups list security groups for a specific server.
func (c *Client) GetServerSecurityGroups(serverId string) ([]SecurityGroup, error) {

	var resp struct {
		Groups []SecurityGroup `json:"security_groups"`
	}
	url := fmt.Sprintf("%s/%s/%s", apiServers, serverId, apiSecurityGroups)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", url, &requestData)
	if err != nil {
		// Sadly HP Cloud lacks the necessary API and also doesn't provide full SecurityGroup lookup.
		// The best we can do for now is to use just the Id and Name from the group entities.
		if errors.IsNotFound(err) {
			serverDetails, err := c.GetServer(serverId)
			if err == nil {
				result := make([]SecurityGroup, len(serverDetails.Groups))
				for i, e := range serverDetails.Groups {
					result[i] = SecurityGroup{
						Id:   e.Id,
						Name: e.Name,
					}
				}
				return result, nil
			}
		}
		return nil, errors.Newf(err, "failed to list server (%s) security groups", serverId)
	}
	return resp.Groups, nil
}

// CreateSecurityGroup creates a new security group.
func (c *Client) CreateSecurityGroup(name, description string) (*SecurityGroup, error) {
	var req struct {
		SecurityGroup struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"security_group"`
	}
	req.SecurityGroup.Name = name
	req.SecurityGroup.Description = description

	var resp struct {
		SecurityGroup SecurityGroup `json:"security_group"`
	}
	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusOK}}
	err := c.client.SendRequest(client.POST, "compute", apiSecurityGroups, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to create a security group with name: %s", name)
	}
	return &resp.SecurityGroup, nil
}

// DeleteSecurityGroup deletes the specified security group.
func (c *Client) DeleteSecurityGroup(groupId string) error {
	url := fmt.Sprintf("%s/%s", apiSecurityGroups, groupId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.DELETE, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete security group with id: %s", groupId)
	}
	return err
}

// UpdateSecurityGroup updates the name and description of the given group.
func (c *Client) UpdateSecurityGroup(groupId, name, description string) (*SecurityGroup, error) {
	var req struct {
		SecurityGroup struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"security_group"`
	}
	req.SecurityGroup.Name = name
	req.SecurityGroup.Description = description
	var resp struct {
		SecurityGroup SecurityGroup `json:"security_group"`
	}
	url := fmt.Sprintf("%s/%s", apiSecurityGroups, groupId)
	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp, ExpectedStatus: []int{http.StatusOK}}
	err := c.client.SendRequest(client.PUT, "compute", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to update security group with Id %s to name: %s", groupId, name)
	}
	return &resp.SecurityGroup, nil
}

// RuleInfo allows the callers of CreateSecurityGroupRule() to
// create 2 types of security group rules: ingress rules and group
// rules. The difference stems from how the "source" is defined.
// It can be either:
// 1. Ingress rules - specified directly with any valid subnet mask
//    in CIDR format (e.g. "192.168.0.0/16");
// 2. Group rules - specified indirectly by giving a source group,
// which can be any user's group (different tenant ID).
//
// Every rule works as an iptables ACCEPT rule, thus a group/ with no
// rules does not allow ingress at all. Rules can be added and removed
// while the server(s) are running. The set of security groups that
// apply to a server is changed only when the server is
// started. Adding or removing a security group on a running server
// will not take effect until that server is restarted. However,
// changing rules of existing groups will take effect immediately.
//
// For more information:
// http://docs.openstack.org/developer/nova/nova.concepts.html#concept-security-groups
// Nova source: https://github.com/openstack/nova.git
type RuleInfo struct {
	/// IPProtocol is optional, and if specified must be "tcp", "udp" or
	//  "icmp" (in this case, both FromPort and ToPort can be -1).
	IPProtocol string `json:"ip_protocol"`

	// FromPort and ToPort are both optional, and if specifed must be
	// integers between 1 and 65535 (valid TCP port numbers). -1 is a
	// special value, meaning "use default" (e.g. for ICMP).
	FromPort int `json:"from_port"`
	ToPort   int `json:"to_port"`

	// Cidr cannot be specified with GroupId. Ingress rules need a valid
	// subnet mast in CIDR format here, while if GroupID is specifed, it
	// means you're adding a group rule, specifying source group ID, which
	// must exist already and can be equal to ParentGroupId).
	// need Cidr, while
	Cidr    string  `json:"cidr"`
	GroupId *string `json:"-"`

	// ParentGroupId is always required and specifies the group to which
	// the rule is added.
	ParentGroupId string `json:"-"`
}

// CreateSecurityGroupRule creates a security group rule.
// It can either be an ingress rule or group rule (see the
// description of RuleInfo).
func (c *Client) CreateSecurityGroupRule(ruleInfo RuleInfo) (*SecurityGroupRule, error) {
	var req struct {
		SecurityGroupRule RuleInfo `json:"security_group_rule"`
	}
	req.SecurityGroupRule = ruleInfo

	var resp struct {
		SecurityGroupRule SecurityGroupRule `json:"security_group_rule"`
	}

	requestData := goosehttp.RequestData{ReqValue: req, RespValue: &resp}
	err := c.client.SendRequest(client.POST, "compute", apiSecurityGroupRules, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to create a rule for the security group with id: %v", ruleInfo.GroupId)
	}
	return &resp.SecurityGroupRule, nil
}

// DeleteSecurityGroupRule deletes the specified security group rule.
func (c *Client) DeleteSecurityGroupRule(ruleId string) error {
	url := fmt.Sprintf("%s/%s", apiSecurityGroupRules, ruleId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.DELETE, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete security group rule with id: %s", ruleId)
	}
	return err
}

// AddServerSecurityGroup adds a security group to the specified server.
func (c *Client) AddServerSecurityGroup(serverId, groupName string) error {
	var req struct {
		AddSecurityGroup struct {
			Name string `json:"name"`
		} `json:"addSecurityGroup"`
	}
	req.AddSecurityGroup.Name = groupName

	url := fmt.Sprintf("%s/%s/action", apiServers, serverId)
	requestData := goosehttp.RequestData{ReqValue: req, ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to add security group '%s' to server with id: %s", groupName, serverId)
	}
	return err
}

// RemoveServerSecurityGroup removes a security group from the specified server.
func (c *Client) RemoveServerSecurityGroup(serverId, groupName string) error {
	var req struct {
		RemoveSecurityGroup struct {
			Name string `json:"name"`
		} `json:"removeSecurityGroup"`
	}
	req.RemoveSecurityGroup.Name = groupName

	url := fmt.Sprintf("%s/%s/action", apiServers, serverId)
	requestData := goosehttp.RequestData{ReqValue: req, ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to remove security group '%s' from server with id: %s", groupName, serverId)
	}
	return err
}

// FloatingIP describes a floating (public) IP address, which can be
// assigned to a server, thus allowing connections from outside.
type FloatingIP struct {
	// FixedIP holds the private IP address of the machine (when assigned)
	FixedIP *string `json:"fixed_ip"`
	Id      string  `json:"-"`
	// InstanceId holds the instance id of the machine, if this FIP is assigned to one
	InstanceId *string `json:"-"`
	IP         string  `json:"ip"`
	Pool       string  `json:"pool"`
}

// ListFloatingIPs lists floating IP addresses associated with the tenant or account.
func (c *Client) ListFloatingIPs() ([]FloatingIP, error) {
	var resp struct {
		FloatingIPs []FloatingIP `json:"floating_ips"`
	}

	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiFloatingIPs, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to list floating ips")
	}
	return resp.FloatingIPs, nil
}

// GetFloatingIP lists details of the floating IP address associated with specified id.
func (c *Client) GetFloatingIP(ipId string) (*FloatingIP, error) {
	var resp struct {
		FloatingIP FloatingIP `json:"floating_ip"`
	}

	url := fmt.Sprintf("%s/%s", apiFloatingIPs, ipId)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get floating ip %s details", ipId)
	}
	return &resp.FloatingIP, nil
}

// AllocateFloatingIP allocates a new floating IP address to a tenant or account.
func (c *Client) AllocateFloatingIP() (*FloatingIP, error) {
	var resp struct {
		FloatingIP FloatingIP `json:"floating_ip"`
	}

	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.POST, "compute", apiFloatingIPs, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to allocate a floating ip")
	}
	return &resp.FloatingIP, nil
}

// DeleteFloatingIP deallocates the floating IP address associated with the specified id.
func (c *Client) DeleteFloatingIP(ipId string) error {
	url := fmt.Sprintf("%s/%s", apiFloatingIPs, ipId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.DELETE, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete floating ip %s details", ipId)
	}
	return err
}

// AddServerFloatingIP assigns a floating IP address to the specified server.
func (c *Client) AddServerFloatingIP(serverId, address string) error {
	var req struct {
		AddFloatingIP struct {
			Address string `json:"address"`
		} `json:"addFloatingIp"`
	}
	req.AddFloatingIP.Address = address

	url := fmt.Sprintf("%s/%s/action", apiServers, serverId)
	requestData := goosehttp.RequestData{ReqValue: req, ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to add floating ip %s to server with id: %s", address, serverId)
	}
	return err
}

// RemoveServerFloatingIP removes a floating IP address from the specified server.
func (c *Client) RemoveServerFloatingIP(serverId, address string) error {
	var req struct {
		RemoveFloatingIP struct {
			Address string `json:"address"`
		} `json:"removeFloatingIp"`
	}
	req.RemoveFloatingIP.Address = address

	url := fmt.Sprintf("%s/%s/action", apiServers, serverId)
	requestData := goosehttp.RequestData{ReqValue: req, ExpectedStatus: []int{http.StatusAccepted}}
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to remove floating ip %s from server with id: %s", address, serverId)
	}
	return err
}

// AvailabilityZone identifies an availability zone, and describes its state.
type AvailabilityZone struct {
	Name  string                `json:"zoneName"`
	State AvailabilityZoneState `json:"zoneState"`
}

// AvailabilityZoneState describes an availability zone's state.
type AvailabilityZoneState struct {
	Available bool
}

// ListAvailabilityZones lists all availability zones.
//
// Availability zones are an OpenStack extension; if the server does not
// support them, then an error satisfying errors.IsNotImplemented will be
// returned.
func (c *Client) ListAvailabilityZones() ([]AvailabilityZone, error) {
	var resp struct {
		AvailabilityZoneInfo []AvailabilityZone
	}
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "compute", apiAvailabilityZone, &requestData)
	if errors.IsNotFound(err) {
		// Availability zones are an extension, so don't
		// return an error if the API does not exist.
		return nil, errors.NewNotImplementedf(
			err, nil, "the server does not support availability zones",
		)
	}
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of availability zones")
	}
	return resp.AvailabilityZoneInfo, nil
}

// VolumeAttachment represents both the request and response for
// attaching volumes.
type VolumeAttachment struct {
	Device   string `json:"device"`
	Id       string `json:"id"`
	ServerId string `json:"serverId"`
	VolumeId string `json:"volumeId"`
}

// AttachVolume attaches the given volumeId to the given serverId at
// mount point specified in device. Note that the server must support
// the os-volume_attachments attachment; if it does not, an error will
// be returned stating such.
func (c *Client) AttachVolume(serverId, volumeId, device string) (*VolumeAttachment, error) {

	type volumeAttachment struct {
		VolumeAttachment VolumeAttachment `json:"volumeAttachment"`
	}

	var resp volumeAttachment
	requestData := goosehttp.RequestData{
		ReqValue: &volumeAttachment{VolumeAttachment{
			ServerId: serverId,
			VolumeId: volumeId,
			Device:   device,
		}},
		RespValue: &resp,
	}
	url := fmt.Sprintf("%s/%s/%s", apiServers, serverId, apiVolumeAttachments)
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if errors.IsNotFound(err) {
		return nil, errors.NewNotImplementedf(
			err, nil, "the server does not support attaching volumes",
		)
	}
	if err != nil {
		return nil, errors.Newf(err, "failed to attach volume")
	}
	return &resp.VolumeAttachment, nil
}

// DetachVolume detaches the volume with the given attachmentId from
// the server with the given serverId.
func (c *Client) DetachVolume(serverId, attachmentId string) error {
	requestData := goosehttp.RequestData{
		ExpectedStatus: []int{http.StatusAccepted},
	}
	url := fmt.Sprintf("%s/%s/%s/%s", apiServers, serverId, apiVolumeAttachments, attachmentId)
	err := c.client.SendRequest(client.DELETE, "compute", url, &requestData)
	if errors.IsNotFound(err) {
		return errors.NewNotImplementedf(
			err, nil, "the server does not support deleting attached volumes",
		)
	}
	if err != nil {
		return errors.Newf(err, "failed to delete volume attachment")
	}
	return nil
}

// ListVolumeAttachments lists the volumes currently attached to the
// server with the given serverId.
func (c *Client) ListVolumeAttachments(serverId string) ([]VolumeAttachment, error) {

	var resp struct {
		VolumeAttachments []VolumeAttachment `json:"volumeAttachments"`
	}
	requestData := goosehttp.RequestData{
		RespValue: &resp,
	}
	url := fmt.Sprintf("%s/%s/%s", apiServers, serverId, apiVolumeAttachments)
	err := c.client.SendRequest(client.GET, "compute", url, &requestData)
	if errors.IsNotFound(err) {
		return nil, errors.NewNotImplementedf(
			err, nil, "the server does not support listing attached volumes",
		)
	}
	if err != nil {
		return nil, errors.Newf(err, "failed to list volume attachments")
	}
	return resp.VolumeAttachments, nil
}

// SetServerMetadata sets metadata on a server.
func (c *Client) SetServerMetadata(serverId string, metadata map[string]string) error {
	req := struct {
		Metadata map[string]string `json:"metadata"`
	}{metadata}

	url := fmt.Sprintf("%s/%s/metadata", apiServers, serverId)
	requestData := goosehttp.RequestData{
		ReqValue: req, ExpectedStatus: []int{http.StatusOK},
	}
	err := c.client.SendRequest(client.POST, "compute", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to set metadata %v on server with id: %s", metadata, serverId)
	}
	return err
}
