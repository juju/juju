// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"bytes"
	"io"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/simplestreams/testing"
)

type fetchDataSuite struct {
	requireSigned bool
	source        *testing.StubDataSource

	readerData, expectedData string
	expectedCalls            []string
}

var _ = gc.Suite(&fetchDataSuite{})

func (s *fetchDataSuite) SetUpTest(c *gc.C) {
	s.source = testing.NewStubDataSource()
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithoutPublicKey(c *gc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource("")
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: failed to parse public key: openpgp: invalid argument: no armored data found`)
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithWrongPublicKey(c *gc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource(simplestreams.SimplestreamsJujuPublicKey)
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: openpgp: signature made by unknown entity`)
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithPublicKey(c *gc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource(testSigningKey)
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchSignedDataWithNotRequireSignedDataSourceWithPublicKey(c *gc.C) {
	s.requireSigned = false
	s.expectedCalls = []string{"Fetch"}
	s.readerData = signedData
	// Current implementation will return the full signed data
	// without stripping signature prefix and suffix.
	// In order to return strip signing information, we need to be able to detect if file
	// contents are signed and act accordingly. We do not do this now, we hard-code "requireSigned".
	s.expectedData = signedData
	s.setupDataSource(testSigningKey)
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchSignedDataWithNotRequireSignedDataSourceWithoutPublicKey(c *gc.C) {
	s.requireSigned = false
	s.expectedCalls = []string{"Fetch"}
	s.readerData = signedData
	// Current implementation will return the full signed data
	// without stripping signature prefix and suffix.
	// In order to return strip signing information, we need to be able to detect if file
	// contents are signed and act accordingly. We do not do this now, we hard-code "requireSigned".
	s.expectedData = signedData
	s.setupDataSource("")
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithRequireSignedDataSourceWithoutPublicKey(c *gc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource("")
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: no PGP signature embedded in plain text data`)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithRequireSignedDataSourceWithPublicKey(c *gc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource(testSigningKey)
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: no PGP signature embedded in plain text data`)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithNotRequireSignedDataSourceWithPublicKey(c *gc.C) {
	s.requireSigned = false
	s.expectedCalls = []string{"Fetch"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource(testSigningKey)
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithNotRequireSignedDataSourceWithoutPublicKey(c *gc.C) {
	s.requireSigned = false
	s.expectedCalls = []string{"Fetch"}
	s.readerData = unsignedData
	s.expectedData = unsignedData
	s.setupDataSource("")
	s.assertFetchData(c)
}

func (s *fetchDataSuite) setupDataSource(key string) {
	s.source = testing.NewStubDataSource()
	s.source.FetchFunc = func(path string) (io.ReadCloser, string, error) {
		r := bytes.NewReader([]byte(s.readerData))
		return ioutil.NopCloser(r), path, nil
	}
	s.source.PublicSigningKeyFunc = func() string {
		return key
	}
}

func (s *fetchDataSuite) assertFetchData(c *gc.C) {
	data, _, err := simplestreams.FetchData(s.source, "this.path.doesnt.matter.for.test.either", s.requireSigned)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert([]byte(s.expectedData), gc.DeepEquals, data)
	s.source.CheckCallNames(c, s.expectedCalls...)
}

func (s *fetchDataSuite) assertFetchDataFail(c *gc.C, msg string) {
	data, _, err := simplestreams.FetchData(s.source, "this.path.doesnt.matter.for.test.either", s.requireSigned)
	c.Assert(err, gc.ErrorMatches, msg)
	c.Assert(data, gc.IsNil)
	s.source.CheckCallNames(c, s.expectedCalls...)
}
