import json
from mock import patch
from StringIO import StringIO
from unittest import TestCase

from jujuci import (
    get_build_data,
    JENKINS_URL,
    list_artifacts,
    list_files,
    main,
)


def make_build_data(number='lastSuccessfulBuild'):
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
        "url": "http://juju-ci.vapour.ws:8080/job/build-revision/%s/" % number
    }


class JujuCITestCase(TestCase):

    def test_main_list_options(self):
        with patch('jujuci.list_artifacts') as mock:
            main(['-d', '-v', '-b', '1234', 'list', 'foo', '*.tar.gz'])
            args, kwargs = mock.call_args
            self.assertEqual(('foo', '1234', '*.tar.gz'), args)
            self.assertTrue(kwargs['verbose'])
            self.assertTrue(kwargs['dry_run'])

    def test_main_get_options(self):
        with patch('jujuci.get_artifacts') as mock:
            main(['-d', '-v', '-b', '1234', 'get', 'foo', '*.tar.gz', 'bar'])
            args, kwargs = mock.call_args
            self.assertEqual(('foo', '1234', '*.tar.gz', 'bar'), args)
            self.assertTrue(kwargs['verbose'])
            self.assertTrue(kwargs['dry_run'])

    def test_get_build_data(self):
        expected_data = make_build_data(1234)
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

    def test_list_artifacts(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data) as mock:
            with patch('jujuci.print_now') as pn_mock:
                list_artifacts('foo', '1234', '*')
        mock.assert_called_once([JENKINS_URL, 'foo', '1234'])
        files = sorted(call[1][0] for call in pn_mock.mock_calls)
        self.assertEqual(
            ['buildvars.bash', 'buildvars.json',
             'juju-core_1.22-alpha1.tar.gz'],
            files)
