// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"
)

type CloudCredentialSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&CloudCredentialSerializationSuite{})

func (s *CloudCredentialSerializationSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "cloudCredential"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importCloudCredential(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["owner"] = ""
		m["cloud"] = ""
		m["name"] = ""
		m["auth-type"] = ""
	}
}

func (s *CloudCredentialSerializationSuite) TestMissingOwner(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "owner")
	_, err := importCloudCredential(testMap)
	c.Check(err.Error(), gc.Equals, "cloudCredential v1 schema check failed: owner: expected string, got nothing")
}

func (s *CloudCredentialSerializationSuite) TestMissingCloud(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "cloud")
	_, err := importCloudCredential(testMap)
	c.Check(err.Error(), gc.Equals, "cloudCredential v1 schema check failed: cloud: expected string, got nothing")
}

func (s *CloudCredentialSerializationSuite) TestMissingName(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "name")
	_, err := importCloudCredential(testMap)
	c.Check(err.Error(), gc.Equals, "cloudCredential v1 schema check failed: name: expected string, got nothing")
}

func (s *CloudCredentialSerializationSuite) TestMissingAuthType(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "auth-type")
	_, err := importCloudCredential(testMap)
	c.Check(err.Error(), gc.Equals, "cloudCredential v1 schema check failed: auth-type: expected string, got nothing")
}

func (*CloudCredentialSerializationSuite) allArgs() CloudCredentialArgs {
	return CloudCredentialArgs{
		Owner:    names.NewUserTag("me"),
		Cloud:    names.NewCloudTag("altostratus"),
		Name:     "creds",
		AuthType: "fuzzy",
		Attributes: map[string]string{
			"key": "value",
		},
	}
}

func (s *CloudCredentialSerializationSuite) TestAllArgs(c *gc.C) {
	args := s.allArgs()
	creds := newCloudCredential(args)

	c.Check(creds.Owner(), gc.Equals, args.Owner.Id())
	c.Check(creds.Cloud(), gc.Equals, args.Cloud.Id())
	c.Check(creds.Name(), gc.Equals, args.Name)
	c.Check(creds.AuthType(), gc.Equals, args.AuthType)
	c.Check(creds.Attributes(), jc.DeepEquals, args.Attributes)
}

func (s *CloudCredentialSerializationSuite) TestParsingSerializedData(c *gc.C) {
	args := s.allArgs()
	initial := newCloudCredential(args)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	imported, err := importCloudCredential(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(imported, jc.DeepEquals, initial)
}
