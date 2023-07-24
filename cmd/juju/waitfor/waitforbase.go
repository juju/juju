// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"reflect"
	"strings"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/cmd/juju/waitfor/api"
	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/cmd/modelcmd"
)

type waitForCommandBase struct {
	modelcmd.ModelCommandBase

	newWatchAllAPIFunc func() (api.WatchAllAPI, error)
}

type modelAllWatchShim struct {
	*apiclient.Client
}

func (s modelAllWatchShim) WatchAll() (api.AllWatcher, error) {
	return s.Client.WatchAll()
}

// runQuery handles the more complex error handling of a query with a given
// scope.
func runQuery(input string, q query.Query, scope query.Scope) (bool, error) {
	res, err := q.BuiltinsRun(scope)
	if err != nil {
		return false, HelpDisplay(err, input, scope.GetIdents())
	}
	return res, nil
}

func getIdents(q interface{}) []string {
	var res []string

	refType := reflect.TypeOf(q).Elem()
	for i := 0; i < refType.NumField(); i++ {
		field := refType.Field(i)
		v := strings.Split(field.Tag.Get("json"), ",")[0]
		refValue := reflect.ValueOf(q).Elem()

		switch refValue.Field(i).Kind() {
		case reflect.Int, reflect.Int64, reflect.Float64, reflect.String, reflect.Bool:
			res = append(res, v)
		}
	}
	return res
}
