// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"encoding/json"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cloud"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type regionsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&regionsSuite{})

func (s *regionsSuite) TestListRegionsInvalidCloud(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "invalid")
	c.Assert(err, gc.ErrorMatches, "cloud invalid not found")
}

func (s *regionsSuite) TestListRegionsInvalidArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws", "another")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["another"\]`)
}

func (s *regionsSuite) TestListRegions(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
us-east-1
us-east-2
us-west-1
us-west-2
ca-central-1
eu-west-1
eu-west-2
eu-central-1
ap-south-1
ap-southeast-1
ap-southeast-2
ap-northeast-1
ap-northeast-2
sa-east-1

`[1:])
}

func (s *regionsSuite) TestListRegionsBuiltInCloud(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "localhost")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "localhost\n\n")
}

func (s *regionsSuite) TestListRegionsYaml(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
us-east-1:
  endpoint: https://ec2.us-east-1.amazonaws.com
us-east-2:
  endpoint: https://ec2.us-east-2.amazonaws.com
us-west-1:
  endpoint: https://ec2.us-west-1.amazonaws.com
us-west-2:
  endpoint: https://ec2.us-west-2.amazonaws.com
ca-central-1:
  endpoint: https://ec2.ca-central-1.amazonaws.com
eu-west-1:
  endpoint: https://ec2.eu-west-1.amazonaws.com
eu-west-2:
  endpoint: https://ec2.eu-west-2.amazonaws.com
eu-central-1:
  endpoint: https://ec2.eu-central-1.amazonaws.com
ap-south-1:
  endpoint: https://ec2.ap-south-1.amazonaws.com
ap-southeast-1:
  endpoint: https://ec2.ap-southeast-1.amazonaws.com
ap-southeast-2:
  endpoint: https://ec2.ap-southeast-2.amazonaws.com
ap-northeast-1:
  endpoint: https://ec2.ap-northeast-1.amazonaws.com
ap-northeast-2:
  endpoint: https://ec2.ap-northeast-2.amazonaws.com
sa-east-1:
  endpoint: https://ec2.sa-east-1.amazonaws.com
`[1:])
}

func (s *regionsSuite) TestListGCERegions(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "google")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
us-east1
us-central1
us-west1
europe-west1
asia-east1
asia-northeast1
asia-southeast1
australia-southeast1

`[1:])
}

func (s *regionsSuite) TestListGCERegionsYaml(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "google", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
us-east1:
  endpoint: https://www.googleapis.com
us-central1:
  endpoint: https://www.googleapis.com
us-west1:
  endpoint: https://www.googleapis.com
europe-west1:
  endpoint: https://www.googleapis.com
asia-east1:
  endpoint: https://www.googleapis.com
asia-northeast1:
  endpoint: https://www.googleapis.com
asia-southeast1:
  endpoint: https://www.googleapis.com
australia-southeast1:
  endpoint: https://www.googleapis.com
`[1:])
}

type regionDetails struct {
	Endpoint         string `json:"endpoint"`
	IdentityEndpoint string `json:"identity-endpoint"`
	StorageEndpoint  string `json:"storage-endpoint"`
}

func (s *regionsSuite) TestListRegionsJson(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "azure", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	var data map[string]regionDetails
	err = json.Unmarshal([]byte(out), &data)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, map[string]regionDetails{
		"northeurope":        {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"eastasia":           {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"japanwest":          {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"westcentralus":      {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"centralus":          {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"westus2":            {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"eastus2":            {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"japaneast":          {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"northcentralus":     {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"southcentralus":     {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"australiaeast":      {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"brazilsouth":        {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"centralindia":       {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"southindia":         {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"westeurope":         {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"westindia":          {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"westus":             {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"australiasoutheast": {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"eastus":             {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"southeastasia":      {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"canadacentral":      {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"canadaeast":         {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"uksouth":            {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"ukwest":             {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"koreasouth":         {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
		"koreacentral":       {Endpoint: "https://management.azure.com", IdentityEndpoint: "https://graph.windows.net", StorageEndpoint: "https://core.windows.net"},
	})
}
