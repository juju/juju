from argparse import Namespace
from contextlib import contextmanager
import json
import os
from StringIO import StringIO
import urllib2

from mock import patch

from jujuci import (
    add_artifacts,
    Artifact,
    CERTIFY_UBUNTU_PACKAGES,
    clean_environment,
    Credentials,
    CredentialsMissing,
    find_artifacts,
    get_artifacts,
    get_build_data,
    get_buildvars,
    get_certification_bin,
    get_credentials,
    get_job_data,
    get_juju_bin,
    get_juju_binary,
    get_juju_bin_artifact,
    get_release_package_filename,
    JENKINS_URL,
    JobNamer,
    list_artifacts,
    Namer,
    PackageNamer,
    parse_args,
    main,
    retrieve_artifact,
    setup_workspace,
)
import jujupy
from utility import temp_dir
from tests import (
    FakeHomeTestCase,
    parse_error,
    TestCase,
)


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
                "relativePath": "artifacts/juju-core_1.22-alpha1.tar.gz"
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


def make_job_data():
    return {
        "actions": [
            {
                "parameterDefinitions": [
                    {
                        "defaultParameterValue": {
                            "value": "1.18.1"
                        },
                        "description": "",
                        "name": "old_version",
                        "type": "StringParameterDefinition"
                    }
                ]
            },
            {}
        ],
        "buildable": True,
        "builds": [
            {
                "number": 1510,
                "url": "http://juju-ci.vapour.ws:8080/job/ting/1510/"
            }
        ],
        "color": "red",
        "concurrentBuild": False,
        "description": "",
        "displayName": "ting",
        "downstreamProjects": [],
        "firstBuild": {
            "number": 1,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1/"
        },
        "healthReport": [
            {
                "description": "Build stability: All recent builds failed.",
                "iconUrl": "health-00to19.png",
                "score": 0
            }
        ],
        "inQueue": False,
        "keepDependencies": False,
        "lastBuild": {
            "number": 1510,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1510/"
        },
        "lastCompletedBuild": {
            "number": 1510,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1510/"
        },
        "lastFailedBuild": {
            "number": 1510,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1510/"
        },
        "lastStableBuild": {
            "number": 1392,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1392/"
        },
        "lastSuccessfulBuild": {
            "number": 1392,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1392/"
        },
        "lastUnstableBuild": None,
        "lastUnsuccessfulBuild": {
            "number": 1510,
            "url": "http://juju-ci.vapour.ws:8080/job/ting/1510/"
        },
        "name": "ting",
        "nextBuildNumber": 1511,
        "property": [
            {},
            {
                "parameterDefinitions": [
                    {
                        "defaultParameterValue": {
                            "name": "old_version",
                            "value": "1.18.1"
                        },
                        "description": "",
                        "name": "old_version",
                        "type": "StringParameterDefinition"
                    },
                ]
            }
        ],
        "queueItem": None,
        "upstreamProjects": [],
        "url": "http://juju-ci.vapour.ws:8080/job/ting/"
    }


