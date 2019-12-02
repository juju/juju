// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/firewall"
)

// FirewallRule instances describe the ingress networks
// whitelist/blacklist for a given well known service.
// Primarily, whitelisting allowed ingress subnets is the
// main use case. However, blacklisting subnets is useful
// to allow restrictions placed on incoming traffic for
// cross model relations, where the source of traffic is
// requested from the consuming side.
// WellKnownService is either a well known internet service
// (currently just SSH) or a Juju defined value.
// Supported values are:
// - ssh
// - juju-controller
// - juju-application-offer
type FirewallRule struct {
	id string

	wellKnownService firewall.WellKnownServiceType

	whitelistCIDRs []string
}

func NewFirewallRule(serviceType firewall.WellKnownServiceType, cidrs []string) FirewallRule {
	return FirewallRule{whitelistCIDRs: cidrs, wellKnownService: serviceType}
}

func (f FirewallRule) ID() string {
	return f.id
}

// WellKnownService is the known service for the firewall rules entity.
func (f FirewallRule) WellKnownService() firewall.WellKnownServiceType {
	return f.wellKnownService
}

// WhitelistCIDRS is the whitelist CIDRs for the rule.
func (f FirewallRule) WhitelistCIDRs() []string {
	return f.whitelistCIDRs
}

type firewallRulesDoc struct {
	Id               string   `bson:"_id"`
	WellKnownService string   `bson:"known-service"`
	WhitelistCIDRS   []string `bson:"whitelist-cidrs"`
}

func (r *firewallRulesDoc) toRule() *FirewallRule {
	return &FirewallRule{
		id:               r.Id,
		wellKnownService: firewall.WellKnownServiceType(r.WellKnownService),
		whitelistCIDRs:   r.WhitelistCIDRS,
	}
}

// FirewallRuler instances provide access to firewall rules in state.
type FirewallRuler interface {
	Save(service firewall.WellKnownServiceType, whiteListCidrs []string) (FirewallRule, error)
	Rule(service firewall.WellKnownServiceType) (FirewallRule, error)
	AllRules() ([]FirewallRule, error)
}

type firewallRulesState struct {
	st *State
}

// NewFirewallRules creates a FirewallRule instance backed by a state.
func NewFirewallRules(st *State) *firewallRulesState {
	return &firewallRulesState{st: st}
}

// Save stores the specified firewall rule.
func (fw *firewallRulesState) Save(rule FirewallRule) error {
	if err := checkModelActive(fw.st); err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(int) ([]txn.Op, error) {
		return fw.GetSaveTransactionOps(rule, false)
	}
	if err := fw.st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "failed to create firewall rules")
	}

	return nil
}

func (fw *firewallRulesState) GetSaveTransactionOps(rule FirewallRule, isMigrating bool) ([]txn.Op, error) {
	if err := rule.WellKnownService().Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	for _, cidr := range rule.WhitelistCIDRs() {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, errors.NotValidf("CIDR %q", cidr)
		}
	}
	serviceStr := string(rule.WellKnownService())
	doc := firewallRulesDoc{
		Id:               serviceStr,
		WellKnownService: serviceStr,
		WhitelistCIDRS:   rule.WhitelistCIDRs(),
	}

	model, err := fw.st.Model()
	if err != nil {
		return nil, errors.Annotate(err, "failed to load model")
	}
	_, err = fw.Rule(rule.WellKnownService())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	if err == nil {
		ops = []txn.Op{{
			C:      firewallRulesC,
			Id:     serviceStr,
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"whitelist-cidrs", rule.WhitelistCIDRs()}}},
			},
		}}
	} else {
		doc.WhitelistCIDRS = rule.WhitelistCIDRs()
		ops = []txn.Op{{
			C:      firewallRulesC,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}
	}
	if !isMigrating {
		ops = append(ops, model.assertActiveOp())
	}
	return ops, nil
}

// Rule returns the firewall rule for the specified service.
func (fw *firewallRulesState) Rule(service firewall.WellKnownServiceType) (*FirewallRule, error) {
	coll, closer := fw.st.db().GetCollection(firewallRulesC)
	defer closer()

	var doc firewallRulesDoc
	err := coll.FindId(string(service)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("firewall rules for service %v", service)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return doc.toRule(), nil
}

// AllRules returns all the firewall rules.
func (fw *firewallRulesState) AllRules() ([]*FirewallRule, error) {
	coll, closer := fw.st.db().GetCollection(firewallRulesC)
	defer closer()

	var docs []firewallRulesDoc
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*FirewallRule, len(docs))
	for i, doc := range docs {
		result[i] = doc.toRule()
	}
	return result, nil
}
