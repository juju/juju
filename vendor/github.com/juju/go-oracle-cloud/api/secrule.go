// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// SecRuleParams type used as params in CreateSecRule func
type SecRuleParams struct {

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

	// Name is the name of the security rule
	Name string `json:"name"`

	// Dst_list is the name
	// of the destination security list or security IP list.
	// You must use the prefix seclist: or seciplist
	// : to identify the list type.
	// Note: You can specify a security IP list as
	// the destination in a secrule, provided src_list is
	// a security list that has DENY as its outbound policy.
	// You cannot specify any of the security IP lists
	// in the /oracle/public container as a destination in a secrule.
	Dst_list string `json:"dst_list"`

	// Scr_list is the name of the source security
	// list or security IP list. You must use the prefix seclist:
	// or seciplist: to identify the list type
	Src_list string `json:"src_list"`
}

// validate will validate all the sec rule params
func (s SecRuleParams) validate() (err error) {
	if s.Name == "" {
		return errors.New("go-oracle-cloud: Empty secure rule name")
	}

	if err = s.Action.Validate(); err != nil {
		return err
	}

	if s.Application == "" {
		return errors.New("go-oracle-cloud: Empty secure rule application name")
	}

	if s.Src_list == "" {
		return errors.New("go-oracle-cloud: Empty source list in secure rule")
	}

	if s.Dst_list == "" {
		return errors.New("go-oracle-cloud: Empty destination list in secure rule")
	}

	return nil
}

// CreateSecRule creates a new security rule. A security rule defines network access over a specified
// protocol between instances in two security lists, or from a
// set of external hosts (an IP list) to instances in a security list.
func (c *Client) CreateSecRule(p SecRuleParams) (resp response.SecRule, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["secrule"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		body: &p,
		verb: "POST",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSecRule deletes a security role inside the oracle
// cloud account. If the security rule is not found this will return nil
func (c *Client) DeleteSecRule(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty secure rule name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secrule"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// SecRuleDetails retrives details on a specific security rule
func (c *Client) SecRuleDetails(name string) (resp response.SecRule, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure rule name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secrule"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// AllSecRules retrives all security rulues from the oracle cloud account
func (c *Client) AllSecRules(filter []Filter) (resp response.AllSecRules, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["secrule"], c.identify, c.username)

	if err = c.request(paramsRequest{
		url:    url,
		verb:   "GET",
		resp:   &resp,
		filter: filter,
	}); err != nil {
		return resp, err
	}

	return resp, nil

}

// UpdateSecRule modifies the security rule with the currentName
func (c *Client) UpdateSecRule(
	p SecRuleParams,
	currentName string,
) (resp response.SecRule, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure rule current name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["secrule"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		body: &p,
		verb: "PUT",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// SecRuleNames retrives all secure rule names in the oracle cloud account
func (c *Client) SecRuleNames() (resp response.DirectoryNames, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["secrule"], c.identify, c.username)

	if err = c.request(paramsRequest{
		directory: true,
		url:       url,
		verb:      "GET",
		resp:      &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
