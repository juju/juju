// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/provider/vsphere/internal/ovatest"
	coretesting "github.com/juju/juju/testing"
)

type ProviderFixture struct {
	testing.IsolationSuite
	dialStub testing.Stub
	client   *mockClient
	provider environs.CloudEnvironProvider
	callCtx  context.ProviderCallContext
}

func (s *ProviderFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dialStub.ResetCalls()
	s.client = &mockClient{}
	s.provider = vsphere.NewEnvironProvider(vsphere.EnvironProviderConfig{
		Dial: newMockDialFunc(&s.dialStub, s.client),
	})
	s.callCtx = context.NewCloudCallContext()
}

type EnvironFixture struct {
	ProviderFixture
	imageServer         *httptest.Server
	imageServerRequests []*http.Request
	env                 environs.Environ
	callCtx             context.ProviderCallContext
}

func (s *EnvironFixture) SetUpTest(c *gc.C) {
	s.ProviderFixture.SetUpTest(c)

	s.imageServerRequests = nil
	s.imageServer = serveImageMetadata(&s.imageServerRequests)
	s.AddCleanup(func(*gc.C) {
		s.imageServer.Close()
	})

	env, err := s.provider.Open(environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"image-metadata-url": s.imageServer.URL,
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.env = env

	// Make sure we don't fall back to the public image sources.
	s.PatchValue(&imagemetadata.DefaultUbuntuBaseURL, "")
	s.callCtx = context.NewCloudCallContext()
}

func serveImageMetadata(requests *[]*http.Request) *httptest.Server {
	index := `{
  "index": {
     "com.ubuntu.cloud:released:download": {
      "datatype": "image-downloads", 
      "path": "streams/v1/com.ubuntu.cloud:released:download.json", 
      "updated": "Tue, 24 Feb 2015 10:16:54 +0000", 
      "products": ["com.ubuntu.cloud:server:14.04:amd64"], 
      "format": "products:1.0"
    }
  }, 
  "updated": "Tue, 24 Feb 2015 14:14:24 +0000", 
  "format": "index:1.0"
}`

	download := fmt.Sprintf(`{
  "updated": "Thu, 05 Mar 2015 12:14:40 +0000", 
  "license": "http://www.canonical.com/intellectual-property-policy", 
  "format": "products:1.0", 
  "datatype": "image-downloads", 
  "products": {
    "com.ubuntu.cloud:server:14.04:amd64": {
      "release": "trusty", 
      "version": "14.04", 
      "arch": "amd64", 
      "versions": {
        "20150305": {
          "items": {
            "ova": {
              "size": 7196, 
              "path": "server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ova",
              "ftype": "ova", 
              "sha256": "%s", 
              "md5": "00662c59ca52558e7a3bb9a67d194730"
            }
          }      
        }
      }
    }
  }
}`, ovatest.FakeOVASHA256())

	files := map[string][]byte{
		"/streams/v1/index.json":                                                          []byte(index),
		"/streams/v1/com.ubuntu.cloud:released:download.json":                             []byte(download),
		"/server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ova": ovatest.FakeOVAContents(),
	}
	mux := http.NewServeMux()
	for path := range files {
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			*requests = append(*requests, req)
			w.Write(files[req.URL.Path])
		})
	}
	return httptest.NewServer(mux)
}

func AssertInvalidatesCredential(c *gc.C, client *mockClient, f func(context.ProviderCallContext) error) {
	client.SetErrors(soap.WrapSoapFault(&soap.Fault{
		Code:   "ServerFaultCode",
		String: "No way José",
		Detail: struct {
			Fault types.AnyType `xml:",any,typeattr"`
		}{Fault: types.NoPermission{}},
	}), errors.New("find folder failed"))
	var called bool
	ctx := &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			called = true
			return nil
		},
	}
	err := f(ctx)
	c.Assert(err, gc.ErrorMatches, ".*ServerFaultCode: No way José$")
	c.Assert(called, gc.Equals, true)
}
