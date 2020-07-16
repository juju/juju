// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type printInfoSuite struct{}

var _ = gc.Suite(&printInfoSuite{})

func (s *printInfoSuite) TestCharmPrintInfo(c *gc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, &ir)
	err := iw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
charm-id: charmCHARMcharmCHARMcharmCHARM01
summary: WordPress is a full featured web blogging tool, this charm deploys it.
publisher: Wordress Charmers
supports: bionic, xenial
tags: app, seven
subordinate: true
description: |-
  This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
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
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printInfoSuite) TestBundleChannelClosed(c *gc.C) {
	ir := getBundleInfoClosedTrack()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, &ir)
	err := iw.Print()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printInfoSuite) TestBundlePrintInfo(c *gc.C) {
	ir := getBundleInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx.Stdout, ctx.Warningf, &ir)
	err := iw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: osm
bundle-id: bundleBUNDLEbundleBUNDLEbundle01
summary: A bundle by charmed-osm.
publisher: charmed-osm
tags: networking
description: Single instance OSM bundle.
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
`
	c.Assert(obtained, gc.Equals, expected)
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
		Channels: map[string]Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Revision:   16,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/beta": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "beta",
				Revision:   17,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/candidate": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "candidate",
				Revision:   17,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/edge": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "edge",
				Revision:   18,
				Size:       12042240,
				Version:    "1.0.3",
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
		Publisher:   "Wordress Charmers",
		Description: "This will install and setup WordPress optimized to run in the cloud.\nBy default it will place Ngnix and php-fpm configured to scale horizontally with\nNginx's reverse proxy.",
		Series:      []string{"bionic", "xenial"},
		Tags:        []string{"app", "seven"},
		Charm: &Charm{
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
		Channels: map[string]Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Revision:   16,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/beta": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "beta",
				Revision:   17,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/candidate": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "candidate",
				Revision:   17,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/edge": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "edge",
				Revision:   18,
				Size:       12042240,
				Version:    "1.0.3",
			}},
		Tracks: []string{"latest"},
	}
}

func getBundleInfoClosedTrack() InfoResponse {
	return InfoResponse{
		Name: "osm",
		Type: "bundle",
		Channels: map[string]Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Revision:   15,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/beta": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "beta",
				Revision:   17,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/candidate": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "candidate",
				Revision:   16,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"latest/edge": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "edge",
				Revision:   18,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"2.8/candidate": {
				ReleasedAt: "2019-12-13T19:44:44.076943+00:00",
				Track:      "2.8",
				Risk:       "candidate",
				Revision:   56,
				Size:       12042240,
				Version:    "1.0.3",
			},
			"2.8/edge": {
				ReleasedAt: "2019-12-17T19:44:44.076943+00:00",
				Track:      "2.8",
				Risk:       "edge",
				Revision:   60,
				Size:       12042240,
				Version:    "1.0.3",
			}},
		Tracks: []string{"latest", "2.8"},
	}
}
