// neutronmodel is a package to allow novatestservices and neutrontestservices
// to share data related to FloatingIPs, Networks and SecurityGroups.

package neutronmodel

import (
	"net"
	"strconv"
	"sync"

	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"
	"gopkg.in/goose.v2/testservices"
)

type NeutronModel struct {
	groups       map[string]neutron.SecurityGroupV2
	rules        map[string]neutron.SecurityGroupRuleV2
	floatingIPs  map[string]neutron.FloatingIPV2
	networks     map[string]neutron.NetworkV2
	serverGroups map[string][]string
	serverIPs    map[string][]string
	nextGroupId  int
	nextRuleId   int
	nextIPId     int
	rwMu         *sync.RWMutex
}

// New setups the default data Network and Security Group for the neutron and
// nova test services.
func New() *NeutronModel {
	// Real openstack instances have a default security group "out of the box". So we add it here.
	defaultSecurityGroups := []neutron.SecurityGroupV2{
		{Id: "999", TenantId: "1", Name: "default", Description: "default group"},
	}
	// There are no create/delete network/subnet commands, so make a few default
	t := true
	f := false
	defaultNetworks := []neutron.NetworkV2{
		{ // for use by opentstack provider test
			Id:                  "1",
			Name:                "net",
			SubnetIds:           []string{"sub-net"},
			External:            false,
			AvailabilityZones:   []string{"nova"},
			PortSecurityEnabled: &t,
		},
		{ // for use by opentstack provider test
			Id:                  "2",
			Name:                "net-disabled",
			SubnetIds:           []string{"sub-net2"},
			External:            false,
			AvailabilityZones:   []string{"nova"},
			PortSecurityEnabled: &f,
		},
		{
			Id:                "999",
			Name:              "private_999",
			SubnetIds:         []string{"999-01"},
			External:          false,
			AvailabilityZones: []string{"test-available"},
		},
		{
			Id:                "998",
			Name:              "ext-net",
			SubnetIds:         []string{"998-01"},
			External:          true,
			AvailabilityZones: []string{"test-available"},
			TenantId:          "tenant-one",
		},
		{
			Id:                "997",
			Name:              "ext-net-wrong-az",
			SubnetIds:         []string{"997-01"},
			External:          true,
			AvailabilityZones: []string{"unavailable-az"},
			TenantId:          "tenant-two",
		},
	}

	neutronModel := &NeutronModel{
		groups:      make(map[string]neutron.SecurityGroupV2),
		rules:       make(map[string]neutron.SecurityGroupRuleV2),
		floatingIPs: make(map[string]neutron.FloatingIPV2),
		networks:    make(map[string]neutron.NetworkV2),
		rwMu:        &sync.RWMutex{},
	}

	for _, group := range defaultSecurityGroups {
		err := neutronModel.AddSecurityGroup(group)
		if err != nil {
			panic(err)
		}
	}
	for _, net := range defaultNetworks {
		err := neutronModel.AddNetwork(net)
		if err != nil {
			panic(err)
		}
	}
	return neutronModel
}

// convertNeutronToNovaSecurityGroup converts a nova.SecurityGroup to
// neutron.SecurityGroupV2.
func (n *NeutronModel) convertNeutronToNovaSecurityGroup(group neutron.SecurityGroupV2) nova.SecurityGroup {
	novaGroup := nova.SecurityGroup{
		TenantId:    group.TenantId,
		Id:          group.Id,
		Name:        group.Name,
		Description: group.Description,
		Rules:       []nova.SecurityGroupRule{},
	}
	var novaRules []nova.SecurityGroupRule
	for _, rule := range group.Rules {
		novaRules = append(novaRules, nova.SecurityGroupRule{
			FromPort:      rule.PortRangeMin,
			ToPort:        rule.PortRangeMax,
			Id:            rule.Id,
			ParentGroupId: rule.ParentGroupId,
			IPProtocol:    rule.IPProtocol,
			IPRange:       map[string]string{"cidr": rule.RemoteIPPrefix},
		})
	}
	if len(novaRules) > 0 {
		novaGroup.Rules = novaRules
	}
	return novaGroup
}

