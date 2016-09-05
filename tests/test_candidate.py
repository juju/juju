import json
from mock import patch
import os
from subprocess import CalledProcessError
from unittest import TestCase

from candidate import (
    download_candidate_files,
    extract_candidates,
    find_publish_revision_number,
    get_artifact_dirs,
    get_package,
    get_scripts,
    parse_args,
    prepare_dir,
    publish,
    publish_candidates,
)
from jujuci import Credentials
from utility import temp_dir


class CandidateTestCase(TestCase):

    def test_parse_args_download(self):
        args, credentials = parse_args([
            'download', 'branch', 'path', '--user', 'jrandom', '--password',
            'password1'])
        self.assertEqual(credentials, Credentials('jrandom', 'password1'))

    @staticmethod
    def make_publish_revision_build_data(*args, **kwargs):
        if kwargs['build'] == 'lastSuccessfulBuild':
            number = 1235
        else:
            number = kwargs['build']
        return {
            'number': number,
            'actions': [{'parameters': [{
                'name': 'revision_build', 'value': str(number)}
            ]}],
        }

    def test_find_publish_revision_number(self):
        credentials = Credentials('jrandom', 'password1')
        with patch('candidate.get_build_data',
                   side_effect=self.make_publish_revision_build_data,
                   autospec=True) as mock:
            found_number = find_publish_revision_number(credentials, 1234,
                                                        limit=2)
        self.assertEqual(1234, found_number)
        self.assertEqual(2, mock.call_count)
        output, args, kwargs = mock.mock_calls[0]
        self.assertEqual(
            ('http://juju-ci.vapour.ws:8080', credentials, 'publish-revision'),
            args)
        self.assertEqual('lastSuccessfulBuild', kwargs['build'])
        output, args, kwargs = mock.mock_calls[1]
        self.assertEqual(
            ('http://juju-ci.vapour.ws:8080', credentials, 'publish-revision'),
            args)
        self.assertEqual(1234, kwargs['build'])

    def test_find_publish_revision_number_no_match(self):
        credentials = Credentials('jrandom', 'password1')
        with patch('candidate.get_build_data',
                   side_effect=self.make_publish_revision_build_data) as mock:
            found_number = find_publish_revision_number(credentials, 1,
                                                        limit=2)
        self.assertEqual(None, found_number)
        self.assertEqual(2, mock.call_count)

    def test_find_publish_revision_number_no_build_data(self):
        credentials = Credentials('jrandom', 'password1')
        with patch('candidate.get_build_data', return_value=None) as mock:
            found_number = find_publish_revision_number(credentials, 1,
                                                        limit=5)
        self.assertEqual(None, found_number)
        self.assertEqual(1, mock.call_count)

    def test_prepare_dir_clean_existing(self):
        with temp_dir() as base_dir:
            candidate_dir_path = os.path.join(base_dir, 'candidate', 'master')
            os.makedirs(candidate_dir_path)
            os.makedirs(os.path.join(candidate_dir_path, 'subdir'))
            candidate_file_path = os.path.join(candidate_dir_path, 'vars.json')
            with open(candidate_file_path, 'w') as cf:
                cf.write('data')
            prepare_dir(candidate_dir_path)
            self.assertTrue(os.path.isdir(candidate_dir_path))
            self.assertEqual([], os.listdir(candidate_dir_path))

    def test_prepare_dir_create_new(self):
        with temp_dir() as base_dir:
            os.makedirs(os.path.join(base_dir, 'candidate'))
            candidate_dir_path = os.path.join(base_dir, 'candidate', 'master')
            prepare_dir(candidate_dir_path)
            self.assertTrue(os.path.isdir(candidate_dir_path))
            self.assertEqual([], os.listdir(candidate_dir_path))

    @patch('sys.stdout')
    def test_prepare_dir_with_dry_run(self, so_mock):
        with temp_dir() as base_dir:
            os.makedirs(os.path.join(base_dir, 'candidate'))
            candidate_dir_path = os.path.join(base_dir, 'candidate', 'master')
            prepare_dir(candidate_dir_path, dry_run=True, verbose=True)
            self.assertFalse(os.path.isdir(candidate_dir_path))

    def test_download_candidate_files(self):
        credentials = Credentials('jrandom', 'password1')
        with patch('candidate.prepare_dir') as pd_mock:
            with patch('candidate.find_publish_revision_number',
                       return_value=5678, autospec=True) as fprn_mock:
                with patch(
                        'candidate.get_artifacts', autospec=True) as ga_mock:
                    download_candidate_files(
                        credentials, '1.24.4', '~/candidate', '1234')
        args, kwargs = pd_mock.call_args
        self.assertEqual(('~/candidate/1.24.4-artifacts', False, False), args)
        args, kwargs = fprn_mock.call_args
        self.assertEqual((Credentials('jrandom', 'password1'), '1234'), args)
        self.assertEqual(2, ga_mock.call_count)
        output, args, kwargs = ga_mock.mock_calls[0]
        self.assertEqual(
            (credentials, 'build-revision', '1234',
             'buildvars.json', '~/candidate/1.24.4-artifacts'), args)
        options = {'verbose': False, 'dry_run': False}
        self.assertEqual(options, kwargs)
        output, args, kwargs = ga_mock.mock_calls[1]
        self.assertEqual(
            (credentials, 'publish-revision', 5678, 'juju-core*',
             '~/candidate/1.24.4-artifacts'),
            args)
        self.assertEqual(options, kwargs)

    def test_download_candidate_files_with_dry_run(self):
        credentials = Credentials('jrandom', 'password1')
        with patch('candidate.prepare_dir') as pd_mock:
            with patch('candidate.find_publish_revision_number',
                       return_value=5678, autospec=True):
                with patch(
                        'candidate.get_artifacts', autospec=True) as ga_mock:
                    download_candidate_files(
                        credentials, '1.24.4', '~/candidate', '1234',
                        dry_run=True, verbose=True)
        args, kwargs = pd_mock.call_args
        self.assertEqual(('~/candidate/1.24.4-artifacts', True, True), args)
        args, kwargs = ga_mock.call_args
        options = {'verbose': True, 'dry_run': True}
        self.assertEqual(options, kwargs)

    def test_get_artifact_dirs(self):
        with temp_dir() as base_dir:
            os.makedirs(os.path.join(base_dir, 'master'))
            os.makedirs(os.path.join(base_dir, 'master-artifacts'))
            os.makedirs(os.path.join(base_dir, '1.21-artifacts'))
            os.makedirs(os.path.join(base_dir, 'subdir'))
            dirs = get_artifact_dirs(base_dir)
        self.assertEqual(
            ['1.21-artifacts', 'master-artifacts'], sorted(dirs))

    def test_get_package(self):
        def subprocessor(*args, **kwargs):
            command = args[0]
            if 'lsb_release' in command:
                return '14.04'
            elif 'dpkg' in command:
                return 'amd64'

        with patch('subprocess.check_output',
                   side_effect=subprocessor) as co_mock:
            name = get_package('foo', '1.2.3')
        self.assertEqual(
            'foo/juju-core_1.2.3-0ubuntu1~14.04.1~juju1_amd64.deb', name)
        self.assertEqual(2, co_mock.call_count)
        output, args, kwargs = co_mock.mock_calls[0]
        self.assertEqual((['lsb_release', '-sr'], ), args)
        output, args, kwargs = co_mock.mock_calls[1]
        self.assertEqual((['dpkg', '--print-architecture'], ), args)

    def setup_extract_candidates(self, dry_run=False, verbose=False):
        version = '1.2.3'
        with temp_dir() as base_dir:
            artifacts_dir_path = os.path.join(base_dir, 'master-artifacts')
            os.makedirs(artifacts_dir_path)
            buildvars_path = os.path.join(artifacts_dir_path, 'buildvars.json')
            with open(buildvars_path, 'w') as bv:
                json.dump(dict(version=version), bv)
            os.utime(buildvars_path, (1426186437, 1426186437))
            master_dir_path = os.path.join(base_dir, version)
            os.makedirs(master_dir_path)
            package_path = os.path.join(master_dir_path, 'foo.deb')
            with patch('candidate.prepare_dir') as pd_mock:
                with patch('candidate.get_package',
                           return_value=package_path) as gp_mock:
                    with patch('subprocess.check_call') as cc_mock:
                        extract_candidates(
                            base_dir, dry_run=dry_run, verbose=verbose)
            copied_path = os.path.join(master_dir_path, 'buildvars.json')
            if not dry_run:
                self.assertTrue(os.path.isfile(copied_path))
                self.assertEqual(os.stat(copied_path).st_mtime, 1426186437)
            else:
                self.assertFalse(os.path.isfile(copied_path))
            return (pd_mock, gp_mock, cc_mock,
                    artifacts_dir_path, buildvars_path, master_dir_path,
                    package_path)

    def test_extract_candidates(self):
        results = self.setup_extract_candidates(dry_run=False, verbose=False)
        pd_mock, gp_mock, cc_mock = results[0:3]
        artifacts_dir, buildvars_path, master_dir, package_path = results[3:7]
        args, kwargs = pd_mock.call_args
        self.assertEqual((master_dir, False, False), args)
        args, kwargs = gp_mock.call_args
        self.assertEqual((artifacts_dir, '1.2.3'), args)
        args, kwargs = cc_mock.call_args
        self.assertEqual(
            (['dpkg', '-x', package_path, master_dir], ), args)

    @patch('sys.stdout')
    def test_extract_candidates_dry_run(self, so_mock):
        results = self.setup_extract_candidates(dry_run=True, verbose=True)
        pd_mock, gp_mock, cc_mock = results[0:3]
        artifacts_dir, buildvars_path, master_dir, package_path = results[3:7]
        args, kwargs = pd_mock.call_args
        self.assertEqual((master_dir, True, True), args)
        args, kwargs = gp_mock.call_args
        self.assertEqual((artifacts_dir, '1.2.3'), args)
        self.assertEqual(0, cc_mock.call_count)

    def test_get_scripts(self):
        assemble_script, publish_script = get_scripts()
        self.assertEqual('assemble-streams.bash', assemble_script)
        self.assertEqual('publish-public-tools.bash', publish_script)
        assemble_script, publish_script = get_scripts('../foo/')
        self.assertEqual('../foo/assemble-streams.bash', assemble_script)
        self.assertEqual('../foo/publish-public-tools.bash', publish_script)

    def test_publish_candidates_copies_buildvars(self):
        with temp_dir() as base_dir:
            artifacts_dir_path = os.path.join(base_dir, 'master-artifacts')
            os.makedirs(artifacts_dir_path)
            package_path = os.path.join(
                artifacts_dir_path,
                'buildvars.json')
            with open(package_path, 'w') as pf:
                pf.write('testing')
            with patch('subprocess.check_output') as co_mock:
                with patch('shutil.copyfile') as cf_mock:
                    with patch('candidate.run_command'):
                        with patch('candidate.extract_candidates'):
                            publish_candidates(base_dir, '~/streams',
                                               juju_release_tools='../')
        self.assertEqual(2, cf_mock.call_count)
        output, args, kwargs = cf_mock.mock_calls[0]
        self.assertEqual(package_path, args[0])
        # Convert the temp_dir name to something predictable.
        actual_path = args[1].replace(args[1][5:14], 'foo')
        expected_path = os.path.join(
            '/tmp', 'foo', 'buildvars.json')
        self.assertEqual(expected_path, actual_path)
        output, args, kwargs = cf_mock.mock_calls[1]
        actual_path = args[1].replace(args[1][5:14], 'new')
        # Convert the timestamp to something predictable
        actual_path = actual_path.replace(actual_path[16:35], 'timestamp')
        expected_path = os.path.join(
            '/tmp', 'new', 'weekly', 'timestamp', 'master', 'buildvars.json')
        self.assertEqual(expected_path, actual_path)
        # Check that s3cmd is called.
        self.assertEqual(1, co_mock.call_count)
        output, args, kwargs = co_mock.mock_calls[0]
        actual_path = args[0][5].replace(args[0][5][5:14], 'bar')
        actual_path = actual_path.replace(actual_path[16:35], 'timestamp')
        expected_path = os.path.join(
            '/tmp', 'bar', 'weekly', 'timestamp')
        self.assertEqual(expected_path, actual_path)

    def test_publish_candidates(self):
        with temp_dir() as base_dir:
            artifacts_dir_path = os.path.join(base_dir, 'master-artifacts')
            os.makedirs(artifacts_dir_path)
            package_path = os.path.join(
                artifacts_dir_path,
                'juju-core_1.2.3-0ubuntu1~14.04.1~juju1_amd64.deb')
            with open(package_path, 'w') as pf:
                pf.write('testing')
            with patch('subprocess.check_output'):
                with patch('shutil.copyfile') as cf_mock:
                    with patch('candidate.run_command') as rc_mock:
                        with patch('candidate.extract_candidates') as ec_mock:
                            with patch('candidate.publish') as p_mock:
                                publish_candidates(base_dir, '~/streams',
                                                   juju_release_tools='../')
        self.assertEqual(1, cf_mock.call_count)
        output, args, kwargs = cf_mock.mock_calls[0]
        self.assertEqual(package_path, args[0])
        actual_path = args[1].replace(args[1][5:14], 'foo')
        expected_path = os.path.join(
            '/tmp', 'foo', 'juju-core_1.2.3-0ubuntu1~14.04.1~juju1_amd64.deb')
        self.assertEqual(expected_path, actual_path)
        output, args, kwargs = cf_mock.mock_calls[0]
        self.assertEqual(package_path, args[0])
        actual_path = args[1].replace(args[1][5:14], 'foo')
        expected_path = os.path.join(
            '/tmp', 'foo', 'juju-core_1.2.3-0ubuntu1~14.04.1~juju1_amd64.deb')
        self.assertEqual(1, rc_mock.call_count)
        output, args, kwargs = rc_mock.mock_calls[0]
        normalised_args = list(args[0])
        self.assertTrue(normalised_args[2].startswith('/tmp/'))
        normalised_args[2] = '/tmp/foo'
        self.assertEqual(
            ['../assemble-streams.bash', '-t', '/tmp/foo', 'weekly',
             'IGNORE', '~/streams'],
            normalised_args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        p_mock.assert_called_once_with(
            '~/streams', '../publish-public-tools.bash',
            dry_run=False, verbose=False)
        args, kwargs = ec_mock.call_args
        self.assertEqual((base_dir, ), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    @patch('sys.stdout')
    def test_publish_candidates_with_dry_run(self, so_mock):
        with temp_dir() as base_dir:
            artifacts_dir_path = os.path.join(base_dir, 'master-artifacts')
            os.makedirs(artifacts_dir_path)
            with patch('subprocess.check_output'):
                with patch('candidate.run_command') as rc_mock:
                    with patch('candidate.extract_candidates') as ec_mock:
                        publish_candidates(
                            base_dir, '~/streams', juju_release_tools='../',
                            dry_run=True, verbose=True)
        self.assertEqual(2, rc_mock.call_count)
        output, args, kwargs = rc_mock.mock_calls[0]
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)
        output, args, kwargs = rc_mock.mock_calls[1]
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)
        args, kwargs = ec_mock.call_args
        self.assertEqual((base_dir, ), args)
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)

    def test_publish_retry(self):
        error = CalledProcessError('', '', '')
        with patch('candidate.run_command', autospec=True,
                   side_effect=[error, error, '']) as rc_mock:
            publish('streams_path', 'publish_script',
                    dry_run=True, verbose=True)
        self.assertEqual(3, rc_mock.call_count)
        rc_mock.assert_called_with(
            ['publish_script', 'weekly', 'streams_path/juju-dist', 'cpc'],
            verbose=True, dry_run=True)

    def test_publish_faile(self):
        error = CalledProcessError('', '', '')
        with patch('candidate.run_command', autospec=True,
                   side_effect=[error, error, error]) as rc_mock:
            with self.assertRaises(CalledProcessError):
                publish('streams_path', 'publish_script',
                        dry_run=True, verbose=True)
        self.assertEqual(3, rc_mock.call_count)
        rc_mock.assert_called_with(
            ['publish_script', 'weekly', 'streams_path/juju-dist', 'cpc'],
            verbose=True, dry_run=True)
