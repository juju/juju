import contextlib
from mock import (
    call,
    patch,
)
import os
from unittest import TestCase

import gotestwin
from utility import temp_dir


JUJU_CI_PATH = os.path.join(gotestwin.SCRIPTS, 'jujuci.py')
JUJU_HOME = os.path.join(gotestwin.SCRIPTS, '..', 'cloud-city')


@contextlib.contextmanager
def working_directory(path):
    curdir = os.getcwd()
    try:
        os.chdir(path)
        yield
    finally:
        os.chdir(curdir)


@patch('subprocess.check_output', return_value='path/foo.tar.gz')
@patch('subprocess.check_call')
class GoTestWinTestCase(TestCase):

    def test_main_with_revision(self, cc_mock, co_mock):
        with temp_dir() as base:
            with working_directory(base):
                gotestwin.main(['host', '1234'])
        co_mock.assert_called_once_with(
            [JUJU_CI_PATH, 'get', '-b', '1234', 'build-revision',
             '*.tar.gz', './'])
        tarfile_call = call(
            [JUJU_CI_PATH, 'get-build-vars', '--summary', '1234'])
        gotest_call = call(
            ['workspace-run', '-v', '-i', 'cloud-city/staging-juju-rsa',
             'temp-config.yaml', 'Administrator@host'])
        self.assertEqual([tarfile_call, gotest_call], cc_mock.call_args_list)
