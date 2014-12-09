import subprocess
from mock import patch
import os
import tarfile
from unittest import TestCase

from crossbuild import (
    go_build,
    go_tarball,
    main,
    run_command,
)
from utils import temp_dir


class CrossBuildTestCase(TestCase):

    def test_main_setup(self):
        with patch('crossbuild.setup_cross_building') as mock:
            main(['-d', '-v', 'setup', '--build-dir', './foo'])
        args, kwargs = mock.call_args
        self.assertEqual(('./foo', ), args)
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)

    def test_main_osx_clientt(self):
        with patch('crossbuild.build_osx_client') as mock:
            main(['osx-client', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_main_win_client(self):
        with patch('crossbuild.build_win_client') as mock:
            main(['win-client', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_main_win_agent(self):
        with patch('crossbuild.build_win_agent') as mock:
            main(['win-agent', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_go_build(self):
        with patch('crossbuild.run_command') as mock:
            go_build(
                'github/juju/juju/...', './foo', './bar.1.2', '386', 'windows',
                verbose=True, dry_run=True)
        args, kwargs = mock.call_args
        self.assertEqual((['go', 'build', 'github/juju/juju/...'],), args)
        self.assertTrue(kwargs['dry_run'])
        self.assertTrue(kwargs['verbose'])
        env = kwargs['env']
        self.assertEqual('./foo', env['GOROOT'])
        self.assertEqual('./bar.1.2', env['GOPATH'])
        self.assertEqual('386', env['GOARCH'])
        self.assertEqual('windows', env['GOOS'])

    def test_run_command(self):
        with patch('subprocess.check_output') as mock:
            run_command(
                ['ls'], env={'CB_MARK': 'foo'}, dry_run=False, verbose=True)
        args, kwargs = mock.call_args
        self.assertEqual((['ls'], ), args)
        self.assertEqual(
            {'env': {'CB_MARK': 'foo'}, 'stderr': subprocess.STDOUT},
            kwargs)
        with patch('subprocess.check_output') as mock:
            run_command(['ls'], dry_run=True)
        self.assertEqual(0, mock.call_count)

    def test_gotarball_raises_error(self):
        with self.assertRaises(ValueError):
            go_tarball('foo.tar.gz').__enter__()

    def test_go_tarball_gopath(self):
        with temp_dir() as base_dir:
            src_path = os.path.join(base_dir, 'juju-core_1.2.3')
            os.makedirs(src_path)
            tarball_path = '%s.tar.gz' % src_path
            with tarfile.open(tarball_path, 'w:gz') as tar:
                tar.add(src_path, arcname='juju-core_1.2.3')
            with go_tarball(tarball_path) as gopath:
                self.assertTrue(os.path.isdir(gopath))
                self.assertTrue(gopath.endswith('juju-core_1.2.3'), gopath)
