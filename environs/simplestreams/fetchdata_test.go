// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/keys"
)

type fetchDataSuite struct {
	requireSigned bool
	source        *testing.StubDataSource

	readerData, expectedData string
	expectedCalls            []string
}

var _ = tc.Suite(&fetchDataSuite{})

func (s *fetchDataSuite) SetUpTest(c *tc.C) {
	s.source = testing.NewStubDataSource()
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithoutPublicKey(c *tc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource("")
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: failed to parse public key: openpgp: invalid argument: no armored data found`)
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithWrongPublicKey(c *tc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource(keys.JujuPublicKey)
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: openpgp: signature made by unknown entity`)
}

func (s *fetchDataSuite) TestFetchSignedDataWithRequireSignedDataSourceWithPublicKey(c *tc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey"}
	s.readerData = signedData
	s.expectedData = unsignedData[1:]
	s.setupDataSource(testSigningKey)
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchSignedDataWithNotRequireSignedDataSourceWithPublicKey(c *tc.C) {
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

func (s *fetchDataSuite) TestFetchSignedDataWithNotRequireSignedDataSourceWithoutPublicKey(c *tc.C) {
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

func (s *fetchDataSuite) TestFetchUnsignedDataWithRequireSignedDataSourceWithoutPublicKey(c *tc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource("")
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: no PGP signature embedded in plain text data`)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithRequireSignedDataSourceWithPublicKey(c *tc.C) {
	s.requireSigned = true
	s.expectedCalls = []string{"Fetch", "PublicSigningKey", "Description"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource(testSigningKey)
	s.assertFetchDataFail(c, `cannot read data for source "" at URL this.path.doesnt.matter.for.test.either: no PGP signature embedded in plain text data`)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithNotRequireSignedDataSourceWithPublicKey(c *tc.C) {
	s.requireSigned = false
	s.expectedCalls = []string{"Fetch"}
	s.expectedData = unsignedData
	s.readerData = unsignedData
	s.setupDataSource(testSigningKey)
	s.assertFetchData(c)
}

func (s *fetchDataSuite) TestFetchUnsignedDataWithNotRequireSignedDataSourceWithoutPublicKey(c *tc.C) {
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
		return io.NopCloser(r), path, nil
	}
	s.source.PublicSigningKeyFunc = func() string {
		return key
	}
}

func (s *fetchDataSuite) assertFetchData(c *tc.C) {
	data, _, err := simplestreams.FetchData(context.Background(), s.source, "this.path.doesnt.matter.for.test.either", s.requireSigned)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert([]byte(s.expectedData), tc.DeepEquals, data)
	s.source.CheckCallNames(c, s.expectedCalls...)
}

func (s *fetchDataSuite) assertFetchDataFail(c *tc.C, msg string) {
	data, _, err := simplestreams.FetchData(context.Background(), s.source, "this.path.doesnt.matter.for.test.either", s.requireSigned)
	c.Assert(err, tc.ErrorMatches, msg)
	c.Assert(data, tc.IsNil)
	s.source.CheckCallNames(c, s.expectedCalls...)
}
