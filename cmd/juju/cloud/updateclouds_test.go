// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type updateCloudsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeUpdateCloudsAPI
	store jujuclient.ClientStore
}

var _ = gc.Suite(&updateCloudsSuite{})

func (s *updateCloudsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeUpdateCloudsAPI{}
	s.store = jujuclient.NewMemStore()
}

func encodeCloudYAML(c *gc.C, yaml string) string {
	// TODO(wallyworld) - move test signing key elsewhere
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(sstesting.SignedMetadataPrivateKey))
	c.Assert(err, jc.ErrorIsNil)
	privateKey := keyring[0].PrivateKey
	err = privateKey.Decrypt([]byte(sstesting.PrivateKeyPassphrase))
	c.Assert(err, jc.ErrorIsNil)

	var buf bytes.Buffer
	plaintext, err := clearsign.Encode(&buf, privateKey, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = plaintext.Write([]byte(yaml))
	c.Assert(err, jc.ErrorIsNil)
	err = plaintext.Close()
	c.Assert(err, jc.ErrorIsNil)
	return string(buf.Bytes())
}

func (s *updateCloudsSuite) setupTestServer(c *gc.C, serverContent string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch serverContent {
		case "404":
			w.WriteHeader(http.StatusNotFound)
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
		case "unsigned":
			fmt.Fprintln(w, serverContent)
			return
		}
		signedContent := encodeCloudYAML(c, serverContent)
		fmt.Fprintln(w, signedContent)
	}))
}

// func (s *updateCloudsSuite) TestBadArgs(c *gc.C) {
// 	updateCmd := cloud.NewUpdateCloudsCommandForTest("")
// 	_, err := cmdtesting.RunCommand(c, updateCmd, "extra")
// 	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
// }

func (s *updateCloudsSuite) run(c *gc.C, url, errMsg string) string {
	fake := newFakeCloudMetadataStore()
	updateCmd := cloud.NewUpdateCloudsCommandForTest(url, fake, s.store, nil)
	out, err := cmdtesting.RunCommand(c, updateCmd)
	if errMsg == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		errString := strings.Replace(err.Error(), "\n", "", -1)
		c.Assert(errString, gc.Matches, errMsg)
	}
	return cmdtesting.Stderr(out)
}

func (s *updateCloudsSuite) Test404(c *gc.C) {
	ts := s.setupTestServer(c, "404")
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, "Fetching latest public cloud list...Public cloud list is unavailable right now.")
}

func (s *updateCloudsSuite) Test401(c *gc.C) {
	ts := s.setupTestServer(c, "401")
	defer ts.Close()

	s.run(c, ts.URL, "unauthorised access to URL .*")
}

func (s *updateCloudsSuite) TestUnsignedData(c *gc.C) {
	ts := s.setupTestServer(c, "unsigned")
	defer ts.Close()

	s.run(c, ts.URL, "error receiving updated cloud data: no PGP signature embedded in plain text data")
}

func (s *updateCloudsSuite) TestBadDataOnServer(c *gc.C) {
	ts := s.setupTestServer(c, "bad data")
	defer ts.Close()

	s.run(c, ts.URL, ".*invalid cloud data received when updating clouds.*")
}

var sampleUpdateCloudData = `
clouds:
  aws:
    type: ec2
    auth-types: [access-key]
    endpoint: http://region
    regions:
      region:
        endpoint: http://region/1.0
`[1:]

func (s *updateCloudsSuite) TestNoNewData(c *gc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)

	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, "Fetching latest public cloud list...Your list of public clouds is up to date, see `juju clouds`.")
}

func (s *updateCloudsSuite) TestFirstRun(c *gc.C) {
	// make sure there is nothing
	err := jujucloud.WritePublicCloudMetadata(nil)
	c.Assert(err, jc.ErrorIsNil)

	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
	c.Assert(msg, gc.Matches, `
Fetching latest public cloud list...
Updated your list of public clouds with 1 cloud added:

    added cloud:
        - aws
`[1:])
}

func (s *updateCloudsSuite) TestNewData(c *gc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)

	newUpdateCloudData := sampleUpdateCloudData + `
      anotherregion:
        endpoint: http://anotherregion/1.0
`[1:]
	ts := s.setupTestServer(c, newUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	clouds, err = jujucloud.ParseCloudMetadata([]byte(newUpdateCloudData))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
	c.Assert(msg, gc.Matches, `
Fetching latest public cloud list...
Updated your list of public clouds with 1 cloud region added:

    added cloud region:
        - aws/anotherregion
`[1:])
}

func (s *updateCloudsSuite) setupCloudFileScenario(c *gc.C, apiFunc func(controllerName string) (cloud.UpdateCloudsAPI, error)) (*cloud.UpdateCloudsCommand, string) {
	cloudfile := prepareTestCloudYaml(c, garageMaasYamlFile)
	s.AddCleanup(func(_ *gc.C) {
		defer cloudfile.Close()
		defer os.Remove(cloudfile.Name())
	})
	mockCloud, err := jujucloud.ParseCloudMetadataFile(cloudfile.Name())
	c.Assert(err, jc.ErrorIsNil)
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudfile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)
	cmd := cloud.NewUpdateCloudsCommandForTest("", fake, s.store, apiFunc)

	return cmd, cloudfile.Name()
}

func (s *updateCloudsSuite) createLocalCacheFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("public-clouds.yaml"), []byte(garageMaasYamlFile), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCloudsSuite) TestUpdateLocalCacheFromFile(c *gc.C) {
	cmd, fileName := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudsAPI, error) {
		return nil, errors.New("")
	})
	_, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "-f", fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
}

func (s *updateCloudsSuite) TestUpdateControllerFromFile(c *gc.C) {
	var controllerNameCalled string
	cmd, fileName := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudsAPI, error) {
		controllerNameCalled = controllerName
		return s.api, nil
	})
	_, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "-f", fileName, "--controller", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(controllerNameCalled, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
}

func (s *updateCloudsSuite) TestUpdateControllerFromLocalCache(c *gc.C) {
	s.createLocalCacheFile(c)
	var controllerNameCalled string
	cmd, _ := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudsAPI, error) {
		controllerNameCalled = controllerName
		return s.api, nil
	})
	_, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "--controller", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(controllerNameCalled, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
}

type fakeUpdateCloudsAPI struct {
	jujutesting.Stub
	updatedClouds []jujucloud.Cloud
	raiseError    error
}

func (api *fakeUpdateCloudsAPI) Close() error {
	api.AddCall("Close", nil)
	return nil
}

func (api *fakeUpdateCloudsAPI) UpdateCloud(cloud jujucloud.Cloud) error {
	api.AddCall("UpdateCloud", cloud)
	if api.raiseError != nil {
		return api.raiseError
	}
	api.updatedClouds = append(api.updatedClouds, cloud)
	return nil
}
