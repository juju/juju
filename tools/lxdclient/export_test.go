// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

var NewInstanceSummary = newInstanceSummary

type RawInstanceClient rawInstanceClient

func NewInstanceClient(raw RawInstanceClient) *instanceClient {
	return &instanceClient{
		raw:    rawInstanceClient(raw),
		remote: "",
	}
}
