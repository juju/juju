// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

var driveOwner = MakeUserResource("80cb30fb-0ea3-43db-b27b-a125752cc0bf")

var drivesData = []Drive{
	Drive{
		Resource: *MakeDriveResource("2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"),
		Owner:    driveOwner,
		Status:   "unmounted",
	},
	Drive{
		Resource: *MakeDriveResource("3b30c7ef-1fda-416d-91d1-ba616859360c"),
		Owner:    driveOwner,
		Status:   "unmounted",
	},
	Drive{
		Resource: *MakeDriveResource("464aed14-8604-4277-be3c-9d53151d53b4"),
		Owner:    driveOwner,
		Status:   "unmounted",
	},
}

const jsonDrivesData = `{
    "meta": {
        "limit": 0,
        "offset": 0,
        "total_count": 9
    },
    "objects": [
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff/",
            "status": "unmounted",
            "uuid": "2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/3b30c7ef-1fda-416d-91d1-ba616859360c/",
            "status": "unmounted",
            "uuid": "3b30c7ef-1fda-416d-91d1-ba616859360c"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/464aed14-8604-4277-be3c-9d53151d53b4/",
            "status": "unmounted",
            "uuid": "464aed14-8604-4277-be3c-9d53151d53b4"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/47ec5074-6058-4b0f-9505-78c83bd5a88b/",
            "status": "unmounted",
            "uuid": "47ec5074-6058-4b0f-9505-78c83bd5a88b"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/7949e52e-c8ba-461b-a84f-3f247221c644/",
            "status": "unmounted",
            "uuid": "7949e52e-c8ba-461b-a84f-3f247221c644"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/81b020b0-8ea0-4602-b778-e4df4539f0f7/",
            "status": "unmounted",
            "uuid": "81b020b0-8ea0-4602-b778-e4df4539f0f7"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/baf8fed4-757f-4d9e-a23a-3b3ff81e16c4/",
            "status": "unmounted",
            "uuid": "baf8fed4-757f-4d9e-a23a-3b3ff81e16c4"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/cae6df75-00a1-490c-a96b-51777b3ec515/",
            "status": "unmounted",
            "uuid": "cae6df75-00a1-490c-a96b-51777b3ec515"
        },
        {
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/e15dd971-3ef8-497c-9f92-90d5ca1722bd/",
            "status": "unmounted",
            "uuid": "e15dd971-3ef8-497c-9f92-90d5ca1722bd"
        }
    ]
}`

var drivesDetailData = []Drive{
	Drive{
		Resource:    *MakeDriveResource("2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"),
		Jobs:        nil,
		Media:       "disk",
		Meta:        nil,
		Name:        "test_drive_2",
		Owner:       driveOwner,
		Size:        1073741824,
		Status:      "unmounted",
		StorageType: "dssd",
	},
	Drive{
		Resource: *MakeDriveResource("3b30c7ef-1fda-416d-91d1-ba616859360c"),
		Jobs: []Resource{
			*MakeJobResource("fbe05708-fd42-43d5-814c-9cb805edd4cb"),
			*MakeJobResource("32513930-6815-4cd4-ae8e-2eb89733c206"),
		},

		Media:       "disk",
		Meta:        nil,
		Name:        "atom",
		Owner:       driveOwner,
		Size:        10737418240,
		Status:      "unmounted",
		StorageType: "dssd",
	},
	Drive{
		Resource:    *MakeDriveResource("464aed14-8604-4277-be3c-9d53151d53b4"),
		Jobs:        nil,
		Media:       "disk",
		Meta:        nil,
		Name:        "test_drive_1",
		Owner:       driveOwner,
		Size:        1073741824,
		Status:      "unmounted",
		StorageType: "dssd",
	},
}

