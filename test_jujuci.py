import json
from mock import patch
from StringIO import StringIO
from unittest import TestCase

from jujuci import (
    get_build_data,
    list_files,
)


def make_build_data():
    return {
        "actions": [
            {
                "parameters": [
                    {
                        "name": "branch",
                        "value": "gitbranch:master:github.com/juju/juju"
                    },
                    {
                        "name": "revision",
                        "value": "3c53cf578ef100ba5368661224de4af5da72ee74"
                    }
                ]
            },
            {
                "causes": [
                    {
                        "shortDescription": "Started by user admin",
                        "userName": "admin"
                    }
                ]
            },
            {},
            {}
        ],
        "artifacts": [
            {
                "displayPath": "buildvars.bash",
                "fileName": "buildvars.bash",
                "relativePath": "buildvars.bash"
            },
            {
                "displayPath": "buildvars.json",
                "fileName": "buildvars.json",
                "relativePath": "buildvars.json"
            },
            {
                "displayPath": "juju-core_1.22-alpha1.tar.gz",
                "fileName": "juju-core_1.22-alpha1.tar.gz",
                "relativePath": "juju-core_1.22-alpha1.tar.gz"
            }
        ],
        "building": False,
        "builtOn": "",
        "changeSet": {
            "items": [],
            "kind": None
        },
        "culprits": [],
        "description": "gitbranch:master:github.com/juju/juju 3c53cf57",
        "duration": 142986,
        "fullDisplayName": "build-revision #2102",
        "id": "2014-11-19_07-35-02",
        "keepLog": False,
        "number": 2102,
        "result": "SUCCESS",
        "timestamp": 1416382502379,
        "url": "http://juju-ci.vapour.ws:8080/job/build-revision/2102/"
    }


class JujuCITestCase(TestCase):

    def test_get_build_data(self):
        expected_data = make_build_data()
        json_io = StringIO(json.dumps(expected_data))
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            build_data = get_build_data('http://foo:8080', 'bar', '1234')
        mock.assert_called_once(['http://foo:8080/job/bar/1234/api/json'])
        self.assertEqual(expected_data, build_data)

    def test_get_build_data_with_default_build(self):
        expected_data = make_build_data()
        json_io = StringIO(json.dumps(expected_data))
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            get_build_data('http://foo:8080', 'bar')
        mock.assert_called_once(
            ['http://foo:8080/job/bar/lastSuccessfulBuild/api/json'])

    def test_list_files_all(self):
        expected_data = make_build_data()
        files = list_files(expected_data)
        self.assertEqual(
            ['buildvars.bash', 'buildvars.json',
             'juju-core_1.22-alpha1.tar.gz'],
            sorted(files))

    def test_list_files_glob_tarball(self):
        expected_data = make_build_data()
        files = list_files(expected_data, '*.tar.gz')
        self.assertEqual(['juju-core_1.22-alpha1.tar.gz'], files)
