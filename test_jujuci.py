import json
from mock import patch
from StringIO import StringIO
from unittest import TestCase

from jujuci import get_build_data


def make_build_json():
    return json.dumps({
        "actions": [],
        "buildable": True,
        "builds": [
            {
                "number": 2090,
                "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2090/"
            },
            {
                "number": 2089,
                "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2089/"
            }
        ],
        "color": "blue",
        "concurrentBuild": False,
        "description": "tags: subjects=release",
        "displayName": "build-revision",
        "downstreamProjects": [],
        "firstBuild": {
            "number": 2089,
            "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2089/"
        },
        "healthReport": [
            {
                "description": "Build stability: No recent builds failed.",
                "iconUrl": "health-80plus.png",
                "score": 100
            }
        ],
        "inQueue": False,
        "lastBuild": {
            "number": 2090,
            "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2090/"
        },
        "lastCompletedBuild": {
            "number": 2090,
            "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2090/"
        },
        "lastFailedBuild": None,
        "lastStableBuild": {
            "number": 2090,
            "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2090/"
        },
        "lastSuccessfulBuild": {
            "number": 2090,
            "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2090/"
        },
        "lastUnstableBuild": None,
        "lastUnsuccessfulBuild": None,
        "name": "build-revision",
        "nextBuildNumber": 2091,
        "property": [],
        "queueItem": None,
        "scm": {},
        "upstreamProjects": [],
        "url": "http://juju-ci.vapour.ws:8080/job/build-revision/"
    })


class JujuCITestCase(TestCase):

    def test_get_build_data(self):
        expected_json = make_build_json()
        json_io = StringIO(expected_json)
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            build_data = get_build_data('http://foo:8080', 'bar', '1234')
        mock.assert_called_once(['http://foo:8080/job/bar/1234/api/json'])
        expected_data = json.loads(expected_json)
        self.assertEqual(expected_data, build_data)

    def test_get_build_data_with_default_build(self):
        expected_json = make_build_json()
        json_io = StringIO(expected_json)
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            get_build_data('http://foo:8080', 'bar')
        mock.assert_called_once(
            ['http://foo:8080/job/bar/lastSuccessfulBuild/api/json'])
