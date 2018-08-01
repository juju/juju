// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// CreateAcl creates an access control list (ACL) to control
// the traffic between virtual NICs.
// An ACL consists of one or more security rules that is applied
// to a virtual NIC set. Each security rule may refer to a virtual
// NIC set in either the source or destination.See Workflow for
// After creating an ACL, you can associate it to one or more virtual NIC sets.
func (c *Client) CreateAcl(
	name string,
	description string,
	enabledFlag bool,
	tags []string,
) (resp response.Acl, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty acl name",
		)
	}

	url := c.endpoints["acl"] + "/"

	params := struct {
		Name        string   `json:"name"`
		Description string   `json:"description,omitempty"`
		EnabledFlag bool     `json:"enabledFlag"`
		Tags        []string `json:"tags,omitempty"`
	}{
		Name:        name,
		Description: description,
		EnabledFlag: enabledFlag,
		Tags:        tags,
	}

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteAcl deletes specific acl that has the name
//
// If you no longer need to use an ACL, you can delete it.
// Remember, however, that security rules reference ACLs and
// ACLs are applied to vNICsets.
//
// If you delete an ACL that is referenced in one or more security rjkkkjules,
// those security rules can no longer be used.
//
// If you delete an ACL that is applied to a vNICset, the security rules in
// that ACL no longer apply to that vNICset. Before deleting an ACL,
// ensure that other ACLs are in place to provide access to relevant vNICsets.
//
// If you delete all the ACLs applied to a vNICset, some vNICs in that vNICset
// might become unreachable.
//
// If you want to disable an ACL and not delete it, use the UpdateAcl method
func (c *Client) DeleteAcl(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"Empty acl name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["acl"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllAcls retrieves details of all the ACLs
// that are available in the specified container.
func (c *Client) AllAcls(filter []Filter) (resp response.AllAcls, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/", c.endpoints["acl"], c.identify)

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

// AclDetails retrieves information about the specified ACL.
func (c *Client) AclDetails(name string) (resp response.Acl, err error) {
	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty acl name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["acl"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateAcl can update the description and tag fields for an ACL.
// You can also disable an ACL by setting the value of the enabledFlag to false.
// When you disable an ACL, it also disables the flow of traffic
// allowed by the security rules in scope of the ACL.
func (c *Client) UpdateAcl(
	currentName string,
	newName string,
	description string,
	enableFlag bool,
	tags []string,
) (resp response.Acl, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty acl name",
		)
	}

	if newName == "" {
		newName = currentName
	}

	params := response.Acl{
		Description: description,
		Name:        newName,
		EnableFlag:  enableFlag,
		Tags:        tags,
	}

	url := fmt.Sprintf("%s%s", c.endpoints["acl"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		body: &params,
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