// convertNeutronToNovaSecurityGroup converts a neutron.SecurityGroupV2 to a
// nova.SecurityGroup.
func (n *NeutronModel) convertNovaToNeutronSecurityGroup(group nova.SecurityGroup) neutron.SecurityGroupV2 {
	neutronGroup := neutron.SecurityGroupV2{
		TenantId:    group.TenantId,
		Id:          group.Id,
		Name:        group.Name,
		Description: group.Description,
		Rules:       []neutron.SecurityGroupRuleV2{},
	}
	var neutronRules []neutron.SecurityGroupRuleV2
	for _, rule := range group.Rules {
		neutronRules = append(neutronRules, neutron.SecurityGroupRuleV2{
			PortRangeMin:   rule.FromPort,
			PortRangeMax:   rule.ToPort,
			Id:             rule.Id,
			ParentGroupId:  rule.ParentGroupId,
			IPProtocol:     rule.IPProtocol,
			RemoteIPPrefix: rule.IPRange["cidr"],
		})
	}
	if len(neutronRules) > 0 {
		neutronGroup.Rules = neutronRules
	}
	return neutronGroup
}

// UpdateSecurityGroup updates an existing security group given a
// neutron.SecurityGroupRuleV2.
func (n *NeutronModel) UpdateSecurityGroup(group neutron.SecurityGroupV2) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	existingGroup, err := n.SecurityGroup(group.Id)
	if err != nil {
		return testservices.NewSecurityGroupByIDNotFoundError(group.Id)
	}
	existingGroup.Name = group.Name
	existingGroup.Description = group.Description
	n.groups[group.Id] = *existingGroup
	return nil
}

// UpdateNovaSecurityGroup updates an existing security group given a nova.SecurityGroup.
func (n *NeutronModel) UpdateNovaSecurityGroup(group nova.SecurityGroup) error {
	return n.UpdateSecurityGroup(n.convertNovaToNeutronSecurityGroup(group))
}

// AddSecurityGroup creates a new security group given a neutron.SecurityGroupV2.
func (n *NeutronModel) AddSecurityGroup(group neutron.SecurityGroupV2) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.SecurityGroup(group.Id); err == nil {
		return testservices.NewSecurityGroupAlreadyExistsError(group.Id)
	}
	// Neutron adds 2 default egress security group rules to new security
	// groups, copy that behavior here.
	id, _ := strconv.Atoi(group.Id)
	id1 := id * 999
	id2 := id * 998
	group.Rules = []neutron.SecurityGroupRuleV2{
		{
			Direction:     "egress",
			EthernetType:  "IPv4",
			Id:            strconv.Itoa(id1),
			TenantId:      group.TenantId,
			ParentGroupId: group.Id,
		},
		{
			Direction:     "egress",
			EthernetType:  "IPv6",
			Id:            strconv.Itoa(id2),
			TenantId:      group.TenantId,
			ParentGroupId: group.Id,
		},
	}
	for _, rule := range group.Rules {
		n.rules[rule.Id] = rule
	}
	n.groups[group.Id] = group
	return nil
}

// AddNovaSecurityGroup creates a new security group given a nova.SecurityGroup.
func (n *NeutronModel) AddNovaSecurityGroup(group nova.SecurityGroup) error {
	err := n.AddSecurityGroup(n.convertNovaToNeutronSecurityGroup(group))
	if err != nil {
		return err
	}
	return err
}

// SecurityGroup retrieves an existing group by ID, data in neutron.SecurityGroupV2 form.
func (n *NeutronModel) SecurityGroup(groupId string) (*neutron.SecurityGroupV2, error) {
	if n.rwMu == nil {
		n.rwMu.RLock()
		defer n.rwMu.RUnlock()
	}
	group, ok := n.groups[groupId]
	if !ok {
		return nil, testservices.NewSecurityGroupByIDNotFoundError(groupId)
	}
	return &group, nil
}

// NovaSecurityGroup retrieves an existing group by ID, data in nova.SecurityGroup form.
func (n *NeutronModel) NovaSecurityGroup(groupId string) (*nova.SecurityGroup, error) {
	group, err := n.SecurityGroup(groupId)
	if err != nil {
		return nil, err
	}
	novaGroup := n.convertNeutronToNovaSecurityGroup(*group)
	return &novaGroup, nil
}

// SecurityGroupByName retrieves an existing named group, data in
// neutron.SecurityGroupV2 form.
func (n *NeutronModel) SecurityGroupByName(groupName string) ([]neutron.SecurityGroupV2, error) {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	var foundGrps []neutron.SecurityGroupV2
	for _, group := range n.groups {
		if group.Name == groupName {
			foundGrps = append(foundGrps, group)
		}
	}
	return foundGrps, nil
	//return nil, testservices.NewSecurityGroupByNameNotFoundError(groupName)
}

