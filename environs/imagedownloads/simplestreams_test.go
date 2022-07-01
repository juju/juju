// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagedownloads_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/openpgp"
	openpgperrors "golang.org/x/crypto/openpgp/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/core/series"
	. "github.com/juju/juju/v2/environs/imagedownloads"
	"github.com/juju/juju/v2/environs/imagemetadata"
	"github.com/juju/juju/v2/environs/simplestreams"
	streamstesting "github.com/juju/juju/v2/environs/simplestreams/testing"
)

type Suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&Suite{})

func newTestDataSource(factory simplestreams.DataSourceFactory, s string) simplestreams.DataSource {
	return NewDataSource(factory, s+"/"+imagemetadata.ReleasedImagesPath)
}

func newTestDataSourceFunc(s string) func() simplestreams.DataSource {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	return func() simplestreams.DataSource {
		return NewDataSource(ss, s+"/releases/")
	}
}

func (s *Suite) SetUpTest(c *gc.C) {
	s.PatchValue(&series.UbuntuDistroInfo, "/path/notexists")
	imagemetadata.SimplestreamsImagesPublicKey = streamstesting.SignedMetadataPublicKey

	// The index.sjson file used by these tests have been regenerated using
	// the test keys in environs/simplestreams/testing/testing.go. As this
	// signature is not trusted, we need to override the signature check
	// implementation and suppress the ErrUnkownIssuer error.
	s.PatchValue(&simplestreams.PGPSignatureCheckFn, func(keyring openpgp.KeyRing, signed, signature io.Reader) (*openpgp.Entity, error) {
		ent, err := openpgp.CheckDetachedSignature(keyring, signed, signature)
		c.Assert(err, gc.Equals, openpgperrors.ErrUnknownIssuer, gc.Commentf("expected the signature verification to return ErrUnknownIssuer when the index file is signed with the test pgp key"))
		return ent, nil
	})
}

func (Suite) TestNewSignedImagesSource(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	got := DefaultSource(ss)()
	c.Check(got.Description(), jc.DeepEquals, "ubuntu cloud images")
	c.Check(got.PublicSigningKey(), jc.DeepEquals, imagemetadata.SimplestreamsImagesPublicKey)
	c.Check(got.RequireSigned(), jc.IsTrue)
	gotURL, err := got.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL, jc.DeepEquals, "http://cloud-images.ubuntu.com/releases/")
}

func (Suite) TestFetchManyDefaultFilter(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints, err := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"xenial"},
			Stream:   "released",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	got, resolveInfo, err := Fetch(ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(got), jc.DeepEquals, 27)
	for _, v := range got {
		gotURL, err := v.DownloadURL(ts.URL)
		c.Check(err, jc.ErrorIsNil)
		c.Check(strings.HasSuffix(gotURL.String(), v.FType), jc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Release), jc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Version), jc.IsTrue)
	}
}

func (Suite) TestFetchManyDefaultFilterAndCustomImageDownloadURL(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints, err := imagemetadata.NewImageConstraint(
		simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"xenial"},
			Stream:   "released",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	got, resolveInfo, err := Fetch(ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(got), jc.DeepEquals, 27)
	for _, v := range got {
		// Note: instead of the index URL, we are pulling the actual
		// images from a different operator-provided URL.
		gotURL, err := v.DownloadURL("https://tasty-cloud-images.ubuntu.com")
		c.Check(err, jc.ErrorIsNil)
		c.Check(strings.HasPrefix(gotURL.String(), "https://tasty-cloud-images.ubuntu.com"), jc.IsTrue, gc.Commentf("expected image download URL to use the operator-provided URL"))
		c.Check(strings.HasSuffix(gotURL.String(), v.FType), jc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Release), jc.IsTrue)
		c.Check(strings.Contains(gotURL.String(), v.Version), jc.IsTrue)
	}
}

func (Suite) TestFetchSingleDefaultFilter(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Arches:   []string{"ppc64el"},
			Releases: []string{"trusty"},
		}}
	got, resolveInfo, err := Fetch(ss, tds, constraints, nil)
	c.Check(resolveInfo.Signed, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(got), jc.DeepEquals, 6)
	c.Check(got[0].Arch, jc.DeepEquals, "ppc64el")
	c.Check(err, jc.ErrorIsNil)
	for _, v := range got {
		gotURL, err := v.DownloadURL(ts.URL)
		c.Check(err, jc.ErrorIsNil)
		c.Check(strings.HasSuffix(gotURL.String(), v.FType), jc.IsTrue)
	}
}

func (Suite) TestFetchOneWithFilter(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Arches:   []string{"ppc64el"},
			Releases: []string{"xenial"},
		}}
	got, resolveInfo, err := Fetch(ss, tds, constraints, Filter("disk1.img"))
	c.Check(resolveInfo.Signed, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(got), jc.DeepEquals, 1)
	c.Check(got[0].Arch, jc.DeepEquals, "ppc64el")
	// Assuming that the operator has not overridden the image download URL
	// parameter we pass the default empty value which should fall back to
	// the default cloud-images.ubuntu.com URL.
	gotURL, err := got[0].DownloadURL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		gotURL.String(),
		jc.DeepEquals,
		"http://cloud-images.ubuntu.com/server/releases/xenial/release-20161020/ubuntu-16.04-server-cloudimg-ppc64el-disk1.img")
}

