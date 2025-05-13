// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type updatePublicCloudsSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	store *jujuclient.MemStore
	api   *fakeUpdatePublicCloudAPI
}

var _ = tc.Suite(&updatePublicCloudsSuite{})

func (s *updatePublicCloudsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.api = &fakeUpdatePublicCloudAPI{
		Stub:         testhelpers.Stub{},
		cloudsF:      func() (map[names.CloudTag]jujucloud.Cloud, error) { return nil, nil },
		updateCloudF: func(cloud jujucloud.Cloud) error { return nil },
	}

	s.store = jujuclient.NewMemStore()
	s.store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "mycontroller"
}

func encodeCloudYAML(c *tc.C, yaml string) string {
	// TODO(wallyworld) - move test signing key elsewhere
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(sstesting.SignedMetadataPrivateKey))
	c.Assert(err, tc.ErrorIsNil)
	privateKey := keyring[0].PrivateKey
	err = privateKey.Decrypt([]byte(sstesting.PrivateKeyPassphrase))
	c.Assert(err, tc.ErrorIsNil)

	var buf bytes.Buffer
	plaintext, err := clearsign.Encode(&buf, privateKey, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = plaintext.Write([]byte(yaml))
	c.Assert(err, tc.ErrorIsNil)
	err = plaintext.Close()
	c.Assert(err, tc.ErrorIsNil)
	return string(buf.Bytes())
}

func (s *updatePublicCloudsSuite) setupTestServer(c *tc.C, serverContent string) *httptest.Server {
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

func (s *updatePublicCloudsSuite) TestBadArgs(c *tc.C) {
	updateCmd := cloud.NewUpdatePublicCloudsCommandForTest(s.store, nil, "")
	_, err := cmdtesting.RunCommand(c, updateCmd, "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *updatePublicCloudsSuite) run(c *tc.C, url, errMsg string, args ...string) string {
	updateCmd := cloud.NewUpdatePublicCloudsCommandForTest(s.store, s.api, url)
	out, err := cmdtesting.RunCommand(c, updateCmd, args...)
	if errMsg == "" {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.NotNil)
		errString := strings.Replace(err.Error(), "\n", "", -1)
		c.Assert(errString, tc.Matches, errMsg)
	}
	return cmdtesting.Stderr(out)
}

func (s *updatePublicCloudsSuite) Test404(c *tc.C) {
	ts := s.setupTestServer(c, "404")
	defer ts.Close()
	_, err := cloud.PublishedPublicClouds(context.Background(), ts.URL, "")
	c.Assert(err, tc.ErrorMatches, "public cloud list is unavailable right now")
}

func (s *updatePublicCloudsSuite) Test401(c *tc.C) {
	ts := s.setupTestServer(c, "401")
	defer ts.Close()
	_, err := cloud.PublishedPublicClouds(context.Background(), ts.URL, "")
	c.Assert(err, tc.ErrorMatches, "unauthorised access to URL .*")
}

func (s *updatePublicCloudsSuite) TestUnsignedData(c *tc.C) {
	ts := s.setupTestServer(c, "unsigned")
	defer ts.Close()
	_, err := cloud.PublishedPublicClouds(context.Background(), ts.URL, "")
	c.Assert(err, tc.ErrorMatches, "receiving updated cloud data: no PGP signature embedded in plain text data")
}

func (s *updatePublicCloudsSuite) TestBadDataOnServer(c *tc.C) {
	ts := s.setupTestServer(c, "bad data")
	defer ts.Close()
	_, err := cloud.PublishedPublicClouds(context.Background(), ts.URL, sstesting.SignedMetadataPublicKey)
	c.Assert(err, tc.ErrorMatches, "(?s)invalid cloud data received when updating clouds.*")
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

func (s *updatePublicCloudsSuite) TestNoNewDataOnClient(c *tc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, tc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, tc.ErrorIsNil)

	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "", "--client")
	c.Assert(strings.Replace(msg, "\n", "", -1), tc.Matches, "Fetching latest public cloud list...List of public clouds on this client is up to date, see `juju clouds --client`.")
}

func (s *updatePublicCloudsSuite) TestFirstRunOnClient(c *tc.C) {
	// make sure there is nothing
	err := jujucloud.WritePublicCloudMetadata(nil)
	c.Assert(err, tc.ErrorIsNil)

	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "", "--client")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsFalse)
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(publicClouds, tc.DeepEquals, clouds)
	c.Assert(msg, tc.Matches, `
Fetching latest public cloud list...
Updated list of public clouds on this client, 1 cloud added:

    added cloud:
        - aws
`[1:])
}