// NovaSecurityGroupByName retrieves an existing named group, data in
// nova.SecurityGroup form.
func (n *NeutronModel) NovaSecurityGroupByName(groupName string) (*nova.SecurityGroup, error) {
	groups, err := n.SecurityGroupByName(groupName)
	if err != nil {
		return nil, err
	}
	// Nova SecurityGroupsByName expects only 1 return value, return
	// the first one found.
	novaGroup := n.convertNeutronToNovaSecurityGroup(groups[0])
	return &novaGroup, nil
}

// AllSecurityGroups returns a list of all existing groups, data in
// neutron.SecurityGroupV2 form.
func (n *NeutronModel) AllSecurityGroups() []neutron.SecurityGroupV2 {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	var groups []neutron.SecurityGroupV2
	for _, group := range n.groups {
		groups = append(groups, group)
	}
	return groups
}

// AllNovaSecurityGroups returns a list of all existing groups,
// nova.SecurityGroup form.
func (n *NeutronModel) AllNovaSecurityGroups() []nova.SecurityGroup {
	neutronGroups := n.AllSecurityGroups()
	var groups []nova.SecurityGroup
	for _, group := range neutronGroups {
		groups = append(groups, n.convertNeutronToNovaSecurityGroup(group))
	}
	return groups
}

// RemoveSecurityGroup deletes an existing group.
func (n *NeutronModel) RemoveSecurityGroup(groupId string) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.SecurityGroup(groupId); err != nil {
		return err
	}
	delete(n.groups, groupId)
	return nil
}

// AddSecurityGroupRule creates a new rule in an existing group.
// This can be either an ingress or an egress rule (see the notes
// about neutron.RuleInfoV2).
func (n *NeutronModel) AddSecurityGroupRule(ruleId string, rule neutron.RuleInfoV2) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.SecurityGroupRule(ruleId); err == nil {
		return testservices.NewNeutronSecurityGroupRuleAlreadyExistsError(rule.ParentGroupId)
	}
	group, err := n.SecurityGroup(rule.ParentGroupId)
	if err != nil {
		return err
	}
	newrule := neutron.SecurityGroupRuleV2{
		ParentGroupId:  rule.ParentGroupId,
		Id:             ruleId,
		RemoteIPPrefix: rule.RemoteIPPrefix,
	}
	if rule.Direction == "ingress" || rule.Direction == "egress" {
		newrule.Direction = rule.Direction
	} else {
		return testservices.NewInvalidDirectionSecurityGroupError(rule.Direction)
	}
	if rule.PortRangeMin != 0 {
		newrule.PortRangeMin = &rule.PortRangeMin
	}
	if rule.PortRangeMax != 0 {
		newrule.PortRangeMax = &rule.PortRangeMax
	}
	if rule.IPProtocol != "" {
		newrule.IPProtocol = &rule.IPProtocol
	}
	switch rule.EthernetType {
	case "":
		// Neutron assumes IPv4 if no EthernetType is specified
		newrule.EthernetType = "IPv4"
	case "IPv4", "IPv6":
		newrule.EthernetType = rule.EthernetType
	default:
		return testservices.NewSecurityGroupRuleInvalidEthernetType(rule.EthernetType)
	}
	if newrule.RemoteIPPrefix != "" {
		ip, _, err := net.ParseCIDR(newrule.RemoteIPPrefix)
		if err != nil {
			return testservices.NewSecurityGroupRuleInvalidCIDR(rule.RemoteIPPrefix)
		}
		if (newrule.EthernetType == "IPv4" && ip.To4() == nil) ||
			(newrule.EthernetType == "IPv6" && ip.To4() != nil) {
			return testservices.NewSecurityGroupRuleParameterConflict("ethertype", newrule.EthernetType, "CIDR", newrule.RemoteIPPrefix)
		}
	}
	if group.TenantId != "" {
		newrule.TenantId = group.TenantId
	}

	group.Rules = append(group.Rules, newrule)
	n.groups[group.Id] = *group
	n.rules[newrule.Id] = newrule
	return nil
}

// AddSecurityGroupRule creates a new rule in an existing group, data in nova.RuleInfo
// form.  Rule is assumed to be ingress.
func (n *NeutronModel) AddNovaSecurityGroupRule(ruleId string, rule nova.RuleInfo) error {
	neutronRule := neutron.RuleInfoV2{
		IPProtocol:     rule.IPProtocol,
		PortRangeMin:   rule.FromPort,
		PortRangeMax:   rule.ToPort,
		RemoteIPPrefix: rule.Cidr,
		ParentGroupId:  rule.ParentGroupId,
		Direction:      "ingress",
	}
	return n.AddSecurityGroupRule(ruleId, neutronRule)
}

