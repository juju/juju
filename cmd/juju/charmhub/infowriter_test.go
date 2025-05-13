// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type printInfoSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&printInfoSuite{})

func (s *printInfoSuite) TestCharmPrintInfo(c *tc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeBoth, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
charm-id: charmCHARMcharmCHARMcharmCHARM01
supports: ubuntu@18.04, ubuntu@16.04
tags: app, seven
subordinate: true
relations:
  provides:
    one: two
    three: four
  requires:
    five: six
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB  amd64  ubuntu@22.04, ubuntu@20.04
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB  amd64  ubuntu@22.04
  latest/beta:       1.0.3  2019-12-16  (17)  12MB  amd64  ubuntu@22.04
  latest/edge:       1.0.3  2019-12-16  (18)  12MB  amd64  coolos@3.14
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestCharmPrintInfoModeNone(c *tc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeNone, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
charm-id: charmCHARMcharmCHARMcharmCHARM01
supports: ubuntu@18.04, ubuntu@16.04
tags: app, seven
subordinate: true
relations:
  provides:
    one: two
    three: four
  requires:
    five: six
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestCharmPrintInfoModeArches(c *tc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeArches, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
charm-id: charmCHARMcharmCHARMcharmCHARM01
supports: ubuntu@18.04, ubuntu@16.04
tags: app, seven
subordinate: true
relations:
  provides:
    one: two
    three: four
  requires:
    five: six
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB  amd64
                     1.0.3  2018-12-16  (15)  12MB  arm64
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB  amd64
  latest/beta:       1.0.3  2019-12-16  (17)  12MB  amd64
  latest/edge:       1.0.3  2019-12-16  (18)  12MB  amd64
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestCharmPrintInfoModeBases(c *tc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeBases, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
charm-id: charmCHARMcharmCHARMcharmCHARM01
supports: ubuntu@18.04, ubuntu@16.04
tags: app, seven
subordinate: true
relations:
  provides:
    one: two
    three: four
  requires:
    five: six
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB  ubuntu@22.04, ubuntu@20.04
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB  ubuntu@22.04
  latest/beta:       1.0.3  2019-12-16  (17)  12MB  ubuntu@22.04
  latest/edge:       1.0.3  2019-12-16  (18)  12MB  coolos@3.14
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestCharmPrintInfoWithConfig(c *tc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, true, "never", baseModeNone, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
charm-id: charmCHARMcharmCHARMcharmCHARM01
supports: ubuntu@18.04, ubuntu@16.04
tags: app, seven
subordinate: true
relations:
  provides:
    one: two
    three: four
  requires:
    five: six
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
config:
  settings:
    status:
      type: string
      description: temporary string for unit status
      default: hello
    thing:
      type: string
      description: A thing used by the charm.
      default: "\U0001F381"
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestBundleChannelClosed(c *tc.C) {
	ir := getBundleInfoClosedTrack()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeBoth, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: osm
channels: |
  latest/stable:     1.0.3  2019-12-16  (15)  12MB
  latest/candidate:  1.0.3  2019-12-16  (16)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
  2.8/stable:        --
  2.8/candidate:     1.0.3  2019-12-13  (56)  12MB
  2.8/beta:          ^
  2.8/edge:          1.0.3  2019-12-17  (60)  12MB
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestBundleChannelClosedWithUnicode(c *tc.C) {
	ir := getBundleInfoClosedTrack()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "always", baseModeBoth, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: osm
channels: |
  latest/stable:     1.0.3  2019-12-16  (15)  12MB
  latest/candidate:  1.0.3  2019-12-16  (16)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
  2.8/stable:        –
  2.8/candidate:     1.0.3  2019-12-13  (56)  12MB
  2.8/beta:          ↑
  2.8/edge:          1.0.3  2019-12-17  (60)  12MB
`
	c.Assert(obtained, tc.Equals, expected)
}

func (s *printInfoSuite) TestBundlePrintInfo(c *tc.C) {
	ir := getBundleInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, false, "never", baseModeBoth, &ir, -1)
	err := iw.Print()
	c.Assert(err, tc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: osm
publisher: charmed-osm
summary: A bundle by charmed-osm.
description: Single instance OSM bundle.
bundle-id: bundleBUNDLEbundleBUNDLEbundle01
tags: networking
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
`
	c.Assert(obtained, tc.Equals, expected)
}

func getBundleInfoResponse() InfoResponse {
	return InfoResponse{
		Type:        "Bundle",
		ID:          "bundleBUNDLEbundleBUNDLEbundle01",
		Name:        "osm",
		Description: "Single instance OSM bundle.",
		Publisher:   "charmed-osm",
		Summary:     "A bundle by charmed-osm.",
		Tags:        []string{"networking"},
		Bundle:      nil,
		Channels: RevisionsMap{
			"latest": {
				"stable": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "stable",
					Revision:   16,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"beta": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "beta",
					Revision:   17,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"candidate": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "candidate",
					Revision:   17,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"edge": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "edge",
					Revision:   18,
					Size:       12042240,
					Version:    "1.0.3",
				}},
			}},
		Tracks: []string{"latest"},
	}
}

func getCharmInfoResponse() InfoResponse {
	return InfoResponse{
		Type:        "charm",
		ID:          "charmCHARMcharmCHARMcharmCHARM01",
		Name:        "wordpress",
		Summary:     "WordPress is a full featured web blogging tool, this charm deploys it.",
		Publisher:   "WordPress Charmers",
		Description: "This will install and setup WordPress optimized to run in the cloud.\nBy default it will place Ngnix and php-fpm configured to scale horizontally with\nNginx's reverse proxy.",
		Supports: []Base{
			{Name: "ubuntu", Channel: "18.04"},
			{Name: "ubuntu", Channel: "16.04"},
		},
		Tags: []string{"app", "seven"},
		Charm: &Charm{
			Config: &charm.Config{
				Options: map[string]charm.Option{
					"status": {
						Type:        "string",
						Description: "temporary string for unit status",
						Default:     "hello",
					},
					"thing": {
						Type:        "string",
						Description: "A thing used by the charm.",
						Default:     "🎁",
					},
				},
			},
			Subordinate: true,
			Relations: map[string]map[string]string{
				"provides": {
					"one":   "two",
					"three": "four",
				},
				"requires": {
					"five": "six",
				},
			},
		},
		Channels: RevisionsMap{
			"latest": {
				"stable": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "stable",
					Revision:   16,
					Size:       12042240,
					Version:    "1.0.3",
					Arches:     []string{"amd64"},
					Bases: []Base{
						{Name: "ubuntu", Channel: "22.04"},
						{Name: "ubuntu", Channel: "20.04"},
					},
				}, {
					ReleasedAt: "2018-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "stable",
					Revision:   15,
					Size:       12042240,
					Version:    "1.0.3",
					Arches:     []string{"arm64"},
					Bases: []Base{
						{Name: "ubuntu", Channel: "22.04"},
					},
				}},
				"beta": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "beta",
					Revision:   17,
					Size:       12042240,
					Version:    "1.0.3",
					Arches:     []string{"amd64"},
					Bases: []Base{
						{Name: "ubuntu", Channel: "22.04"},
					},
				}},
				"candidate": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "candidate",
					Revision:   17,
					Size:       12042240,
					Version:    "1.0.3",
					Arches:     []string{"amd64"},
					Bases: []Base{
						{Name: "ubuntu", Channel: "22.04"},
					},
				}},
				"edge": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "edge",
					Revision:   18,
					Size:       12042240,
					Version:    "1.0.3",
					Arches:     []string{"amd64"},
					Bases: []Base{
						{Name: "coolos", Channel: "3.14"},
					},
				}},
			}},
		Tracks: []string{"latest"},
	}
}

func getBundleInfoClosedTrack() InfoResponse {
	return InfoResponse{
		Name: "osm",
		Type: "bundle",
		Channels: RevisionsMap{
			"latest": {
				"stable": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "stable",
					Revision:   15,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"beta": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "beta",
					Revision:   17,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"candidate": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "candidate",
					Revision:   16,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"edge": {{
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
					Track:      "latest",
					Risk:       "edge",
					Revision:   18,
					Size:       12042240,
					Version:    "1.0.3",
				}},
			},
			"2.8": {
				"candidate": {{
					ReleasedAt: "2019-12-13T19:44:44.076943+00:00",
					Track:      "2.8",
					Risk:       "candidate",
					Revision:   56,
					Size:       12042240,
					Version:    "1.0.3",
				}},
				"edge": {{
					ReleasedAt: "2019-12-17T19:44:44.076943+00:00",
					Track:      "2.8",
					Risk:       "edge",
					Revision:   60,
					Size:       12042240,
					Version:    "1.0.3",
				}},
			}},
		Tracks: []string{"latest", "2.8"},
	}
}
