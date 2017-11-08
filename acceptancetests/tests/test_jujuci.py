from argparse import Namespace
from contextlib import contextmanager
import json
import os
from StringIO import StringIO
import urllib2

from mock import patch

from jujuci import (
    acquire_binary,
    add_artifacts,
    Credentials,
    CredentialsMissing,
    find_artifacts,
    get_build_data,
    get_credentials,
    get_job_data,
    JENKINS_URL,
    JobNamer,
    list_artifacts,
    Namer,
    PackageNamer,
    main,
    setup_workspace,
)
from utility import temp_dir
from tests import (
    FakeHomeTestCase,
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
        "url": "http://jenkins:8080/job/build-revision/%s/" % number
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
                "url": "http://jenkins:8080/job/ting/1510/"
            }
        ],
        "color": "red",
        "concurrentBuild": False,
        "description": "",
        "displayName": "ting",
        "downstreamProjects": [],
        "firstBuild": {
            "number": 1,
            "url": "http://jenkins:8080/job/ting/1/"
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
            "url": "http://jenkins:8080/job/ting/1510/"
        },
        "lastCompletedBuild": {
            "number": 1510,
            "url": "http://jenkins:8080/job/ting/1510/"
        },
        "lastFailedBuild": {
            "number": 1510,
            "url": "http://jenkins:8080/job/ting/1510/"
        },
        "lastStableBuild": {
            "number": 1392,
            "url": "http://jenkins:8080/job/ting/1392/"
        },
        "lastSuccessfulBuild": {
            "number": 1392,
            "url": "http://jenkins:8080/job/ting/1392/"
        },
        "lastUnstableBuild": None,
        "lastUnsuccessfulBuild": {
            "number": 1510,
            "url": "http://jenkins:8080/job/ting/1510/"
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
        "url": "http://jenkins:8080/job/ting/"
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

    def test_main_setup_workspace_options(self):
        print_list = []
        with patch('jujuci.print_now', side_effect=print_list.append):
            with patch('jujuci.setup_workspace', autospec=True) as mock:
                main(['-d', '-v', 'setup-workspace', './foo'])
        args, kwargs = mock.call_args
        self.assertEqual(('./foo', ), args)
        self.assertTrue(kwargs['dry_run'])
        self.assertTrue(kwargs['verbose'])
        self.assertEqual(print_list, ['Done.'])

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
            'http://jenkins:8080/job/build-revision/2112/'
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


class TestAcquireBinary(TestCase):

    def fake_call(self, args, parent='bin'):
        dest = args[3]
        parent_path = os.path.join(dest, 'bar', parent)
        os.makedirs(parent_path)
        bin_path = os.path.join(parent_path, 'juju')
        open(bin_path, 'w').close()

    def fake_call_completion(self, args):
        return self.fake_call(args, parent='bash_completion.d')

    def test_avoids_bash_completion(self):
        with temp_dir() as workspace:
            with patch('subprocess.check_call',
                       side_effect=self.fake_call_completion):
                result = acquire_binary(None, workspace)
        self.assertIs(None, result)