// HasSecurityGroupRule returns whether the given group contains the given rule,
// or (when groupId="-1") whether the given rule exists.
func (n *NeutronModel) HasSecurityGroupRule(groupId, ruleId string) bool {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	rule, ok := n.rules[ruleId]
	_, err := n.SecurityGroup(groupId)
	return ok && (groupId == "-1" || (err == nil && rule.ParentGroupId == groupId))
}

// SecurityGroupRule retrieves an existing rule by ID, data in neutron.SecurityGroupRuleV2 form.
func (n *NeutronModel) SecurityGroupRule(ruleId string) (*neutron.SecurityGroupRuleV2, error) {
	if n.rwMu == nil {
		n.rwMu.RLock()
		defer n.rwMu.RUnlock()
	}
	rule, ok := n.rules[ruleId]
	if !ok {
		return nil, testservices.NewSecurityGroupRuleNotFoundError(ruleId)
	}
	return &rule, nil
}

// SecurityGroupRule retrieves an existing rule by ID, data in nova.SecurityGroupRule form.
func (n *NeutronModel) NovaSecurityGroupRule(ruleId string) (*nova.SecurityGroupRule, error) {
	rule, err := n.SecurityGroupRule(ruleId)
	if err != nil {
		return nil, err
	}
	novaRule := &nova.SecurityGroupRule{
		IPProtocol:    rule.IPProtocol,
		FromPort:      rule.PortRangeMin,
		ToPort:        rule.PortRangeMax,
		ParentGroupId: rule.ParentGroupId,
	}
	return novaRule, nil
}

// RemoveSecurityGroupRule deletes an existing rule from its group.
func (n *NeutronModel) RemoveSecurityGroupRule(ruleId string) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	rule, err := n.SecurityGroupRule(ruleId)
	if err != nil {
		return err
	}
	if group, err := n.SecurityGroup(rule.ParentGroupId); err == nil {
		idx := -1
		for ri, ru := range group.Rules {
			if ru.Id == ruleId {
				idx = ri
				break
			}
		}
		if idx != -1 {
			group.Rules = append(group.Rules[:idx], group.Rules[idx+1:]...)
			n.groups[group.Id] = *group
		}
		// Silently ignore missing rules...
	}
	// ...or groups
	delete(n.rules, ruleId)
	return nil
}

// AddFloatingIP creates a new floating IP address in the pool, given a neutron.FloatingIPV2.
func (n *NeutronModel) AddFloatingIP(ip neutron.FloatingIPV2) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.FloatingIP(ip.Id); err == nil {
		return testservices.NewFloatingIPExistsError(ip.Id)
	}
	n.floatingIPs[ip.Id] = ip
	return nil
}

// AddNovaFloatingIP creates a new floatingIP IP address in the pool, given a nova.FloatingIP.
func (n *NeutronModel) AddNovaFloatingIP(ip nova.FloatingIP) error {
	fip := neutron.FloatingIPV2{Id: ip.Id, IP: ip.IP}
	if ip.FixedIP != nil {
		fip.FixedIP = *ip.FixedIP
	}
	return n.AddFloatingIP(fip)
}

// HasFloatingIP returns whether the given floating IP address exists.
func (n *NeutronModel) HasFloatingIP(address string) bool {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	if len(n.floatingIPs) == 0 {
		return false
	}
	for _, fip := range n.floatingIPs {
		if fip.IP == address {
			return true
		}
	}
	return false
}

// FloatingIP retrieves a neutron floating IP by ID.
func (n *NeutronModel) FloatingIP(ipId string) (*neutron.FloatingIPV2, error) {
	if n.rwMu == nil {
		n.rwMu.RLock()
		defer n.rwMu.RUnlock()
	}
	ip, ok := n.floatingIPs[ipId]
	if !ok {
		return nil, testservices.NewFloatingIPNotFoundError(ipId)
	}
	return &ip, nil
}

// NovaFloatingIP retrieves a nova floating IP by ID.
func (n *NeutronModel) NovaFloatingIP(ipId string) (*nova.FloatingIP, error) {
	fip, err := n.FloatingIP(ipId)
	if err != nil {
		return nil, err
	}
	return &nova.FloatingIP{Id: fip.Id, IP: fip.IP, FixedIP: &fip.FixedIP}, nil
}

// FloatingIPByAddr retrieves a neutron floating IP by address.
func (n *NeutronModel) FloatingIPByAddr(address string) (*neutron.FloatingIPV2, error) {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	for _, fip := range n.floatingIPs {
		if fip.IP == address {
			return &fip, nil
		}
	}
	return nil, testservices.NewFloatingIPNotFoundError(address)
}

