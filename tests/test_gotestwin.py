import contextlib
import json
from mock import (
    call,
    patch,
)
import os

import gotestwin
from tests import TestCase
from utility import temp_dir


S3_CI_PATH = os.path.join(gotestwin.SCRIPTS, 's3ci.py')
JUJU_HOME = os.path.normpath(os.path.join(
    gotestwin.SCRIPTS, '..', 'cloud-city'))
REMOTE_SCRIPT = (
    'c:\\\\Users\\\\Administrator\\\\juju-ci-tools\\\\gotesttarfile.py')


@contextlib.contextmanager
def working_directory(path):
    curdir = os.getcwd()
    try:
        os.chdir(path)
        yield
    finally:
        os.chdir(curdir)


class GoTestWinTestCase(TestCase):

    @patch('subprocess.check_output', return_value='path/foo.tar.gz')
    @patch('subprocess.check_call')
    def test_main_with_revision(self, cc_mock, co_mock):
        with temp_dir() as base:
            with working_directory(base):
                gotestwin.main(['host', '1234'])
                self.assertTrue(os.path.exists('temp-config.yaml'))
                with open('temp-config.yaml') as f:
                    data = json.load(f)
        self.assertEqual(
            ['python', REMOTE_SCRIPT, '-v', '-g', 'go.exe', '-p',
             'github.com/juju/juju', '--remove', 'ci/foo.tar.gz'],
            data['command'])
        co_mock.assert_called_once_with(
            [S3_CI_PATH, 'get', '1234', 'build-revision',
             '.*.tar.gz', './'])
        tarfile_call = call(
            [S3_CI_PATH, 'get-summary', '1234', 'GoTestWin'])
        gotest_call = call(
            ['workspace-run', '-v', '-i',
             '{}/staging-juju-rsa'.format(JUJU_HOME),
             'temp-config.yaml', 'Administrator@host'])
        self.assertEqual([tarfile_call, gotest_call], cc_mock.call_args_list)

    @patch('subprocess.check_call')
    def test_main_with_tarfile_and_package(self, cc_mock):
        with temp_dir() as base:
            with working_directory(base):
                gotestwin.main(
                    ['host', 'bar.tar.gz', 'github.com/juju/juju/cmd'])
                self.assertTrue(os.path.exists('temp-config.yaml'))
                with open('temp-config.yaml') as f:
                    data = json.load(f)
        self.assertEqual(
            ['python', REMOTE_SCRIPT, '-v', '-g', 'go.exe', '-p',
             'github.com/juju/juju/cmd', '--remove', 'ci/bar.tar.gz'],
            data['command'])
        cc_mock.assert_called_once_with(
            ['workspace-run', '-v', '-i',
             '{}/staging-juju-rsa'.format(JUJU_HOME), 'temp-config.yaml',
             'Administrator@host'])
