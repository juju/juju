// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package simplestreams supports locating, parsing, and filtering metadata in
// simplestreams format.
//
// See http://launchpad.net/simplestreams and in particular the doc/README
// file in that project for more information about the file formats.
//
// Users of this package provide an empty struct and a matching function to be
// able to query and return a list of typed values for a given criteria.
package simplestreams
