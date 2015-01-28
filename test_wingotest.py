from mock import patch
import os
import shutil
import tarfile
from unittest import TestCase

from wingotest import (
    GO_CMD,
    go_test_package,
    setup_workspace,
    untar_gopath,
)
from utility import temp_dir


class WinGoTestTestCase(TestCase):

    def test_setup_workspace_pristine(self):
        with temp_dir() as ci_dir:
            gopath = os.path.join(ci_dir, 'gogo')
            tmp_dir = os.path.join(ci_dir, 'tmp')
            setup_workspace('juju-1.2.3.tar.gz', ci_dir, gopath, tmp_dir)
            gopath = os.path.join(ci_dir, 'gogo')
            self.assertFalse(os.path.isdir(gopath))
            tmp_dir = os.path.join(ci_dir, 'tmp')
            self.assertTrue(os.path.isdir(tmp_dir))

    def test_setup_workspace_dirty(self):
        with temp_dir() as ci_dir:
            gopath = os.path.join(ci_dir, 'gogo')
            tmp_dir = os.path.join(ci_dir, 'tmp')
            os.makedirs(gopath)
            os.makedirs(tmp_dir)
            with open(os.path.join(tmp_dir, 'f.txt'), 'w') as f:
                f.write('file')
            setup_workspace('juju-1.2.3.tar.gz', ci_dir, gopath, tmp_dir)
            self.assertFalse(os.path.isdir(gopath))
            self.assertTrue(os.path.isdir(tmp_dir))
            self.assertEqual([], os.listdir(tmp_dir))

    def test_untar_gopath(self):
        with temp_dir() as base_dir:
            old_dir = os.path.join(base_dir, 'juju-1.2.3')
            for sub in ['bin', 'pkg', 'src']:
                os.makedirs(os.path.join(old_dir, sub))
            tarfile_path = os.path.join(base_dir, 'juju-1.2.3.tar.gz')
            with tarfile.open(name=tarfile_path, mode='w:gz') as tar:
                tar.add(old_dir, arcname='juju-1.2.3')
            shutil.rmtree(old_dir)
            gopath = os.path.join(base_dir, 'gogo')
            untar_gopath(tarfile_path, gopath, delete=True)
            self.assertTrue(os.path.isdir(gopath))
            self.assertEqual(['bin', 'src', 'pkg'], os.listdir(gopath))
            self.assertFalse(os.path.isfile(tarfile_path))

    def test_go_test_package(self):
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
