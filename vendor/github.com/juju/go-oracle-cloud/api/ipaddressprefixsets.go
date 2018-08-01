package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// IpAddressPrefixSetParams params to feed the CreateIpAddressPrefixSet
// An IP address prefix set lists IPv4 addresses in the CIDR address prefix format.
// After creating an IP address prefix set, you can specify it as a source or destination
// for permitted traffic while creating a security rule.
type IpAddressPrefixSetParams struct {

	// Description is a description of the ip address prefix set
	Description string `json:"description,omitmepty"`

	// IpAddressPrefixes is a list of CIDR IPv4 prefixes assigned in the virtual network.
	IpAddressPrefixes []string `json:"ipAddressPrefixes"`

	// Name is the name of the ip address prefix set
	Name string `json:"name"`

	// Tags is strings that you can use to tag the IP address prefix set.
	Tags []string `json:"tags,omitempty"`
}

// validate validates the params provided
func (i IpAddressPrefixSetParams) validate() (err error) {
	if i.Name == "" {
		return errors.New(
			"go-oracle-cloud: Empty ip address prefix set name",
		)
	}

	return nil
}

// CreateIpAddressPrefixSet creates a new ip address prefix set in the
// oracle cloud account
func (c *Client) CreateIpAddressPrefixSet(
	p IpAddressPrefixSetParams,
) (resp response.IpAddressPrefixSet, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	url := c.endpoints["ipaddressprefixset"] + "/"

	if err = c.request(paramsRequest{
		url:  url,
		verb: "POST",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// UpdateIpAddressPrefixSet changes, updates the ip address prefix set in the
// oracle cloud account
func (c *Client) UpdateIpAddressPrefixSet(
	p IpAddressPrefixSetParams,
	currentName string,
) (resp response.IpAddressPrefixSet, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if err = p.validate(); err != nil {
		return resp, err
	}

	if currentName == "" {
		return resp, errors.New(
			"go-oracle-cloud Empty ip address prefix set current name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressprefixset"], currentName)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "PUT",
		resp: &resp,
		body: &p,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// IpAddressPrefixSetDetails retrives details
// of a ip address prefix given a name
func (c *Client) IpAddressPrefixSetDetails(
	name string,
) (resp response.IpAddressPrefixSet, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud Empty ip address prefix set name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressprefixset"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}

// DeleteIpAddressPrefixSet deletes an IP address prefix set.
func (c *Client) DeleteIpAddressPrefixSet(name string) (err error) {
	if !c.isAuth() {
		return errNotAuth
	}

	if name == "" {
		return errors.New(
			"go-oracle-cloud Empty ip address prefix set name",
		)
	}

	url := fmt.Sprintf("%s%s", c.endpoints["ipaddressprefixset"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "DELETE",
	}); err != nil {
		return err
	}

	return nil
}

// AllIpAddressPrefixSets retrieves details of all the IP address
// prefix sets that are available in the specified container
func (c *Client) AllIpAddressPrefixSets(
	filter []Filter,
) (resp response.AllIpAddressPrefixSets, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	url := fmt.Sprintf("%s/Compute-%s/%s/",
		c.endpoints["ipaddressprefixset"], c.identify, c.username)

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
