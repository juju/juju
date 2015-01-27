from mock import patch
import os
import tarfile
from unittest import TestCase

from wingotest import (
    GO_CMD,
    go_test_package,
)
from utility import temp_dir


class WinGoTestTestCase(TestCase):

    def test_go_test_package(self):
        # build_agent creates a win amd64 jujud.
        with temp_dir() as gopath:
            package_path = os.path.join(
                gopath, 'src', 'github', 'juju', 'juju')
            os.makedirs(package_path)
            with patch('wingotest.run', return_value='') as run_mock:
                devnull = open(os.devnull, 'w')
                with patch('sys.stdout', devnull):
                    go_test_package('github/juju/juju', GO_CMD, gopath)
                    args, kwargs = run_mock.call_args
                    self.assertEqual((GO_CMD, 'test', './...'), args)
                    self.assertEqual('amd64', kwargs['env'].get('GOARCH'))
                    self.assertEqual(gopath, kwargs['env'].get('GOPATH'))
