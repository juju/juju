// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
)

// CreatesSecList a security list. After creating security
// lists, you can add instances to them by using the HTTP request,
// CreateSecAssociation (Create a Security Association).
func (c *Client) CreateSecList(
	description string,
	name string,
	outbound_cidr_policy common.SecRuleAction,
	policy common.SecRuleAction,
) (resp response.SecList, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure list name")
	}

	if err = outbound_cidr_policy.Validate(); err != nil {
		return resp, err
	}

	if err = policy.Validate(); err != nil {
		return resp, err
	}

	params := struct {
		Description          string               `json:"description,omitempty"`
		Name                 string               `json:"name"`
		Outbound_cidr_policy common.SecRuleAction `json:"outbound_cidr_policy"`
		Policy               common.SecRuleAction `json:"policy"`
	}{
		Description:          description,
		Name:                 name,
		Outbound_cidr_policy: outbound_cidr_policy,
		Policy:               policy,
	}

	url := c.endpoints["seclist"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		body: &params,
		verb: "POST",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteSecList the specified security list. No response is returned.<Paste>
func (c *Client) DeleteSecList(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New("go-oracle-cloud: Empty secure list name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["seclist"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllSecLists retrieves details of the security lists that are in the specified
// container and match the specified query criteria.
// You can filter by name
func (c *Client) AllSecLists(filter []Filter) (resp response.AllSecLists, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["seclist"], c.identify, c.username)

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

// SecListDetails retrieves information about the specified security list.
func (c *Client) SecListDetails(name string) (resp response.SecList, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure list name")
	}

	url := fmt.Sprintf("%s%s", c.endpoints["seclist"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// Updates inbound policy, outbound policy, and description for
// the specified security list.
// newName could be "" if you don't want to change the name
// but it's required at leas to have a currentName
// outbound_cidr_policy is the policy for outbound traffic
// from the security list. You can specify one of the following values:
// deny: Packets are dropped. No response is sent.
// reject: Packets are dropped, but a response is sent.
// permit(default): Packets are allowed.
func (c *Client) UpdateSecList(
	description string,
	currentName string,
	newName string,
	outbound_cidr_policy common.SecRuleAction,
	policy common.SecRuleAction,
) (resp response.SecList, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New("go-oracle-cloud: Empty secure list name")
	}

	if newName == "" {
		newName = currentName
	}

	if err = outbound_cidr_policy.Validate(); err != nil {
		return resp, err
	}

	if err = policy.Validate(); err != nil {
		return resp, err
	}

	params := struct {
		Policy               common.SecRuleAction `json:"policy"`
		Description          string               `json:"description,omitempty"`
		Name                 string               `json:"name"`
		Outbound_cidr_policy common.SecRuleAction `json:"outbound_cidr_policy"`
	}{
		Description:          description,
		Name:                 newName,
		Outbound_cidr_policy: outbound_cidr_policy,
		Policy:               policy,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["seclist"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		body: &params,
		verb: "PUT",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
