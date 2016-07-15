// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package mock

import "net/http"

var response = `{
    "drives": {
        "dssd": {
            "max_size": 4391067795456,
            "min_size": 536870912
        },
        "zadara": {
            "max_size": 5905580032000,
            "min_size": 1073741824
        }
    },
    "servers": {
        "cpu": {
            "max": 40000,
            "min": 250
        },
        "cpu_per_smp": {
            "max": 2500,
            "min": 1000
        },
        "mem": {
            "max": 68719476736,
            "min": 268435456
        },
        "smp": {
            "max": 24,
            "min": 1
        }
    },
    "snapshots": {
        "current": 0,
        "max": 600
    }
}
`

func capsHandler(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h["Content-Type"] = append(h["Content-Type"], "application/json")
	h["Content-Type"] = append(h["Content-Type"], "charset=utf-8")
	w.Write([]byte(response))
}
