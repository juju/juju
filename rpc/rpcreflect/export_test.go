// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpcreflect

import "reflect"

func ResetCaches() {
	typeMutex.Lock()
	typesByGoType = make(map[reflect.Type]*Type)
	typeMutex.Unlock()

	objTypeMutex.Lock()
	objTypesByGoType = make(map[reflect.Type]*ObjType)
	objTypeMutex.Unlock()
}