const jsonDrivesDetailData = `{
    "meta": {
        "limit": 0,
        "offset": 0,
        "total_count": 9
    },
    "objects": [
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "test_drive_2",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [
                {
                    "resource_uri": "/api/2.0/jobs/fbe05708-fd42-43d5-814c-9cb805edd4cb/",
                    "uuid": "fbe05708-fd42-43d5-814c-9cb805edd4cb"
                },
                {
                    "resource_uri": "/api/2.0/jobs/32513930-6815-4cd4-ae8e-2eb89733c206/",
                    "uuid": "32513930-6815-4cd4-ae8e-2eb89733c206"
                }
            ],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "atom",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/3b30c7ef-1fda-416d-91d1-ba616859360c/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 10737418240,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "3b30c7ef-1fda-416d-91d1-ba616859360c"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "test_drive_1",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/464aed14-8604-4277-be3c-9d53151d53b4/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "464aed14-8604-4277-be3c-9d53151d53b4"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "test_drive_4",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/47ec5074-6058-4b0f-9505-78c83bd5a88b/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "47ec5074-6058-4b0f-9505-78c83bd5a88b"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {
                "description": ""
            },
            "mounted_on": [],
            "name": "xxx",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/7949e52e-c8ba-461b-a84f-3f247221c644/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "7949e52e-c8ba-461b-a84f-3f247221c644"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "t1",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/81b020b0-8ea0-4602-b778-e4df4539f0f7/",
            "runtime": {
                "is_snapshotable": false,
                "snapshots_allocated_size": 0,
                "storage_type": "zadara"
            },
            "size": 3221225472,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "zadara",
            "tags": [],
            "uuid": "81b020b0-8ea0-4602-b778-e4df4539f0f7"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "test_drive_3",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/baf8fed4-757f-4d9e-a23a-3b3ff81e16c4/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "baf8fed4-757f-4d9e-a23a-3b3ff81e16c4"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {},
            "mounted_on": [],
            "name": "test_drive_0",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/cae6df75-00a1-490c-a96b-51777b3ec515/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 1073741824,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "cae6df75-00a1-490c-a96b-51777b3ec515"
        },
        {
            "affinities": [],
            "allow_multimount": false,
            "jobs": [],
            "licenses": [],
            "media": "disk",
            "meta": {
                "arch": "64",
                "category": "general",
                "description": "",
                "favourite": "False",
                "image_type": "preinst",
                "install_notes": "",
                "os": "linux",
                "paid": "False",
                "url": ""
            },
            "mounted_on": [],
            "name": "otom",
            "owner": {
                "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
                "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
            },
            "resource_uri": "/api/2.0/drives/e15dd971-3ef8-497c-9f92-90d5ca1722bd/",
            "runtime": {
                "is_snapshotable": true,
                "snapshots_allocated_size": 0,
                "storage_type": "dssd"
            },
            "size": 10737418240,
            "snapshots": [],
            "status": "unmounted",
            "storage_type": "dssd",
            "tags": [],
            "uuid": "e15dd971-3ef8-497c-9f92-90d5ca1722bd"
        }
    ]
}`

var driveData = Drive{
	Resource:    *MakeDriveResource("2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"),
	Affinities:  []string{"123", "321"},
	Jobs:        nil,
	Media:       "disk",
	Meta:        nil,
	Name:        "test_drive_2",
	Owner:       driveOwner,
	Size:        1073741824,
	Status:      "unmounted",
	StorageType: "dssd",
}

const jsonDriveData = `{
    "affinities": ["123","321"],
    "allow_multimount": false,
    "jobs": [],
    "licenses": [],
    "media": "disk",
    "meta": {},
    "mounted_on": [],
    "name": "test_drive_2",
    "owner": {
        "resource_uri": "/api/2.0/user/80cb30fb-0ea3-43db-b27b-a125752cc0bf/",
        "uuid": "80cb30fb-0ea3-43db-b27b-a125752cc0bf"
    },
    "resource_uri": "/api/2.0/drives/2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff/",
    "runtime": {
        "is_snapshotable": true,
        "snapshots_allocated_size": 0,
        "storage_type": "dssd"
    },
    "size": 1073741824,
    "snapshots": [],
    "status": "unmounted",
    "storage_type": "dssd",
    "tags": [],
    "uuid": "2ef7b7c7-7ec4-47a7-9b69-087c9417c0ff"
}`

var libraryDriveData = Drive{
	Resource: *MakeLibDriveResource("22bd1b24-ea78-47bb-a59b-a09ed5407867"),
	Media:    "cdrom",
	Name:     "Debian 7.1.0 Netinstall",
	Owner:    nil,
	Status:   "unmounted",
	Size:     536870912,
	LibraryDrive: LibraryDrive{
		Arch:      "64",
		ImageType: "install",
		OS:        "linux",
		Paid:      false,
	},
}

const jsonLibraryDriveData = `{
            "affinities": [],
            "allow_multimount": false,
            "arch": "64",
            "category": [
                "general"
            ],
            "description": "Debian 7.1.0 Netinstall AMD64.",
            "favourite": false,
            "image_type": "install",
            "install_notes": "1. Attach the CD. \\n\r\nPlease be aware that the CD needs to be attached to the server as IDE. \\n\r\n \\n\r\n2. Attach a Drive. \\n\r\nPlease be aware that the minimum drive size where you are going to install the OS should be 5 GB. \\n\r\n \\n\r\n3. Connecting to your server via VNC. \\n\r\na) Go to the \u201cProperties\u201d tab of the server and Turn on the VNC Tunnel by clicking the button right next to it \\n\r\nb) In order to use the inbuilt client click on the icon right next to the VNC link and choose \u201cOpen in Dialog Window\u201d or \u201cOpen in new browser window/tab\u201d \\n\r\nOR \\n\r\nc) Having installed a compatible VNC client, open a VNC connection to your server through the UI.  \\n\r\nd) Enter your VNC url and VNC password as displayed on your Server Properties Window.  \\n\r\n \\n\r\n4. Minimum Hardware Requirements. \\n\r\nThe recommended minimum hardware requirements as published by debian.org are: 1GB RAM and 1GHz CPU",
            "jobs": [],
            "licenses": [],
            "media": "cdrom",
            "meta": {},
            "mounted_on": [],
            "name": "Debian 7.1.0 Netinstall",
            "os": "linux",
            "owner": null,
            "paid": false,
            "resource_uri": "/api/2.0/libdrives/22bd1b24-ea78-47bb-a59b-a09ed5407867/",
            "size": 536870912,
            "status": "unmounted",
            "storage_type": null,
            "tags": [],
            "url": "http://debian.org/",
            "uuid": "22bd1b24-ea78-47bb-a59b-a09ed5407867"
        }`
