package testservices

import "fmt"

// This map is copied from nova python client
// https://github.com/openstack/nova/blob/master/nova/api/openstack/wsgi.py#L1185
var nameReference = map[int]string{
	400: "badRequest",
	401: "unauthorized",
	403: "forbidden",
	404: "itemNotFound",
	405: "badMethod",
	409: "conflictingRequest",
	413: "overLimit",
	415: "badMediaType",
	429: "overLimit",
	501: "notImplemented",
	503: "serviceUnavailable",
}

type ServerError struct {
	message string
	code    int
}

func serverErrorf(code int, message string, args ...interface{}) *ServerError {
	return &ServerError{code: code, message: fmt.Sprintf(message, args...)}
}

func (n *ServerError) Code() int {
	return n.code
}

func (n *ServerError) AsJSON() string {
	return fmt.Sprintf(`{%q:{"message":%q, "code":%d}}`, n.Name(), n.message, n.code)
}

func (n *ServerError) Error() string {
	return fmt.Sprintf("%s: %s", n.Name(), n.message)
}

func (n *ServerError) Name() string {
	name, ok := nameReference[n.code]
	if !ok {
		return "computeFault"
	}
	return name
}

func NewInternalServerError(message string) *ServerError {
	return serverErrorf(500, message)
}

func NewNotFoundError(message string) *ServerError {
	return serverErrorf(404, message)
}

func NewNoMoreFloatingIpsError() *ServerError {
	return serverErrorf(404, "Zero floating ips available")
}

func NewIPLimitExceededError() *ServerError {
	return serverErrorf(413, "Maximum number of floating ips exceeded")
}

func NewRateLimitExceededError() *ServerError {
	// This is an undocumented error
	return serverErrorf(413, "Retry limit exceeded")
}

func NewTooManyRequestsError() *ServerError {
	return serverErrorf(429, "Too many requests")
}

func NewForbiddenRateLimitError() *ServerError {
	return serverErrorf(403, "User Rate Limit Exceeded.")
}

func NewServiceUnavailRateLimitError() *ServerError {
	return serverErrorf(503, "The maximum request receiving rate is exceeded.")
}

func NewAvailabilityZoneIsNotAvailableError() *ServerError {
	return serverErrorf(400, "The requested availability zone is not available")
}

func NewAddFlavorError(id string) *ServerError {
	return serverErrorf(409, "A flavor with id %q already exists", id)
}

func NewNoSuchFlavorError(id string) *ServerError {
	return serverErrorf(404, "No such flavor %q", id)
}

func NewServerByIDNotFoundError(id string) *ServerError {
	return serverErrorf(404, "No such server %q", id)
}

func NewServerByNameNotFoundError(name string) *ServerError {
	return serverErrorf(404, "No such server named %q", name)
}

func NewServerAlreadyExistsError(id string) *ServerError {
	return serverErrorf(409, "A server with id %q already exists", id)
}

func NewSecurityGroupAlreadyExistsError(id string) *ServerError {
	return serverErrorf(409, "A security group with id %s already exists", id)
}

func NewSecurityGroupByIDNotFoundError(groupId string) *ServerError {
	return serverErrorf(404, "No such security group %s", groupId)
}

func NewSecurityGroupByNameNotFoundError(name string) *ServerError {
	return serverErrorf(404, "No such security group named %q", name)
}

func NewSecurityGroupRuleAlreadyExistsError(id string) *ServerError {
	return serverErrorf(409, "A security group rule with id %s already exists", id)
}

func NewNeutronSecurityGroupRuleAlreadyExistsError(parentId string) *ServerError {
	return serverErrorf(409, "Security group rule already exists. Group id is %s.", parentId)
}

func NewCannotAddTwiceRuleToGroupError(ruleId, groupId string) *ServerError {
	return serverErrorf(409, "Cannot add twice rule %s to security group %s", ruleId, groupId)
}

func NewUnknownSecurityGroupError(groupId string) *ServerError {
	return serverErrorf(409, "Unknown source security group %s", groupId)
}

func NewSecurityGroupRuleNotFoundError(ruleId string) *ServerError {
	return serverErrorf(404, "No such security group rule %s", ruleId)
}

func NewInvalidDirectionSecurityGroupError(direction string) *ServerError {
	return serverErrorf(400, "Invalid input for direction. Reason: %s is not ingress or egress.", direction)
}

func NewSecurityGroupRuleInvalidEthernetType(ethernetType string) *ServerError {
	return serverErrorf(400, "Invalid input for ethertype. Reason: %s is not '', 'IPv4' or 'IPv6'.", ethernetType)
}

func NewSecurityGroupRuleParameterConflict(param1 string, value1 string, param2 string, value2 string) *ServerError {
	return serverErrorf(400, "Conflicting value %s %s for %s %s", param1, value1, param2, value2)
}

func NewSecurityGroupRuleInvalidCIDR(cidr string) *ServerError {
	return serverErrorf(400, "Invalid CIDR %s given as IP prefix.", cidr)
}

func NewServerBelongsToGroupError(serverId, groupId string) *ServerError {
	return serverErrorf(409, "Server %q already belongs to group %s", serverId, groupId)
}

func NewServerDoesNotBelongToGroupsError(serverId string) *ServerError {
	return serverErrorf(400, "Server %q does not belong to any groups", serverId)
}

func NewServerDoesNotBelongToGroupError(serverId, groupId string) *ServerError {
	return serverErrorf(400, "Server %q does not belong to group %s", serverId, groupId)
}

func NewFloatingIPExistsError(ipID string) *ServerError {
	return serverErrorf(409, "A floating IP with id %s already exists", ipID)
}

func NewFloatingIPNotFoundError(address string) *ServerError {
	return serverErrorf(404, "No such floating IP %q", address)
}

func NewServerHasFloatingIPError(serverId, ipId string) *ServerError {
	return serverErrorf(409, "Server %q already has floating IP %s", serverId, ipId)
}

func NewNoFloatingIPsToRemoveError(serverId string) *ServerError {
	return serverErrorf(409, "Server %q does not have any floating IPs to remove", serverId)
}

func NewNoFloatingIPsError(serverId, ipId string) *ServerError {
	return serverErrorf(404, "Server %q does not have floating IP %s", serverId, ipId)
}

func NewNetworkNotFoundError(network string) *ServerError {
	return serverErrorf(404, "No such network %q", network)
}

func NewNetworkAlreadyExistsError(id string) *ServerError {
	return serverErrorf(409, "A network with id %q already exists", id)
}

func NewSubnetNotFoundError(subnet string) *ServerError {
	return serverErrorf(404, "No such subnet %q", subnet)
}

func NewSubnetAlreadyExistsError(id string) *ServerError {
	return serverErrorf(409, "A subnet with id %q already exists", id)
}