func (Suite) TestFetchManyWithFilter(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	tds := []simplestreams.DataSource{
		newTestDataSource(ss, ts.URL)}
	constraints := &imagemetadata.ImageConstraint{
		simplestreams.LookupParams{
			Arches:   []string{"amd64", "arm64", "ppc64el"},
			Releases: []string{"xenial"},
		}}
	got, resolveInfo, err := Fetch(ss, tds, constraints, Filter("disk1.img"))
	c.Check(resolveInfo.Signed, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(got), jc.DeepEquals, 3)
	c.Check(got[0].Arch, jc.DeepEquals, "amd64")
	c.Check(got[1].Arch, jc.DeepEquals, "arm64")
	c.Check(got[2].Arch, jc.DeepEquals, "ppc64el")
	for i, arch := range []string{"amd64", "arm64", "ppc64el"} {
		wantURL := fmt.Sprintf("http://cloud-images.ubuntu.com/server/releases/xenial/release-20161020/ubuntu-16.04-server-cloudimg-%s-disk1.img", arch)
		// Assuming that the operator has not overridden the image
		// download URL parameter we pass the default empty value which
		// should fall back to the default cloud-images.ubuntu.com URL.
		gotURL, err := got[i].DownloadURL("")
		c.Check(err, jc.ErrorIsNil)
		c.Check(gotURL.String(), jc.DeepEquals, wantURL)

	}
}

func (Suite) TestOneAmd64XenialTarGz(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := One(ss, "amd64", "xenial", "", "tar.gz", newTestDataSourceFunc(ts.URL))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &Metadata{
		Arch:    "amd64",
		Release: "xenial",
		Version: "16.04",
		FType:   "tar.gz",
		SHA256:  "9c00e4340c636b684e93c77dd7f08ab70989faffccf1a66fb00612ca6f64d8e4",
		Path:    "server/releases/xenial/release-20161020/ubuntu-16.04-server-cloudimg-amd64.tar.gz",
		Size:    287271698,
	})
}

func (Suite) TestOneArm64TrustyImg(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := One(ss, "arm64", "trusty", "released", "disk1.img", newTestDataSourceFunc(ts.URL))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &Metadata{
		Arch:    "arm64",
		Release: "trusty",
		Version: "14.04",
		FType:   "disk1.img",
		SHA256:  "7da1bec20dfd78c7175155190c6fcdc16a6064be2a8bdabf13369f041c06aaf2",
		Path:    "server/releases/trusty/release-20161020/ubuntu-14.04-server-cloudimg-arm64-disk1.img",
		Size:    356123136,
	})
}

func (Suite) TestOnePpc64elXenialImg(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	got, err := One(ss, "ppc64el", "xenial", "", "disk1.img", newTestDataSourceFunc(ts.URL))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, &Metadata{
		Arch:    "ppc64el",
		Release: "xenial",
		Version: "16.04",
		FType:   "disk1.img",
		SHA256:  "787b16d29faaa1cf2c90b7beb1a98b1e1b990fb6a5beae47c6d6f09120ed49e8",
		Path:    "server/releases/xenial/release-20161020/ubuntu-16.04-server-cloudimg-ppc64el-disk1.img",
		Size:    320012288,
	})
}

func (Suite) TestOneErrors(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(streamstesting.TestDataSourceFactory())
	table := []struct {
		description, arch, series, stream, ftype, errorMatch string
	}{
		{"empty arch", "", "xenial", "", "disk1.img", `invalid parameters supplied arch=""`},
		{"invalid arch", "<F7>", "xenial", "", "disk1.img", `invalid parameters supplied arch="<F7>"`},
		{"empty series", "arm64", "", "released", "disk1.img", `invalid parameters supplied series=""`},
		{"invalid series", "amd64", "rusty", "", "disk1.img", `invalid parameters supplied series="rusty"`},
		{"empty ftype", "ppc64el", "xenial", "", "", `invalid parameters supplied ftype=""`},
		{"invalid file type", "amd64", "trusty", "", "tragedy", `invalid parameters supplied ftype="tragedy"`},
		{"all wrong except stream", "a", "t", "", "y", `invalid parameters supplied arch="a" series="t" ftype="y"`},
		{"stream not found", "amd64", "xenial", "hourly", "disk1.img", `no results for "amd64", "xenial", "hourly", "disk1.img"`},
	}
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	for i, test := range table {
		c.Logf("test % 1d: %s\n", i+1, test.description)
		_, err := One(ss, test.arch, test.series, test.stream, test.ftype, newTestDataSourceFunc(ts.URL))
		c.Check(err, gc.ErrorMatches, test.errorMatch)
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
