// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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

	wellKnownService string

	whitelistCIDRs []string
}

func NewFirewallRule(serviceType WellKnownServiceType, cidrs []string) FirewallRule {
	return FirewallRule{whitelistCIDRs: cidrs, wellKnownService: string(serviceType)}
}

func (f FirewallRule) ID() string {
	return f.id
}

// WellKnownServiceType is the known service for the firewall rules entity.
func (f FirewallRule) WellKnownServiceType() WellKnownServiceType {
	return WellKnownServiceType(f.wellKnownService)
}

// WellKnownService is the known service for the firewall rules entity as a string.
func (f FirewallRule) WellKnownService() string {
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
		wellKnownService: r.WellKnownService,
		whitelistCIDRs:   r.WhitelistCIDRS,
	}
}

// FirewallRuler instances provide access to firewall rules in state.
type FirewallRuler interface {
	Save(service WellKnownServiceType, whiteListCidrs []string) (FirewallRule, error)
	Rule(service WellKnownServiceType) (FirewallRule, error)
	AllRules() ([]FirewallRule, error)
}

const (
	// SSHRule is a rule for SSH connections.
	SSHRule = WellKnownServiceType("ssh")

	// JujuControllerRule is a rule for connections to the Juju controller.
	JujuControllerRule = WellKnownServiceType("juju-controller")

	// JujuApplicationOfferRule is a rule for connections to a Juju offer.
	JujuApplicationOfferRule = WellKnownServiceType("juju-application-offer")
)

// WellKnownServiceType defines a service for which firewall rules may be applied.
type WellKnownServiceType string

func (v WellKnownServiceType) validate() error {
	switch v {
	case SSHRule, JujuControllerRule, JujuApplicationOfferRule:
		return nil
	}
	return errors.NotValidf("well known service type %q", v)
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
	if err := rule.WellKnownServiceType().validate(); err != nil {
		return errors.Trace(err)
	}
	for _, cidr := range rule.WhitelistCIDRs() {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return errors.NotValidf("CIDR %q", cidr)
		}
	}
	serviceStr := string(rule.WellKnownServiceType())
	doc := firewallRulesDoc{
		Id:               serviceStr,
		WellKnownService: serviceStr,
		WhitelistCIDRS:   rule.WhitelistCIDRs(),
	}
	buildTxn := func(int) ([]txn.Op, error) {
		model, err := fw.st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if err := checkModelActive(fw.st); err != nil {
			return nil, errors.Trace(err)
		}

		_, err = fw.Rule(rule.WellKnownServiceType())
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
			}, model.assertActiveOp()}
		} else {
			doc.WhitelistCIDRS = rule.WhitelistCIDRs()
			ops = []txn.Op{{
				C:      firewallRulesC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}, model.assertActiveOp()}
		}
		return ops, nil
	}
	if err := fw.st.db().Run(buildTxn); err != nil {
		return errors.Annotate(err, "failed to create firewall rules")
	}

	return nil
}

// Rule returns the firewall rule for the specified service.
func (fw *firewallRulesState) Rule(service WellKnownServiceType) (*FirewallRule, error) {
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