class JujuCITestCase(FakeHomeTestCase):

    def test_get_credentials(self):
        self.assertEqual(
            get_credentials(Namespace(user='jrandom', password='password1')),
            Credentials('jrandom', 'password1'))

    def test_get_credentials_no_user(self):
        self.assertIs(get_credentials(Namespace()), None)

    def test_get_credentials_no_value(self):
        with self.assertRaisesRegexp(
                CredentialsMissing,
                'Jenkins username and/or password not supplied.'):
            get_credentials(Namespace(user=None, password='password1'))
        with self.assertRaisesRegexp(
                CredentialsMissing,
                'Jenkins username and/or password not supplied.'):
            get_credentials(Namespace(user='jrandom', password=None))

    def test_main_list_options(self):
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.list_artifacts') as mock:
                main(['-d', '-v', 'list', '-b', '1234', 'foo', '*.tar.gz',
                      '--user', 'jrandom', '--password', '1password'])
        args, kwargs = mock.call_args
        self.assertEqual((Credentials('jrandom', '1password'), 'foo',
                         '1234', '*.tar.gz'), args)
        self.assertTrue(kwargs['verbose'])
        self.assertEqual(print_list, ['Done.'])

    def test_main_get_options(self):
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.get_artifacts') as mock:
                main(['-d', '-v',
                      'get', '-a', '-b', '1234', 'foo', '*.tar.gz', 'bar',
                      '--user', 'jrandom', '--password', '1password'])
        args, kwargs = mock.call_args
        self.assertEqual((Credentials('jrandom', '1password'), 'foo',
                         '1234', '*.tar.gz', 'bar'), args)
        self.assertTrue(kwargs['archive'])
        self.assertTrue(kwargs['verbose'])
        self.assertTrue(kwargs['dry_run'])
        self.assertEqual(print_list, ['Done.'])

    def test_main_setup_workspace_options(self):
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.setup_workspace', autospec=True) as mock:
                main(['-d', '-v', 'setup-workspace', '--clean-env', 'bar',
                      './foo'])
        args, kwargs = mock.call_args
        self.assertEqual(('./foo', ), args)
        self.assertEqual('bar', kwargs['env'])
        self.assertTrue(kwargs['dry_run'])
        self.assertTrue(kwargs['verbose'])
        self.assertEqual(print_list, ['Done.'])

    def test_main_get_buildvars(self):
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.get_buildvars', autospec=True,
                       return_value='sample buildvars') as mock:
                main(
                    ['get-build-vars', '--env', 'foo', '--summary',
                     '--revision-build', '--version', '--short-branch',
                     '--short-revision', '--branch', '--revision', '123',
                     '--user', 'jrandom', '--password', '1password'])
        args, kwargs = mock.call_args
        self.assertEqual((Credentials('jrandom', '1password'), '123'), args)
        self.assertEqual('foo', kwargs['env'])
        self.assertTrue(kwargs['summary'])
        self.assertTrue(kwargs['revision_build'])
        self.assertTrue(kwargs['version'])
        self.assertTrue(kwargs['short_revision'])
        self.assertTrue(kwargs['short_branch'])
        self.assertTrue(kwargs['branch'])
        self.assertTrue(kwargs['revision'])
        self.assertEqual(print_list, ['sample buildvars'])

    def test_parse_arg_buildvars_common_options(self):
        args, credentials = parse_args(
            ['get-build-vars', '--env', 'foo', '--summary',
             '--user', 'jrandom', '--password', '1password', '1234'])
        self.assertEqual(Credentials('jrandom', '1password'), credentials)
        self.assertEqual('foo', args.env)
        self.assertTrue(args.summary)
        args, credentials = parse_args(
            ['get-build-vars', '--version',
             '--user', 'jrandom', '--password', '1password', '1234'])
        self.assertTrue(args.version)
        args, credentials = parse_args(
            ['get-build-vars', '--short-branch',
             '--user', 'jrandom', '--password', '1password', '1234'])
        self.assertTrue(args.short_branch)

    def test_parse_arg_buildvars_error(self):
        with parse_error(self) as stderr:
            parse_args(['get-build-vars', '1234'])
        self.assertIn(
            'Expected --summary or one or more of:', stderr.getvalue())

    def test_parse_arg_get_package_name(self):
        with parse_error(self) as stderr:
            parse_args(['get-package-name'])
        self.assertIn('error: too few arguments', stderr.getvalue())
        args = parse_args(['get-package-name', '1.22.1'])
        expected = Namespace(command='get-package-name', version='1.22.1',
                             dry_run=False, verbose=False)
        self.assertEqual(args, (expected, None))

    def test_get_build_data(self):
        expected_data = make_build_data(1234)
        json_io = StringIO(json.dumps(expected_data))
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            build_data = get_build_data(
                'http://foo:8080', Credentials('jrandom', '1password'), 'bar',
                '1234')
        self.assertEqual(1, mock.call_count)
        request = mock.mock_calls[0][1][0]
        self.assertRequest(
            request, 'http://foo:8080/job/bar/1234/api/json',
            {'Authorization': 'Basic anJhbmRvbToxcGFzc3dvcmQ='})
        self.assertEqual(expected_data, build_data)

    def test_get_job_data(self):
        expected_data = make_job_data()
        json_io = StringIO(json.dumps(expected_data))
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            build_data = get_job_data(
                'http://foo:8080', Credentials('jrandom', '1password'), 'ting')
        self.assertEqual(1, mock.call_count)
        request = mock.mock_calls[0][1][0]
        self.assertRequest(
            request, 'http://foo:8080/job/ting/api/json',
            {'Authorization': 'Basic anJhbmRvbToxcGFzc3dvcmQ='})
        self.assertEqual(expected_data, build_data)

    def assertRequest(self, request, url, headers):
        self.assertIs(request.__class__, urllib2.Request)
        self.assertEqual(request.get_full_url(), url)
        self.assertEqual(request.headers, headers)

    def test_get_build_data_with_default_build(self):
        expected_data = make_build_data()
        json_io = StringIO(json.dumps(expected_data))
        with patch('urllib2.urlopen', return_value=json_io) as mock:
            get_build_data(
                'http://foo:8080', Credentials('jrandom', '1password'), 'bar')
        self.assertEqual(mock.call_count, 1)
        self.assertRequest(
            mock.mock_calls[0][1][0],
            'http://foo:8080/job/bar/lastSuccessfulBuild/api/json',
            {'Authorization': 'Basic anJhbmRvbToxcGFzc3dvcmQ='})

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
            'artifact/artifacts/juju-core_1.22-alpha1.tar.gz',
            artifact.location)

    def test_list_artifacts(self):
        credentials = Credentials('jrandom', '1password')
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data) as mock:
            with patch('jujuci.print_now') as pn_mock:
                list_artifacts(credentials, 'foo', '1234', '*')
        mock.assert_called_once_with(JENKINS_URL, credentials, 'foo', '1234')
        files = sorted(call[1][0] for call in pn_mock.mock_calls)
        self.assertEqual(
            ['buildvars.bash', 'buildvars.json',
             'juju-core_1.22-alpha1.tar.gz'],
            files)

    def test_list_artifacts_verbose(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('jujuci.print_now') as pn_mock:
                list_artifacts(
                    Credentials('jrandom', '1password'), 'foo', '1234',
                    '*.bash', verbose=True)
        files = sorted(call[1][0] for call in pn_mock.mock_calls)
        self.assertEqual(
            ['http://juju-ci.vapour.ws:8080/job/build-revision/1234/artifact/'
             'buildvars.bash'],
            files)

    def test_retrieve_artifact(self):
        location = (
            'http://juju-ci.vapour.ws:8080/job/build-revision/1234/artifact/'
            'buildvars.bash')
        local_path = '%s/buildvars.bash' % os.path.abspath('./')
        with patch('urllib.urlretrieve') as uo_mock:
            retrieve_artifact(
                Credentials('jrandom', '1password'), location, local_path)
        self.assertEqual(1, uo_mock.call_count)
        args, kwargs = uo_mock.call_args
        auth_location = location.replace('http://',
                                         'http://jrandom:1password@')
        self.assertEqual((auth_location, local_path), args)

    def test_get_juju_bin_artifact(self):
        package_namer = PackageNamer('foo', 'bar', 'baz')
        bin_filename = 'juju-core_1.27.32-0ubuntu1~bar.1~juju1_foo.deb'
        artifact = get_juju_bin_artifact(package_namer, '1.27.32', {
            'url': 'http://asdf/',
            'artifacts': [{
                'relativePath': 'foo',
                'fileName': bin_filename,
                }],
            })
        self.assertEqual(artifact.location, 'http://asdf/artifact/foo')
        self.assertEqual(artifact.file_name, bin_filename)

    def test_get_release_package_filename(self):
        package_namer = PackageNamer('foo', 'bar', 'baz')
        credentials = ('jrandom', 'password1')
        publish_build_data = {'actions': [], }
        build_revision_data = json.dumps({
            'artifacts': [{
                'fileName': 'buildvars.json',
                'relativePath': 'buildvars.json'
                }],
            'url': 'http://foo/'})

        def mock_urlopen(request):
            if request.get_full_url() == 'http://foo/artifact/buildvars.json':
                data = json.dumps({'version': '1.42'})
            else:
                data = build_revision_data
            return StringIO(data)

        with patch('urllib2.urlopen', autospec=True,
                   side_effect=mock_urlopen):
            with patch.object(PackageNamer, 'factory',
                              return_value=package_namer):
                file_name = get_release_package_filename(
                    credentials, publish_build_data)
        self.assertEqual(file_name, package_namer.get_release_package('1.42'))

    def test_get_juju_bin(self):
        build_data = {
            'url': 'http://foo/',
            'artifacts': [{
                'fileName': 'steve',
                'relativePath': 'baz',
                }]
            }
        credentials = Credentials('jrandom', 'password1')
        job_namer = JobNamer('a64', '42.34', 'wacky')

        with self.get_juju_binary_mocks() as (workspace, cc_mock, uo_mock):
            with patch('jujuci.get_build_data', return_value=build_data,
                       autospec=True) as gbd_mock:
                with patch(
                        'jujuci.get_release_package_filename',
                        return_value='steve', autospec=True) as grpf_mock:
                    with patch.object(
                            JobNamer, 'factory', spec=JobNamer.factory,
                            return_value=job_namer) as jnf_mock:
                        bin_loc = get_juju_bin(credentials, workspace)
        self.assertEqual(bin_loc, os.path.join(
            workspace, 'extracted-bin', 'subdir', 'sub-subdir', 'juju'))
        grpf_mock.assert_called_once_with(credentials, build_data)
        gbd_mock.assert_called_once_with(
            JENKINS_URL, credentials, 'build-binary-wacky-a64', 'lastBuild')
        self.assertEqual(1, jnf_mock.call_count)

    def test_get_certification_bin(self):
        package_namer = PackageNamer('foo', 'bar', 'baz')
        build_data = {
            'url': 'http://foo/',
            'artifacts': [{
                'fileName': 'juju-core_36.1~0ubuntu1~bar.1_foo.deb',
                'relativePath': 'baz',
                }]
            }
        credentials = Credentials('jrandom', 'password1')

        with self.get_juju_binary_mocks() as (workspace, ur_mock, cc_mock):
            with patch('jujuci.get_build_data', return_value=build_data,
                       autospec=True) as gbd_mock:
                with patch.object(PackageNamer, 'factory',
                                  return_value=package_namer):
                    bin_loc = get_certification_bin(credentials,
                                                    '36.1~0ubuntu1',
                                                    workspace)
        self.assertEqual(bin_loc, os.path.join(
            workspace, 'extracted-bin', 'subdir', 'sub-subdir', 'juju'))
        ur_mock.assert_called_once_with(
            'http://jrandom:password1@foo/artifact/baz',
            os.path.join(workspace, 'juju-core_36.1~0ubuntu1~bar.1_foo.deb'))
        gbd_mock.assert_called_once_with(
            JENKINS_URL, credentials, CERTIFY_UBUNTU_PACKAGES, 'lastBuild')

    @contextmanager
    def get_juju_binary_mocks(self):
        def mock_extract_deb(args):
            parent = os.path.join(args[3], 'subdir', 'sub-subdir')
            os.makedirs(parent)
            with open(os.path.join(parent, 'juju'), 'w') as f:
                f.write('foo')

        with temp_dir() as workspace:
            with patch('urllib.urlretrieve') as ur_mock:
                with patch('subprocess.check_call',
                           side_effect=mock_extract_deb) as cc_mock:
                    yield workspace, ur_mock, cc_mock

    def test_get_juju_binary(self):
        build_data = {
            'url': 'http://foo/',
            'artifacts': [{
                'fileName': 'steve',
                'relativePath': 'baz',
                }]
            }
        credentials = Credentials('jrandom', 'password1')
        with self.get_juju_binary_mocks() as (workspace, ur_mock, cc_mock):
            bin_loc = get_juju_binary(credentials, 'steve', build_data,
                                      workspace)
        target_path = os.path.join(workspace, 'steve')
        ur_mock.assert_called_once_with(
            'http://jrandom:password1@foo/artifact/baz', target_path)
        out_dir = os.path.join(workspace, 'extracted-bin')
        cc_mock.assert_called_once_with(['dpkg', '-x', target_path, out_dir])
        self.assertEqual(
            bin_loc, os.path.join(workspace, 'extracted-bin', 'subdir',
                                  'sub-subdir', 'juju'))

    def test_get_artifacts(self):
        build_data = make_build_data(1234)
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.get_build_data', return_value=build_data):
                with patch('urllib.urlretrieve') as uo_mock:
                    found = get_artifacts(
                        Credentials('jrandom', '1password'), 'foo', '1234',
                        '*.bash', './', verbose=True)
        location = (
            'http://juju-ci.vapour.ws:8080/job/build-revision/1234/artifact/'
            'buildvars.bash')
        buildvar_artifact = Artifact('buildvars.bash', location)
        self.assertEqual([buildvar_artifact], found)
        self.assertEqual(1, uo_mock.call_count)
        args, kwargs = uo_mock.call_args
        local_path = '%s/buildvars.bash' % os.path.abspath('./')
        auth_location = location.replace('http://',
                                         'http://jrandom:1password@')
        self.assertEqual((auth_location, local_path), args)
        self.assertEqual(
            print_list,
            ['Retrieving %s => %s' % (location, local_path)]
        )

    def test_get_artifacts_with_dry_run(self):
        build_data = make_build_data(1234)
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.get_build_data', return_value=build_data):
                with patch('urllib.urlretrieve') as uo_mock:
                    get_artifacts(
                        Credentials('jrandom', '1password'), 'foo', '1234',
                        '*.bash', './', dry_run=True)
        self.assertEqual(0, uo_mock.call_count)
        self.assertEqual(print_list, ['buildvars.bash'])

    def test_get_artifacts_with_archive(self):
        build_data = make_build_data(1234)
        print_list = []
        with temp_dir() as base_dir:
            path = os.path.join(base_dir, 'foo')
            os.mkdir(path)
            old_file_path = os.path.join(path, 'old_file.txt')
            with open(old_file_path, 'w') as old_file:
                old_file.write('old')
            with patch('jujuci.print_now', side_effect=print_list.append):
                with patch('jujuci.get_build_data', return_value=build_data):
                    with patch('urllib.urlretrieve'):
                        get_artifacts(
                            Credentials('jrandom', '1password'), 'foo', '1234',
                            '*.bash', path, archive=True)
            self.assertFalse(os.path.isfile(old_file_path))
            self.assertTrue(os.path.isdir(path))
        self.assertEqual(print_list, ['buildvars.bash'])

    def test_get_artifacts_with_archive_error(self):
        build_data = make_build_data(1234)
        with patch('jujuci.get_build_data', return_value=build_data):
            with patch('urllib.urlretrieve'):
                with self.assertRaises(ValueError):
                    get_artifacts(
                        Credentials('jrandom', '1password'), 'foo', '1234',
                        '*.bash', '/foo-bar-baz', archive=True)

    def test_setup_workspace(self):
        with temp_dir() as base_dir:
            workspace_dir = os.path.join(base_dir, 'workspace')
            foo_dir = os.path.join(workspace_dir, 'foo')
            os.makedirs(foo_dir)
            with open(os.path.join(workspace_dir, 'old.txt'), 'w') as of:
                of.write('old')
            print_list = []
            with patch('jujuci.print_now', side_effect=print_list.append):
                setup_workspace(workspace_dir, dry_run=False, verbose=False)
            self.assertEqual(['artifacts'], os.listdir(workspace_dir))
            artifacts_dir = os.path.join(workspace_dir, 'artifacts')
            self.assertEqual(['empty'], os.listdir(artifacts_dir))
            self.assertEqual(
                print_list,
                ['Removing old.txt', 'Removing foo', 'Creating artifacts dir.']
            )

    def test_setup_workspace_with_env(self):
        with temp_dir() as base_dir:
            workspace_dir = os.path.join(base_dir, 'workspace')
            os.makedirs(workspace_dir)
            print_list = []
            with patch('jujuci.print_now', side_effect=print_list.append):
                with patch('jujuci.clean_environment', autospec=True) as mock:
                    setup_workspace(
                        workspace_dir, env='foo', dry_run=False, verbose=False)
            mock.assert_called_once_with('foo', verbose=False)
            self.assertEqual(print_list, ['Creating artifacts dir.'])

    def test_clean_environment_not_dirty(self):
        config = {'environments': {'local': {'type': 'local'}}}
        with jujupy._temp_env(config, set_home=True):
            with patch('jujuci.destroy_environment', autospec=True) as mock_de:
                with patch.object(jujupy.ModelClient, 'get_version'):
                    with patch('jujupy.get_client_class') as gcc_mock:
                        gcc_mock.return_value = jujupy.ModelClient
                        dirty = clean_environment('foo', verbose=False)
        self.assertFalse(dirty)
        self.assertEqual(0, mock_de.call_count)

    def test_clean_environment_dirty(self):
        config = {'environments': {'foo': {'type': 'local'}}}
        with jujupy._temp_env(config, set_home=True):
            with patch('jujuci.destroy_environment', autospec=True) as mock_de:
                with patch.object(jujupy.ModelClient, 'get_version'):
                    with patch('jujupy.get_client_class') as gcc_mock:
                        factory = gcc_mock.return_value
                        factory.return_value = jujupy.ModelClient(
                            None, None, None)
                        with patch.object(jujupy.ModelClient,
                                          'get_full_path'):
                            with patch.object(jujupy.JujuData, 'load_yaml'):
                                dirty = clean_environment('foo', verbose=False)
        self.assertTrue(dirty)
        self.assertEqual(1, mock_de.call_count)
        args, kwargs = mock_de.call_args
        self.assertIsInstance(args[0], jujupy.ModelClient)
        self.assertEqual('foo', args[1])

    def test_add_artifacts_simple(self):
        with temp_dir() as workspace_dir:
            artifacts_dir = os.path.join(workspace_dir, 'artifacts')
            os.mkdir(artifacts_dir)
            for name in ['foo.tar.gz', 'bar.json', 'baz.txt',
                         'juju-core_1.2.3.tar.gz', 'juju-core-1.2.3.deb',
                         'buildvars.bash', 'buildvars.json']:
                with open(os.path.join(workspace_dir, name), 'w') as nf:
                    nf.write(name)
            globs = ['buildvars.*', 'juju-core_*.tar.gz', 'juju-*.deb']
            add_artifacts(
                workspace_dir, globs, dry_run=False, verbose=False)
            artifacts = sorted(os.listdir(artifacts_dir))
            expected = [
                'buildvars.bash', 'buildvars.json',
                'juju-core-1.2.3.deb', 'juju-core_1.2.3.tar.gz']
            self.assertEqual(expected, artifacts)

    def test_add_artifacts_deep(self):
        with temp_dir() as workspace_dir:
            artifacts_dir = os.path.join(workspace_dir, 'artifacts')
            os.mkdir(artifacts_dir)
            sub_dir = os.path.join(workspace_dir, 'sub_dir')
            os.mkdir(sub_dir)
            for name in ['foo.tar.gz', 'juju-core-1.2.3.deb']:
                with open(os.path.join(sub_dir, name), 'w') as nf:
                    nf.write(name)
            add_artifacts(
                workspace_dir, ['sub_dir/*.deb'], dry_run=False, verbose=False)
            artifacts = os.listdir(artifacts_dir)
            self.assertEqual(['juju-core-1.2.3.deb'], artifacts)

    def test_get_buildvars(self):
        buildvars = {
            'revision_id': '1234567abcdef',
            'version': '1.22.4',
            'revision_build': '1234',
            'branch': 'gitbranch:1.22:github.com/juju/juju',
            }
        credentials = Credentials('bar', 'baz')
        with patch('jujuci.retrieve_buildvars', autospec=True,
                   return_value=buildvars) as rb_mock:
            # The summary case for deploy and upgrade jobs.
            text = get_buildvars(
                credentials, 1234, env='foo',
                summary=True, revision_build=False, version=False,
                short_branch=False, short_revision=False,
                branch=False, revision=False)
            rb_mock.assert_called_once_with(credentials, 1234)
            self.assertEqual(
                'Testing gitbranch:1.22:github.com/juju/juju 1234567 '
                'on foo for 1234',
                text)
            # The version case used to skip jobs testing old versions.
            text = get_buildvars(
                credentials, 1234,
                summary=False, revision_build=True, version=True,
                branch=True, short_branch=True,
                revision=True, short_revision=True)
            self.assertEqual(
                '1234 1.22.4 1.22 1234567 '
                'gitbranch:1.22:github.com/juju/juju 1234567abcdef',
                text)


class TestNamer(TestCase):

    def test_factory(self):
        with patch('subprocess.check_output', return_value=' amd42 \n'):
            with patch('jujuci.get_distro_information',
                       return_value={'RELEASE': '42.42', 'CODENAME': 'wacky'}):
                namer = Namer.factory()
        self.assertIs(type(namer), Namer)
        self.assertEqual(namer.arch, 'amd42')
        self.assertEqual(namer.distro_release, '42.42')
        self.assertEqual(namer.distro_series, 'wacky')


class TestPackageNamer(TestNamer):

    def test_get_release_package(self):
        package_namer = PackageNamer('amd42', '42.34', 'wacky')
        self.assertEqual(
            package_namer.get_release_package('27.6'),
            'juju-core_27.6-0ubuntu1~42.34.1~juju1_amd42.deb')

    def test_get_certification_package(self):
        package_namer = PackageNamer('amd42', '42.34', 'wacky')
        self.assertEqual(
            package_namer.get_certification_package('27.6~0ubuntu1'),
            'juju-core_27.6~0ubuntu1~42.34.1_amd42.deb')


class TestJobNamer(TestNamer):

    def test_get_build_binary(self):
        self.assertEqual(
            JobNamer('ppc64el', '42.34', 'wacky').get_build_binary_job(),
            'build-binary-wacky-ppc64el')
