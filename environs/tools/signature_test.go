// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/testing"
)

type signatureSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&signatureSuite{})

func getVersions(c *gc.C) *tools.SignedVersions {
	r := bytes.NewReader([]byte(signedData))
	versions, err := tools.ParseSignedVersions(r, testSigningKey)
	c.Assert(err, jc.ErrorIsNil)
	return versions
}

func (s *signatureSuite) TestParseSignedVersions(c *gc.C) {
	c.Assert(getVersions(c), gc.DeepEquals, &tools.SignedVersions{
		Versions: []tools.VersionHash{{
			Version: "2.2.4-xenial-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-trusty-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-centos7-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-xenial-arm64",
			SHA256:  "f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47",
		}},
	})
}

func (s *signatureSuite) TestVersionsMatching(c *gc.C) {
	v := getVersions(c)
	results, err := v.VersionsMatching(bytes.NewReader([]byte(fakeContent1)))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-amd64",
		"2.2.4-trusty-amd64",
		"2.2.4-centos7-amd64",
	})
	results, err = v.VersionsMatching(bytes.NewReader([]byte(fakeContent2)))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-arm64",
	})
}

func (s *signatureSuite) TestVersionsMatchingHash(c *gc.C) {
	v := getVersions(c)
	results := v.VersionsMatchingHash(
		"eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c")
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-amd64",
		"2.2.4-trusty-amd64",
		"2.2.4-centos7-amd64",
	})
	results = v.VersionsMatchingHash(
		"f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47")
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-arm64",
	})
}

const (
	testSigningKey = `-----BEGIN PGP PRIVATE KEY BLOCK-----
Version: GnuPG v1.4.10 (GNU/Linux)

lQHYBE2rFNoBBADFwqWQIW/DSqcB4yCQqnAFTJ27qS5AnB46ccAdw3u4Greeu3Bp
idpoHdjULy7zSKlwR1EA873dO/k/e11Ml3dlAFUinWeejWaK2ugFP6JjiieSsrKn
vWNicdCS4HTWn0X4sjl0ZiAygw6GNhqEQ3cpLeL0g8E9hnYzJKQ0LWJa0QARAQAB
AAP/TB81EIo2VYNmTq0pK1ZXwUpxCrvAAIG3hwKjEzHcbQznsjNvPUihZ+NZQ6+X
0HCfPAdPkGDCLCb6NavcSW+iNnLTrdDnSI6+3BbIONqWWdRDYJhqZCkqmG6zqSfL
IdkJgCw94taUg5BWP/AAeQrhzjChvpMQTVKQL5mnuZbUCeMCAN5qrYMP2S9iKdnk
VANIFj7656ARKt/nf4CBzxcpHTyB8+d2CtPDKCmlJP6vL8t58Jmih+kHJMvC0dzn
gr5f5+sCAOOe5gt9e0am7AvQWhdbHVfJU0TQJx+m2OiCJAqGTB1nvtBLHdJnfdC9
TnXXQ6ZXibqLyBies/xeY2sCKL5qtTMCAKnX9+9d/5yQxRyrQUHt1NYhaXZnJbHx
q4ytu0eWz+5i68IYUSK69jJ1NWPM0T6SkqpB3KCAIv68VFm9PxqG1KmhSrQIVGVz
dCBLZXmIuAQTAQIAIgUCTasU2gIbAwYLCQgHAwIGFQgCCQoLBBYCAwECHgECF4AA
CgkQO9o98PRieSoLhgQAkLEZex02Qt7vGhZzMwuN0R22w3VwyYyjBx+fM3JFETy1
ut4xcLJoJfIaF5ZS38UplgakHG0FQ+b49i8dMij0aZmDqGxrew1m4kBfjXw9B/v+
eIqpODryb6cOSwyQFH0lQkXC040pjq9YqDsO5w0WYNXYKDnzRV0p4H1pweo2VDid
AdgETasU2gEEAN46UPeWRqKHvA99arOxee38fBt2CI08iiWyI8T3J6ivtFGixSqV
bRcPxYO/qLpVe5l84Nb3X71GfVXlc9hyv7CD6tcowL59hg1E/DC5ydI8K8iEpUmK
/UnHdIY5h8/kqgGxkY/T/hgp5fRQgW1ZoZxLajVlMRZ8W4tFtT0DeA+JABEBAAEA
A/0bE1jaaZKj6ndqcw86jd+QtD1SF+Cf21CWRNeLKnUds4FRRvclzTyUMuWPkUeX
TaNNsUOFqBsf6QQ2oHUBBK4VCHffHCW4ZEX2cd6umz7mpHW6XzN4DECEzOVksXtc
lUC1j4UB91DC/RNQqwX1IV2QLSwssVotPMPqhOi0ZLNY7wIA3n7DWKInxYZZ4K+6
rQ+POsz6brEoRHwr8x6XlHenq1Oki855pSa1yXIARoTrSJkBtn5oI+f8AzrnN0BN
oyeQAwIA/7E++3HDi5aweWrViiul9cd3rcsS0dEnksPhvS0ozCJiHsq/6GFmy7J8
QSHZPteedBnZyNp5jR+H7cIfVN3KgwH/Skq4PsuPhDq5TKK6i8Pc1WW8MA6DXTdU
nLkX7RGmMwjC0DBf7KWAlPjFaONAX3a8ndnz//fy1q7u2l9AZwrj1qa1iJ8EGAEC
AAkFAk2rFNoCGwwACgkQO9o98PRieSo2/QP/WTzr4ioINVsvN1akKuekmEMI3LAp
BfHwatufxxP1U+3Si/6YIk7kuPB9Hs+pRqCXzbvPRrI8NHZBmc8qIGthishdCYad
AHcVnXjtxrULkQFGbGvhKURLvS9WnzD/m1K2zzwxzkPTzT9/Yf06O6Mal5AdugPL
VrM0m72/jnpKo04=
=zNCn
-----END PGP PRIVATE KEY BLOCK-----
`

	signedData = `
-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA1

versions:
  - version: 2.2.4-xenial-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-trusty-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-centos7-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-xenial-arm64
    sha256: f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47
-----BEGIN PGP SIGNATURE-----
Version: GnuPG v1

iJwEAQECAAYFAlm7JioACgkQO9o98PRieSr25wQAv3sbWbDHmQh6RiZoV+rw7FOt
onpj2aDnOHT1y2PVzaHGWEl9rRKpfF5sz2b98uPxwH0C2d/UtxVsHgZrQxNz1Lw+
VCotwf+Hyo5WgI63yAiez5mpeqwSSYdzZbtj9khpl1xt5eEPwc67HR0L1itbZ0nX
e6MRlGoEGSg+WgLJGys=
=VVPR
-----END PGP SIGNATURE-----
`

	fakeContent1 = "fake binary content 1\n"
	fakeContent2 = "fake binary content 2\n"
)
