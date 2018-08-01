// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

// NewMAAS returns an interface to the MAAS API as a *MAASObject.
func NewMAAS(client Client) *MAASObject {
	attrs := map[string]interface{}{resourceURI: client.APIURL.String()}
	obj := newJSONMAASObject(attrs, client)
	return &obj
}
