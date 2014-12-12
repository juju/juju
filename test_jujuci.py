import json
from mock import patch
import os
from StringIO import StringIO
from unittest import TestCase

from jujuci import (
    get_build_data,
    JENKINS_URL,
    get_artifacts,
    list_artifacts,
    find_artifacts,
    main,
    setup_workspace,
)
from utility import temp_dir


def make_build_data(number='lastSuccessfulBuild'):
    if number == 'lastSuccessfulBuild':
        number = 2112
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
            main(['-d', '-v', 'list', '-b', '1234', 'foo', '*.tar.gz'])
            args, kwargs = mock.call_args
            self.assertEqual(('foo', '1234', '*.tar.gz'), args)
            self.assertTrue(kwargs['verbose'])

    def test_main_get_options(self):
        with patch('jujuci.get_artifacts') as mock:
            main(['-d', '-v',
                  'get', '-a', '-b', '1234', 'foo', '*.tar.gz', 'bar'])
            args, kwargs = mock.call_args
            self.assertEqual(('foo', '1234', '*.tar.gz', 'bar'), args)
            self.assertTrue(kwargs['archive'])
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

    def test_find_artifacts_all(self):
        expected_data = make_build_data()
        artifacts = find_artifacts(expected_data)
        self.assertEqual(
            ['buildvars.bash', 'buildvars.json',
             'juju-core_1.22-alpha1.tar.gz'],
            sorted(a.file_name for a in artifacts))

    def test_find_artifacts_glob_tarball(self):
        expected_data = make_build_data()
        artifacts = find_artifacts(expected_data, '*.tar.gz')
        artifact = artifacts[0]
        self.assertEqual('juju-core_1.22-alpha1.tar.gz', artifact.file_name)
        self.assertEqual(
            'http://juju-ci.vapour.ws:8080/job/build-revision/2112/'
            'artifact/juju-core_1.22-alpha1.tar.gz',
            artifact.location)

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

    def test_list_artifacts_verbose(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('jujuci.print_now') as pn_mock:
                list_artifacts('foo', '1234', '*.bash', verbose=True)
        files = sorted(call[1][0] for call in pn_mock.mock_calls)
        self.assertEqual(
            ['http://juju-ci.vapour.ws:8080/job/build-revision/1234/artifact/'
             'buildvars.bash'],
            files)

    def test_get_artifacts(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('urllib.URLopener.retrieve') as uo_mock:
                with patch('jujuci.print_now') as pn_mock:
                    get_artifacts(
                        'foo', '1234', '*.bash', './', verbose=True)
        self.assertEqual(1, uo_mock.call_count)
        args, kwargs = uo_mock.call_args
        location = (
            'http://juju-ci.vapour.ws:8080/job/build-revision/1234/artifact/'
            'buildvars.bash')
        local_path = '%s/buildvars.bash' % os.path.abspath('./')
        self.assertEqual((location, local_path), args)
        messages = sorted(call[1][0] for call in pn_mock.mock_calls)
        self.assertEqual(1, len(messages))
        message = messages[0]
        self.assertEqual(
            'Retrieving %s => %s' % (location, local_path),
            message)

    def test_get_artifacts_with_dry_run(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('urllib.URLopener.retrieve') as uo_mock:
                get_artifacts(
                    'foo', '1234', '*.bash', './', dry_run=True)
        self.assertEqual(0, uo_mock.call_count)

    def test_get_artifacts_with_archive(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('urllib.URLopener.retrieve'):
                with temp_dir() as base_dir:
                    path = os.path.join(base_dir, 'foo')
                    os.mkdir(path)
                    old_file_path = os.path.join(path, 'old_file.txt')
                    with open(old_file_path, 'w') as old_file:
                        old_file.write('old')
                    get_artifacts(
                        'foo', '1234', '*.bash', path, archive=True)
                    self.assertFalse(os.path.isfile(old_file_path))
                    self.assertTrue(os.path.isdir(path))

    def test_get_artifacts_with_archive_error(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('urllib.URLopener.retrieve'):
                with self.assertRaises(ValueError):
                    get_artifacts(
                        'foo', '1234', '*.bash', '/foo-bar-baz', archive=True)

    def test_setup_workspace(self):
        with temp_dir() as base_dir:
            workspace_dir = os.path.join(base_dir, 'workspace')
            foo_dir = os.path.join(workspace_dir, 'foo')
            os.makedirs(foo_dir)
            with open(os.path.join(workspace_dir, 'old.txt'), 'w') as of:
                of.write('old')
            setup_workspace(workspace_dir, dry_run=False, verbose=False)
            self.assertEqual(['artifacts'], os.listdir(workspace_dir))
            artifacts_dir = os.path.join(workspace_dir, 'artifacts')
            self.assertEqual(['empty'], os.listdir(artifacts_dir))
