// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagedownloads_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/tc"
	"golang.org/x/crypto/openpgp"
	openpgperrors "golang.org/x/crypto/openpgp/errors"

	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	streamstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type Suite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&Suite{})

func newTestDataSource(factory simplestreams.DataSourceFactory, s string) simplestreams.DataSource {
	return imagedownloads.NewDataSource(factory, s+"/"+imagemetadata.ReleasedImagesPath)
}

func newTestDataSourceFunc(s string) func() simplestreams.DataSource {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	return func() simplestreams.DataSource {
		return imagedownloads.NewDataSource(ss, s+"/releases/")
	}
}

func (s *Suite) SetUpTest(c *tc.C) {
	imagemetadata.SimplestreamsImagesPublicKey = streamstesting.SignedMetadataPublicKey

	// The index.sjson file used by these tests have been regenerated using
	// the test keys in environs/simplestreams/testing/testing.go. As this
	// signature is not trusted, we need to override the signature check
	// implementation and suppress the ErrUnkownIssuer error.
	s.PatchValue(&simplestreams.PGPSignatureCheckFn, func(keyring openpgp.KeyRing, signed, signature io.Reader) (*openpgp.Entity, error) {
		ent, err := openpgp.CheckDetachedSignature(keyring, signed, signature)
		c.Assert(err, tc.Equals, openpgperrors.ErrUnknownIssuer, tc.Commentf("expected the signature verification to return ErrUnknownIssuer when the index file is signed with the test pgp key"))
		return ent, nil
	})
}

func (*Suite) TestNewSignedImagesSource(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	got := imagedownloads.DefaultSource(ss)()
	c.Check(got.Description(), tc.DeepEquals, "ubuntu cloud images")
	c.Check(got.PublicSigningKey(), tc.DeepEquals, imagemetadata.SimplestreamsImagesPublicKey)
	c.Check(got.RequireSigned(), tc.IsTrue)
	gotURL, err := got.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURL, tc.DeepEquals, "http://cloud-images.ubuntu.com/releases/")
}

func (*Suite) TestFetchManyDefaultFilter(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints, err := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"16.04"},
			Stream:   "released",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	got, resolveInfo, err := imagedownloads.Fetch(c.Context(), ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, tc.IsTrue)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(got), tc.DeepEquals, 27)
	for _, v := range got {
		gotURL, err := v.DownloadURL(ts.URL)
		c.Check(err, tc.ErrorIsNil)
		c.Check(strings.HasSuffix(gotURL.String(), v.FType), tc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Release), tc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Version), tc.IsTrue)
	}
}

func (*Suite) TestFetchManyDefaultFilterAndCustomImageDownloadURL(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints, err := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"16.04"},
			Stream:   "released",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	got, resolveInfo, err := imagedownloads.Fetch(c.Context(), ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, tc.IsTrue)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(got), tc.DeepEquals, 27)
	for _, v := range got {
		// Note: instead of the index URL, we are pulling the actual
		// images from a different operator-provided URL.
		gotURL, err := v.DownloadURL("https://tasty-cloud-images.ubuntu.com")
		c.Check(err, tc.ErrorIsNil)
		c.Check(strings.HasPrefix(gotURL.String(), "https://tasty-cloud-images.ubuntu.com"), tc.IsTrue, tc.Commentf("expected image download URL to use the operator-provided URL"))
		c.Check(strings.HasSuffix(gotURL.String(), v.FType), tc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Release), tc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Version), tc.IsTrue)
	}
}

func (*Suite) TestFetchSingleDefaultFilter(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		LookupParams: simplestreams.LookupParams{
			Arches:   []string{"ppc64el"},
			Releases: []string{"16.04"},
		}}
	got, resolveInfo, err := imagedownloads.Fetch(c.Context(), ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, tc.IsTrue)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(got), tc.DeepEquals, 8)
	c.Check(got[0].Arch, tc.DeepEquals, "ppc64el")
	c.Check(err, tc.ErrorIsNil)
	for _, v := range got {
		_, err := v.DownloadURL(ts.URL)
		c.Check(err, tc.ErrorIsNil)
	}
}

func (*Suite) TestFetchOneWithFilter(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		LookupParams: simplestreams.LookupParams{
			Arches:   []string{"ppc64el"},
			Releases: []string{"16.04"},
		}}
	got, resolveInfo, err := imagedownloads.Fetch(c.Context(), ss, tds, constraints, imagedownloads.Filter("disk1.img"))
	c.Check(resolveInfo.Signed, tc.IsTrue)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(got), tc.DeepEquals, 1)
	c.Check(got[0].Arch, tc.DeepEquals, "ppc64el")
	// Assuming that the operator has not overridden the image download URL
	// parameter we pass the default empty value which should fall back to
	// the default cloud-images.ubuntu.com URL.
	gotURL, err := got[0].DownloadURL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(
		gotURL.String(),
		tc.DeepEquals,
		"http://cloud-images.ubuntu.com/server/releases/xenial/release-20211001/ubuntu-16.04-server-cloudimg-ppc64el-disk1.img")
}

