// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package ec2instancetypes contains instance type information
// for the ec2 provider, generated from the AWS Price List API.
//
// To update this package, first fetch index.json to this
// directory, and then run "go generate". The current index.json
// file can be found at:
//     https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.json
package ec2instancetypes
