// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// Note: In juju 2.x, the concept of juju tools were renamed to juju agent
// binaries. Seen in the renaming of the juju metadata plugin commands. You'll
// find references to both in the juju code base. It's the tarball of jujud
// installed on juju controllers and juju machines. It was NOT changed in the
// launchpad:simplestreams which still requires a tools directory.
//
// Package tools supports locating, parsing, and filtering juju agent binary
// metadata in simplestreams format.
//
// See http://launchpad.net/simplestreams and in particular the doc/README
// file in that project for more information about the file formats.
//
// Generally, agent binaries and related metadata are mirrored from https://streams.canonical.com/juju/tools.
//
// Providers may allow additional locations to search for metadata and agent
// binaries. For Openstack, keystone endpoints may be created by the cloud
// administrator. These are defined as follows:
//
// juju-tools      : the <path_url> value as described above in Agent Binaries Metadata Contents
// product-streams : the <path_url> value as described above in Image Metadata Contents

package tools