func (s *updatePublicCloudsSuite) TestNewDataOnClient(c *tc.C) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(sampleUpdateCloudData))
	c.Assert(err, tc.ErrorIsNil)
	err = jujucloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, tc.ErrorIsNil)

	newUpdateCloudData := sampleUpdateCloudData + `
      anotherregion:
        endpoint: http://anotherregion/1.0
`[1:]
	ts := s.setupTestServer(c, newUpdateCloudData)
	defer ts.Close()

	msg := s.run(c, ts.URL, "", "--client")
	publicClouds, fallbackUsed, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsFalse)
	clouds, err = jujucloud.ParseCloudMetadata([]byte(newUpdateCloudData))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(publicClouds, tc.DeepEquals, clouds)
	c.Assert(msg, tc.Matches, `
Fetching latest public cloud list...
Updated list of public clouds on this client, 1 cloud region added:

    added cloud region:
        - aws/anotherregion
`[1:])
	s.api.CheckNoCalls(c)
}

func (s *updatePublicCloudsSuite) TestNoPublicCloudOnController(c *tc.C) {
	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	s.api.cloudsF = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("kloud"): {Name: "kloud"},
		}, nil
	}
	msg := s.run(c, ts.URL, "", "-c", "mycontroller")
	c.Assert(strings.Replace(msg, "\n", "", -1), tc.Matches, "Fetching latest public cloud list...List of public clouds on controller \"mycontroller\" is up to date, see `juju clouds --controller mycontroller`.")
	s.api.CheckCallNames(c, "Clouds", "Close")
}

func (s *updatePublicCloudsSuite) TestUpdatePublicCloudOnController(c *tc.C) {
	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	s.api.cloudsF = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("kloud"): {Name: "kloud"}, // want it here to make sure only public clouds on the controller are looked at.
			names.NewCloudTag("aws"):   {Name: "aws"},
		}, nil
	}
	msg := s.run(c, ts.URL, "", "-c", "mycontroller")
	c.Assert(msg, tc.Equals, `
Fetching latest public cloud list...
Updated list of public clouds on controller "mycontroller", 1 cloud region added as well as 1 cloud attribute changed:

    added cloud region:
        - aws/region
    changed cloud attribute:
        - aws
`[1:])
	s.api.CheckCallNames(c, "Clouds", "UpdateCloud", "Close")
	s.api.CheckCall(c, 1, "UpdateCloud", jujucloud.Cloud{
		Name:        "aws",
		Type:        "ec2",
		Description: "Amazon Web Services",
		AuthTypes:   jujucloud.AuthTypes{"access-key"},
		Endpoint:    "http://region",
		Regions: []jujucloud.Region{
			{Name: "region", Endpoint: "http://region/1.0"},
		},
	})
}

func (s *updatePublicCloudsSuite) TestSamePublicCloudOnController(c *tc.C) {
	ts := s.setupTestServer(c, sampleUpdateCloudData)
	defer ts.Close()

	s.api.cloudsF = func() (map[names.CloudTag]jujucloud.Cloud, error) {
		return map[names.CloudTag]jujucloud.Cloud{
			names.NewCloudTag("aws"): {
				Name:        "aws",
				Type:        "ec2",
				Description: "Amazon Web Services",
				AuthTypes:   jujucloud.AuthTypes{"access-key"},
				Endpoint:    "http://region",
				Regions: []jujucloud.Region{
					{Name: "region", Endpoint: "http://region/1.0"},
				},
			},
		}, nil
	}
	msg := s.run(c, ts.URL, "", "-c", "mycontroller")
	c.Assert(msg, tc.Equals, `
Fetching latest public cloud list...
List of public clouds on controller "mycontroller" is up to date, see `[1:]+"`juju clouds --controller mycontroller`"+`.
`)
	s.api.CheckCallNames(c, "Clouds", "Close")
}

type fakeUpdatePublicCloudAPI struct {
	testhelpers.Stub
	cloudsF      func() (map[names.CloudTag]jujucloud.Cloud, error)
	updateCloudF func(cloud jujucloud.Cloud) error
}

func (f *fakeUpdatePublicCloudAPI) Close() error {
	f.AddCall("Close", nil)
	return nil
}

func (f *fakeUpdatePublicCloudAPI) UpdateCloud(ctx context.Context, cloud jujucloud.Cloud) error {
	f.AddCall("UpdateCloud", cloud)
	return f.updateCloudF(cloud)
}

func (f *fakeUpdatePublicCloudAPI) Clouds(ctx context.Context) (map[names.CloudTag]jujucloud.Cloud, error) {
	f.AddCall("Clouds")
	return f.cloudsF()
}
