// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// MAASObject represents a MAAS object as returned by the MAAS API, such as a
// Node or a Tag.
// You can extract a MAASObject out of a JSONObject using
// JSONObject.GetMAASObject.  A MAAS API call will usually return either a
// MAASObject or a list of MAASObjects.  The list itself would be wrapped in
// a JSONObject, so if an API call returns a list of objects "l," you first
// obtain the array using l.GetArray().  Then, for each item "i" in the array,
// obtain the matching MAASObject using i.GetMAASObject().
type MAASObject struct {
	values map[string]JSONObject
	client Client
	uri    *url.URL
}

// newJSONMAASObject creates a new MAAS object.  It will panic if the given map
// does not contain a valid URL for the 'resource_uri' key.
func newJSONMAASObject(jmap map[string]interface{}, client Client) MAASObject {
	obj, err := maasify(client, jmap).GetMAASObject()
	if err != nil {
		panic(err)
	}
	return obj
}

// MarshalJSON tells the standard json package how to serialize a MAASObject.
func (obj MAASObject) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(obj.GetMap(), "", "  ")
}

// With MarshalJSON, MAASObject implements json.Marshaler.
var _ json.Marshaler = (*MAASObject)(nil)

func marshalNode(node MAASObject) string {
	res, _ := json.MarshalIndent(node, "", "  ")
	return string(res)

}

var noResourceURI = errors.New("not a MAAS object: no 'resource_uri' key")

// extractURI obtains the "resource_uri" string from a JSONObject map.
func extractURI(attrs map[string]JSONObject) (*url.URL, error) {
	uriEntry, ok := attrs[resourceURI]
	if !ok {
		return nil, noResourceURI
	}
	uri, err := uriEntry.GetString()
	if err != nil {
		return nil, fmt.Errorf("invalid resource_uri: %v", uri)
	}
	resourceURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("resource_uri does not contain a valid URL: %v", uri)
	}
	return resourceURL, nil
}

// JSONObject getter for a MAAS object.  From a decoding perspective, a
// MAASObject is just like a map except it contains a key "resource_uri", and
// it keeps track of the Client you got it from so that you can invoke API
// methods directly on their MAAS objects.
func (obj JSONObject) GetMAASObject() (MAASObject, error) {
	attrs, err := obj.GetMap()
	if err != nil {
		return MAASObject{}, err
	}
	uri, err := extractURI(attrs)
	if err != nil {
		return MAASObject{}, err
	}
	return MAASObject{values: attrs, client: obj.client, uri: uri}, nil
}

// GetField extracts a string field from this MAAS object.
func (obj MAASObject) GetField(name string) (string, error) {
	return obj.values[name].GetString()
}

// URI is the resource URI for this MAAS object.  It is an absolute path, but
// without a network part.
func (obj MAASObject) URI() *url.URL {
	// Duplicate the URL.
	uri, err := url.Parse(obj.uri.String())
	if err != nil {
		panic(err)
	}
	return uri
}

// URL returns a full absolute URL (including network part) for this MAAS
// object on the API.
func (obj MAASObject) URL() *url.URL {
	return obj.client.GetURL(obj.URI())
}

// GetMap returns all of the object's attributes in the form of a map.
func (obj MAASObject) GetMap() map[string]JSONObject {
	return obj.values
}

// GetSubObject returns a new MAASObject representing the API resource found
// at a given sub-path of the current object's resource URI.
func (obj MAASObject) GetSubObject(name string) MAASObject {
	uri := obj.URI()
	newURL := url.URL{Path: name}
	resUrl := uri.ResolveReference(&newURL)
	resUrl.Path = EnsureTrailingSlash(resUrl.Path)
	input := map[string]interface{}{resourceURI: resUrl.String()}
	return newJSONMAASObject(input, obj.client)
}

var NotImplemented = errors.New("Not implemented")

// Get retrieves a fresh copy of this MAAS object from the API.
func (obj MAASObject) Get() (MAASObject, error) {
	uri := obj.URI()
	result, err := obj.client.Get(uri, "", url.Values{})
	if err != nil {
		return MAASObject{}, err
	}
	jsonObj, err := Parse(obj.client, result)
	if err != nil {
		return MAASObject{}, err
	}
	return jsonObj.GetMAASObject()
}

// Post overwrites this object's existing value on the API with those given
// in "params."  It returns the object's new value as received from the API.
func (obj MAASObject) Post(params url.Values) (JSONObject, error) {
	uri := obj.URI()
	result, err := obj.client.Post(uri, "", params, nil)
	if err != nil {
		return JSONObject{}, err
	}
	return Parse(obj.client, result)
}

// Update modifies this object on the API, based on the values given in
// "params."  It returns the object's new value as received from the API.
func (obj MAASObject) Update(params url.Values) (MAASObject, error) {
	uri := obj.URI()
	result, err := obj.client.Put(uri, params)
	if err != nil {
		return MAASObject{}, err
	}
	jsonObj, err := Parse(obj.client, result)
	if err != nil {
		return MAASObject{}, err
	}
	return jsonObj.GetMAASObject()
}

// Delete removes this object on the API.
func (obj MAASObject) Delete() error {
	uri := obj.URI()
	return obj.client.Delete(uri)
}

// CallGet invokes an idempotent API method on this object.
func (obj MAASObject) CallGet(operation string, params url.Values) (JSONObject, error) {
	uri := obj.URI()
	result, err := obj.client.Get(uri, operation, params)
	if err != nil {
		return JSONObject{}, err
	}
	return Parse(obj.client, result)
}

// CallPost invokes a non-idempotent API method on this object.
func (obj MAASObject) CallPost(operation string, params url.Values) (JSONObject, error) {
	return obj.CallPostFiles(operation, params, nil)
}

// CallPostFiles invokes a non-idempotent API method on this object.  It is
// similar to CallPost but has an extra parameter, 'files', which should
// contain the files that will be uploaded to the API.
func (obj MAASObject) CallPostFiles(operation string, params url.Values, files map[string][]byte) (JSONObject, error) {
	uri := obj.URI()
	result, err := obj.client.Post(uri, operation, params, files)
	if err != nil {
		return JSONObject{}, err
	}
	return Parse(obj.client, result)
}