func (*Suite) TestFetchManyWithFilter(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		LookupParams: simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"16.04"},
		}}
	got, resolveInfo, err := imagedownloads.Fetch(c.Context(), ss, tds, constraints, imagedownloads.Filter("disk1.img"))
	c.Check(resolveInfo.Signed, tc.IsTrue)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(got), tc.DeepEquals, 3)
	c.Check(got[0].Arch, tc.DeepEquals, "amd64")
	c.Check(got[1].Arch, tc.DeepEquals, "arm64")
	c.Check(got[2].Arch, tc.DeepEquals, "ppc64el")
	for i, arch := range []string{"amd64", "arm64", "ppc64el"} {
		wantURL := fmt.Sprintf("http://cloud-images.ubuntu.com/server/releases/xenial/release-20211001/ubuntu-16.04-server-cloudimg-%s-disk1.img", arch)
		// Assuming that the operator has not overridden the image
		// download URL parameter we pass the default empty value which
		// should fall back to the default cloud-images.ubuntu.com URL.
		gotURL, err := got[i].DownloadURL("")
		c.Check(err, tc.ErrorIsNil)
		c.Check(gotURL.String(), tc.DeepEquals, wantURL)
	}
}

func (*Suite) TestOneAmd64XenialTarGz(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := imagedownloads.One(c.Context(), ss, "amd64", "22.04", "", "tar.gz", newTestDataSourceFunc(ts.URL))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &imagedownloads.Metadata{
		Arch:    "amd64",
		Release: "jammy",
		Version: "22.04",
		FType:   "tar.gz",
		SHA256:  "4e466ce60488c520e34c5f3e4aa57b88528b9500b2f48bf40773192c9260ed93",
		Path:    "server/releases/jammy/release-20220923/ubuntu-22.04-server-cloudimg-amd64.tar.gz",
		Size:    610831936,
	})
}

func (*Suite) TestOneArm64JammyImg(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := imagedownloads.One(c.Context(), ss, "arm64", "22.04", "released", "disk1.img", newTestDataSourceFunc(ts.URL))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &imagedownloads.Metadata{
		Arch:    "arm64",
		Release: "jammy",
		Version: "22.04",
		FType:   "disk1.img",
		SHA256:  "78b5ca0da456b228e2441bdca0cca1eab30b1b6a3eaf9594eabcb2cfc21275f3",
		Path:    "server/releases/jammy/release-20220923/ubuntu-22.04-server-cloudimg-arm64.img",
		Size:    642646016,
	})
}

func (*Suite) TestOneArm64FocalImg(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := imagedownloads.One(c.Context(), ss, "arm64", "20.04", "released", "disk1.img", newTestDataSourceFunc(ts.URL))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &imagedownloads.Metadata{
		Arch:    "arm64",
		Release: "focal",
		Version: "20.04",
		FType:   "disk1.img",
		SHA256:  "b8176161962c4f54e59366444bb696e92406823f643ed7bdcdd3d15d38dc0d53",
		Path:    "server/releases/focal/release-20221003/ubuntu-20.04-server-cloudimg-arm64.img",
		Size:    569901056,
	})
}

func (*Suite) TestOneErrors(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	table := []struct {
		description, arch, version, stream, ftype, errorMatch string
	}{
		{"empty arch", "", "20.04", "", "disk1.img", `invalid parameters supplied arch=""`},
		{"invalid arch", "<F7>", "20.04", "", "disk1.img", `invalid parameters supplied arch="<F7>"`},
		{"empty series", "arm64", "", "released", "disk1.img", `invalid parameters supplied version=""`},
		{"invalid series", "amd64", "rusty", "", "disk1.img", `invalid parameters supplied version="rusty"`},
		{"empty ftype", "ppc64el", "20.04", "", "", `invalid parameters supplied ftype=""`},
		{"invalid file type", "amd64", "22.04", "", "tragedy", `invalid parameters supplied ftype="tragedy"`},
		{"all wrong except stream", "a", "t", "", "y", `invalid parameters supplied arch="a" version="t" ftype="y"`},
		{"stream not found", "amd64", "22.04", "hourly", "disk1.img", `no results for "amd64", "22.04", "hourly", "disk1.img"`},
	}
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	for i, test := range table {
		c.Logf("test % 1d: %s\n", i+1, test.description)
		_, err := imagedownloads.One(c.Context(), ss, test.arch, test.version, test.stream, test.ftype, newTestDataSourceFunc(ts.URL))
		c.Check(err, tc.ErrorMatches, test.errorMatch)
	}
}

type sstreamsHandler struct{}

func (h sstreamsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/releases/streams/v1/index.sjson":
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "testdata/index.sjson")
		return
	case "/releases/streams/v1/com.ubuntu.cloud:released:download.sjson":
		w.Header().Set("Content-Type", "application/json")
		http.ServeFile(w, r, "testdata/com.ubuntu.cloud-released-download.sjson")
		return
	default:
		http.Error(w, r.URL.Path, 404)
		return
	}
}
