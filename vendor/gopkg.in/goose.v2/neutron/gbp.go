// goose/neutron/gbp - Go package to interact with GBP plugin of the
// OpenStack Network Service (Neutron) API.
// It currently supports small subset of the API needed to create a
// policy target.
// There is no documentation for GBP API so this code is reverse engineered
// from Python gbp client code.

package neutron

import (
	"fmt"
	"net/http"
	"net/url"

	"gopkg.in/goose.v2/client"
	"gopkg.in/goose.v2/errors"
	goosehttp "gopkg.in/goose.v2/http"
)

const (
	ApiApplicationPolicyGroupsV2 = "grouppolicy/application_policy_groups"
	ApiExternalPolicysV2         = "grouppolicy/external_policys"
	ApiExternalSegmentsV2        = "grouppolicy/external_segments"
	ApiL2PolicysV2               = "grouppolicy/l2_policys"
	ApiL3PolicysV2               = "grouppolicy/l3_policys"
	ApiNatPoolsV2                = "grouppolicy/nat_pools"
	ApiNetworkServicePolicysV2   = "grouppolicy/network_service_policys"
	ApiPolicyActionsV2           = "grouppolicy/policy_actions"
	ApiPolicyClassifiersV2       = "grouppolicy/policy_classifiers"
	ApiPolicyRulesV2             = "grouppolicy/policy_rules"
	ApiPolicyRuleSetsV2          = "grouppolicy/policy_rule_sets"
	ApiPolicyTargetsV2           = "grouppolicy/policy_targets"
	ApiPolicyTargetGroupsV2      = "grouppolicy/policy_target_groups"
)

type PolicyTargetV2 struct {
	Id                  string   `json:"id,omitempty"`
	Name                string   `json:"name,omitempty"`
	Description         string   `json:"description,omitempty"`
	PolicyTargetGroupId string   `json:"policy_target_group_id"`
	PortId              string   `json:"port_id,omitempty"`
	FixedIps            []string `json:"fixed_ips,omitempty"`
}

// ListPolicyTargetsV2 lists policy targets filtered by filter.
func (c *Client) ListPolicyTargetsV2(filter ...*Filter) ([]PolicyTargetV2, error) {
	var resp struct {
		PolicyTargets []PolicyTargetV2 `json:"policy_targets"`
	}
	var params *url.Values
	if len(filter) > 0 {
		params = &filter[0].v
	}
	requestData := goosehttp.RequestData{RespValue: &resp, Params: params}
	err := c.client.SendRequest(client.GET, "network", "v2.0", ApiPolicyTargetsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get list of networks")
	}
	return resp.PolicyTargets, nil
}

// GetPolicyTargetV2 fetches single policy target by id.
func (c *Client) GetPolicyTargetV2(ptId string) (*PolicyTargetV2, error) {
	var resp struct {
		PolicyTarget PolicyTargetV2 `json:"policy_target"`
	}
	url := fmt.Sprintf("%s/%s", ApiPolicyTargetsV2, ptId)
	requestData := goosehttp.RequestData{RespValue: &resp}
	err := c.client.SendRequest(client.GET, "network", "v2.0", url, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to get policy_target_group detail")
	}
	return &resp.PolicyTarget, nil
}

// CreatePolicyTargetV2 creates policy target as declared in pt. It returns policy target
// with Id field filled.
func (c *Client) CreatePolicyTargetV2(pt PolicyTargetV2) (*PolicyTargetV2, error) {
	var req struct {
		PolicyTargetV2 `json:"policy_target"`
	}
	req.PolicyTargetV2 = pt

	var resp struct {
		PolicyTargetV2 `json:"policy_target"`
	}
	requestData := goosehttp.RequestData{
		ReqValue:       req,
		RespValue:      &resp,
		ExpectedStatus: []int{http.StatusCreated},
	}
	err := c.client.SendRequest(client.POST, "network", "v2.0", ApiPolicyTargetsV2, &requestData)
	if err != nil {
		return nil, errors.Newf(err, "failed to create policy target")
	}
	return &resp.PolicyTargetV2, nil
}

// DeletePolicyTargetV2 deletes policy targed identified by Id.
func (c *Client) DeletePolicyTargetV2(ptId string) error {
	url := fmt.Sprintf("%s/%s", ApiPolicyTargetsV2, ptId)
	requestData := goosehttp.RequestData{ExpectedStatus: []int{http.StatusNoContent}}
	err := c.client.SendRequest(client.DELETE, "network", "v2.0", url, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to delete policy target id %s", ptId)
	}
	return err
}
