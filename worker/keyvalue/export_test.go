// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyvalue

var (
	NewKeyValueStore = newKeyValueStore
)

func (f *KeyValueStore) ResetData() {
	f.data = make(map[string]interface{})
}
