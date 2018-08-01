// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import "time"

var jobData = Job{
	Resource: Resource{
		"/api/2.0/jobs/305867d6-5652-41d2-be5c-bbae1eed5676/",
		"305867d6-5652-41d2-be5c-bbae1eed5676",
	},
	Created:      time.Date(2014, time.January, 30, 15, 24, 42, 205092, time.UTC),
	Data:         JobData{100},
	LastModified: time.Date(2014, time.January, 30, 15, 24, 42, 937432, time.UTC),
	Operation:    "drive_clone",
	Resources: []string{
		"/api/2.0/drives/df05497c-1504-4fea-af24-2825fc5133cf/",
		"/api/2.0/drives/db7a095c-622d-4b98-88fd-25a7e34d402e/",
	},
	State: "success",
}

const jsonJobData = `{
    "children": [],
    "created": "2014-01-30T15:24:42.205092+00:00",
    "data": {
        "progress": 100
    },
    "last_modified": "2014-01-30T15:24:42.937432+00:00",
    "operation": "drive_clone",
    "resource_uri": "/api/2.0/jobs/305867d6-5652-41d2-be5c-bbae1eed5676/",
    "resources": [
        "/api/2.0/drives/df05497c-1504-4fea-af24-2825fc5133cf/",
        "/api/2.0/drives/db7a095c-622d-4b98-88fd-25a7e34d402e/"
    ],
    "state": "success",
    "uuid": "305867d6-5652-41d2-be5c-bbae1eed5676"
}
`
