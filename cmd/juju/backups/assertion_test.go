// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/backups"
)

type assertionSuite struct{}

func TestAssertionSuite(t *testing.T) {
	tc.Run(t, &assertionSuite{})
}

func (s *assertionSuite) readAssertion(c *tc.C, content string) (*backups.Assertion, error) {
	return backups.ReadAssertion(strings.NewReader(content))
}

func (s *assertionSuite) validChecksum() string {
	return strings.Repeat("a", 64)
}

// TestReadAssertionChecksumSpellings verifies that the recorded checksum
// may be bare or "sha256-" prefixed, in either hex case, and that the
// recorded spelling is preserved.
func (s *assertionSuite) TestReadAssertionChecksumSpellings(c *tc.C) {
	upper := strings.ToUpper(s.validChecksum())
	for _, checksum := range []string{
		s.validChecksum(),
		"sha256-" + s.validChecksum(),
		upper,
		"sha256-" + upper,
	} {
		c.Logf("checksum %q", checksum)
		assertion, err := s.readAssertion(c, "checksum: "+checksum+"\nchecksum-format: SHA-256\n")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(assertion.Checksum, tc.Equals, checksum)
		c.Check(assertion.ChecksumFormat, tc.Equals, "SHA-256")
	}
}

// TestReadAssertionDescriptiveMetadata verifies that the remaining
// summary fields round-trip without validation.
func (s *assertionSuite) TestReadAssertionDescriptiveMetadata(c *tc.C) {
	content := (`
checksum: sha256-` + s.validChecksum() + `
checksum-format: SHA-256
format-version: 2
size: 12345
notes: a note
controller-uuid: ctrl-uuid
ha-nodes: 3
model-uuid: model-uuid
machine-id: "0"
hostname: myhost
juju-version: 4.1.2.1
base: ubuntu@24.04
`)[1:]
	assertion, err := s.readAssertion(c, content)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(assertion.FormatVersion, tc.Equals, int64(2))
	c.Check(assertion.Size, tc.Equals, int64(12345))
	c.Check(assertion.Notes, tc.Equals, "a note")
	c.Check(assertion.ControllerUUID, tc.Equals, "ctrl-uuid")
	c.Check(assertion.HANodes, tc.Equals, int64(3))
	c.Check(assertion.ModelUUID, tc.Equals, "model-uuid")
	c.Check(assertion.MachineID, tc.Equals, "0")
	c.Check(assertion.Hostname, tc.Equals, "myhost")
	c.Check(assertion.JujuVersion.String(), tc.Equals, "4.1.2.1")
	c.Check(assertion.Base, tc.Equals, "ubuntu@24.04")
}

func (s *assertionSuite) TestReadAssertionEmpty(c *tc.C) {
	for _, content := range []string{"", "\n", "   \n"} {
		_, err := s.readAssertion(c, content)
		c.Check(err, tc.ErrorMatches, "backup assertion is empty")
	}
}

// TestReadAssertionNullDocument verifies that an empty YAML document is
// rejected rather than silently producing zero metadata.
func (s *assertionSuite) TestReadAssertionNullDocument(c *tc.C) {
	for _, content := range []string{"null\n", "~\n", "---\n"} {
		_, err := s.readAssertion(c, content)
		c.Check(err, tc.ErrorMatches, "backup assertion is empty")
	}
}

func (s *assertionSuite) TestReadAssertionEmptyMapping(c *tc.C) {
	_, err := s.readAssertion(c, "{}\n")
	c.Check(err, tc.ErrorMatches, `unsupported backup assertion checksum format ""`)
}

func (s *assertionSuite) TestReadAssertionMalformed(c *tc.C) {
	_, err := s.readAssertion(c, "checksum: [unclosed\n")
	c.Check(err, tc.ErrorMatches, "cannot parse backup assertion: .*")
}

func (s *assertionSuite) TestReadAssertionUnknownField(c *tc.C) {
	content := (`
checksum: sha256-` + s.validChecksum() + `
checksum-format: SHA-256
signature: nobody-signed-this
`)[1:]
	_, err := s.readAssertion(c, content)
	c.Check(err, tc.ErrorMatches, "(?s)cannot parse backup assertion: .*")
}

func (s *assertionSuite) TestReadAssertionDuplicateKey(c *tc.C) {
	content := (`
checksum: sha256-` + s.validChecksum() + `
checksum: sha256-` + strings.Repeat("b", 64) + `
checksum-format: SHA-256
`)[1:]
	_, err := s.readAssertion(c, content)
	c.Check(err, tc.ErrorMatches, "(?s)cannot parse backup assertion: .*")
}

func (s *assertionSuite) TestReadAssertionMultipleDocuments(c *tc.C) {
	document := "checksum: sha256-" + s.validChecksum() + "\nchecksum-format: SHA-256\n"
	_, err := s.readAssertion(c, document+"---\n"+document)
	c.Check(err, tc.ErrorMatches, "backup assertion must contain exactly one YAML document")
}

func (s *assertionSuite) TestReadAssertionUnsupportedChecksumFormat(c *tc.C) {
	_, err := s.readAssertion(c, "checksum: "+s.validChecksum()+"\nchecksum-format: SHA-1\n")
	c.Check(err, tc.ErrorMatches, `unsupported backup assertion checksum format "SHA-1"`)
}

// TestReadAssertionInvalidChecksum verifies that a checksum that is not
// 64 hexadecimal characters is refused, including an empty one: an
// empty expected digest would make recovery skip the archive
// comparison altogether.
func (s *assertionSuite) TestReadAssertionInvalidChecksum(c *tc.C) {
	for _, checksum := range []string{
		"",
		"deadbeef",
		"sha256-deadbeef",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		"sha256-" + strings.Repeat("g", 64),
	} {
		_, err := s.readAssertion(c, "checksum: "+checksum+"\nchecksum-format: SHA-256\n")
		c.Check(err, tc.ErrorMatches,
			"backup assertion checksum must be a SHA-256 checksum: 64 hexadecimal characters")
	}
}
