// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"encoding/json"
	"reflect"
)

// itemCollection is a clone of ItemCollection, but
// does not implement json.Unmarshaler.
type itemCollection struct {
	Items      map[string]*json.RawMessage `json:"items"`
	Arch       string                      `json:"arch,omitempty"`
	Version    string                      `json:"version,omitempty"`
	Series     string                      `json:"release,omitempty"`
	RegionName string                      `json:"region,omitempty"`
	Endpoint   string                      `json:"endpoint,omitempty"`
}

// ItemsCollection.UnmarshalJSON unmarshals the ItemCollection,
// storing the raw bytes for each item. These can later be
// unmarshalled again into product-specific types.
func (c *ItemCollection) UnmarshalJSON(b []byte) error {
	var raw itemCollection
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.rawItems = raw.Items
	c.Items = make(map[string]interface{}, len(raw.Items))
	c.Arch = raw.Arch
	c.Version = raw.Version
	c.Series = raw.Series
	c.RegionName = raw.RegionName
	c.Endpoint = raw.Endpoint
	for key, rawv := range raw.Items {
		var v interface{}
		if err := json.Unmarshal([]byte(*rawv), &v); err != nil {
			return err
		}
		c.Items[key] = v
	}
	return nil
}

func (c *ItemCollection) construct(itemType reflect.Type) error {
	for key, rawv := range c.rawItems {
		itemValuePtr := reflect.New(itemType)
		err := json.Unmarshal([]byte(*rawv), itemValuePtr.Interface())
		if err != nil {
			return err
		}
		c.Items[key] = itemValuePtr.Interface()
	}
	return nil
}