// NovaFloatingIPByAddr retrieves a nova floating IP by address.
func (n *NeutronModel) NovaFloatingIPByAddr(address string) (*nova.FloatingIP, error) {
	fip, err := n.FloatingIPByAddr(address)
	if err != nil {
		return nil, err
	}
	return &nova.FloatingIP{Id: fip.Id, IP: fip.IP, FixedIP: &fip.FixedIP}, nil
}

// AllFloatingIPs returns a list of all created floating IPs, data in
// neutron.FloatingIPV2 form.
func (n *NeutronModel) AllFloatingIPs() []neutron.FloatingIPV2 {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	var fips []neutron.FloatingIPV2
	for _, fip := range n.floatingIPs {
		fips = append(fips, fip)
	}
	return fips
}

// AllNovaFloatingIPs returns a list of all created floating IPs, data in
// nova.FloatingIP form.
func (n *NeutronModel) AllNovaFloatingIPs() []nova.FloatingIP {
	neutronFips := n.AllFloatingIPs()
	var novaFips []nova.FloatingIP
	for _, fip := range neutronFips {
		novaFips = append(novaFips, nova.FloatingIP{
			Id:      fip.Id,
			IP:      fip.IP,
			FixedIP: &fip.FixedIP,
		})
	}
	return novaFips
}

// RemoveFloatingIP deletes an existing floating IP by ID.
func (n *NeutronModel) RemoveFloatingIP(ipId string) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.FloatingIP(ipId); err != nil {
		return err
	}
	delete(n.floatingIPs, ipId)
	return nil
}

// UpdateNovaFloatingIP updates the Fixed IP, given a nova.FloatingIP.
func (n *NeutronModel) UpdateNovaFloatingIP(fip *nova.FloatingIP) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	ip, ok := n.floatingIPs[fip.Id]
	if !ok {
		return testservices.NewFloatingIPNotFoundError(fip.Id)
	}
	if fip.FixedIP != nil {
		ip.FixedIP = *fip.FixedIP
	}
	n.floatingIPs[fip.Id] = ip
	return nil
}

// AllNetworks returns a list of all existing networks in neutron.NetworkV2.
func (n *NeutronModel) AllNetworks() (networks []neutron.NetworkV2) {
	n.rwMu.RLock()
	defer n.rwMu.RUnlock()
	for _, net := range n.networks {
		networks = append(networks, net)
	}
	return networks
}

// AllNovaNetworks returns of list of all existing networks in nova.Network form.
func (n *NeutronModel) AllNovaNetworks() (networks []nova.Network) {
	neutronNetworks := n.AllNetworks()
	var novaNetworks []nova.Network
	for _, net := range neutronNetworks {
		// copy Id and Name to new Nova Network, leave off Cidr for
		// Neutron networking keeps Cidr in a subnet, not a network.
		novaNetworks = append(novaNetworks, nova.Network{
			Id:    net.Id,
			Label: net.Name,
		})
	}
	return novaNetworks
}

// Network retrieves the network in Neutron Network form by ID.
func (n *NeutronModel) Network(networkId string) (*neutron.NetworkV2, error) {
	if n.rwMu == nil {
		n.rwMu.RLock()
		defer n.rwMu.RUnlock()
	}
	network, ok := n.networks[networkId]
	if !ok {
		return nil, testservices.NewNetworkNotFoundError(networkId)
	}
	return &network, nil
}

// NovaNetwork retrieves the network in Nova Network form by ID.
func (n *NeutronModel) NovaNetwork(networkId string) (*nova.Network, error) {
	neutronNet, err := n.Network(networkId)
	if err != nil {
		return nil, err
	}
	return &nova.Network{Id: neutronNet.Id, Label: neutronNet.Name}, nil
}

// AddNetwork creates a new network.
func (n *NeutronModel) AddNetwork(network neutron.NetworkV2) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.Network(network.Id); err == nil {
		return testservices.NewNetworkAlreadyExistsError(network.Id)
	}
	if network.SubnetIds == nil {
		network.SubnetIds = []string{}
	}
	n.networks[network.Id] = network
	return nil
}

// RemoveNetwork deletes an existing group.
func (n *NeutronModel) RemoveNetwork(netId string) error {
	n.rwMu.Lock()
	defer n.rwMu.Unlock()
	if _, err := n.Network(netId); err != nil {
		return err
	}
	delete(n.networks, netId)
	return nil
}
