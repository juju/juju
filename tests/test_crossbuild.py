from contextlib import contextmanager
from mock import patch
import os
import subprocess
import sys
import tarfile
from unittest import TestCase

from crossbuild import (
    build_centos,
    build_osx_client,
    build_win_agent,
    build_win_client,
    go_build,
    go_tarball,
    GOLANG_VERSION,
    ISCC_CMD,
    ISS_DIR,
    make_installer,
    make_client_tarball,
    make_agent_tarball,
    main,
    run_command,
    working_directory,
)
from utils import (
    autopatch,
    temp_dir,
)


@contextmanager
def fake_go_tarball(path):
    gopath = path.replace('.tar.gz', '')
    version = os.path.basename(gopath).split('_')[-1]
    yield gopath, version


class CrossBuildTestCase(TestCase):

    def setUp(self):
        # Ensure that no test ever fails and calls subprocess
        patcher = patch('subprocess.check_output')
        self.addCleanup(patcher.stop)
        self.co_mock = patcher.start()

    def test_main_setup(self):
        with patch('crossbuild.setup_cross_building') as mock:
            main(['-d', '-v', 'setup', '--build-dir', './foo'])
        args, kwargs = mock.call_args
        self.assertEqual(('./foo', ), args)
        self.assertEqual({'dry_run': True, 'verbose': True}, kwargs)

    def test_main_osx_client(self):
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

    def test_main_centos_both(self):
        with patch('crossbuild.build_centos') as mock:
            main(['centos', '--build-dir', './foo', 'bar.1.2.3.tar.gz'])
        args, kwargs = mock.call_args
        self.assertEqual(('bar.1.2.3.tar.gz', './foo'), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)

    def test_go_build(self):
        with patch('crossbuild.run_command') as mock:
            go_build(
                'github/juju/juju/...', './foo', './bar.1.2', '386', 'windows',
                verbose=True, dry_run=True)
        args, kwargs = mock.call_args
        self.assertEqual((['go', 'install', 'github/juju/juju/...'],), args)
        self.assertTrue(kwargs['dry_run'])
        self.assertTrue(kwargs['verbose'])
        env = kwargs['env']
        self.assertEqual('./foo', env['GOROOT'])
        self.assertEqual('./bar.1.2', env['GOPATH'])
        self.assertEqual('386', env['GOARCH'])
        self.assertEqual('windows', env['GOOS'])

    def test_run_command(self):
        with patch('subprocess.call', autospec=True, return_value=0) as mock:
            run_command(
                ['ls'], env={'CB_MARK': 'foo'}, dry_run=False, verbose=True)
        args, kwargs = mock.call_args
        self.assertEqual((['ls'], ), args)
        self.assertEqual(
            {'env': {'CB_MARK': 'foo'}},
            kwargs)
        with patch('subprocess.check_output') as mock:
            run_command(['ls'], dry_run=True)
        self.assertEqual(0, mock.call_count)

    def test_gotarball_raises_error(self):
        with self.assertRaises(ValueError) as ae:
            go_tarball('foo.tar.gz').__enter__()
        self.assertEqual('Not a tar.gz: foo.tar.gz', str(ae.exception))

    def test_go_tarball_gopath(self):
        with temp_dir() as base_dir:
            src_path = os.path.join(base_dir, 'juju-core_1.2.3')
            os.makedirs(src_path)
            tarball_path = '%s.tar.gz' % src_path
            with tarfile.open(tarball_path, 'w:gz') as tar:
                tar.add(src_path, arcname='juju-core_1.2.3')
            with go_tarball(tarball_path) as (gopath, version):
                self.assertTrue(os.path.isdir(gopath))
                self.assertTrue(gopath.endswith('juju-core_1.2.3'), gopath)
                self.assertEqual('1.2.3', version)

    def test_working_directory(self):
        this_dir = os.getcwd()
        with temp_dir() as base_dir:
            new_dir = os.path.join(base_dir, 'juju-core_1.2.3')
            os.makedirs(new_dir)
            with working_directory(new_dir):
                self.assertEqual(os.path.realpath(new_dir), os.getcwd())
        self.assertEqual(this_dir, os.getcwd())

    def test_build_win_client(self):
        with patch('crossbuild.go_tarball',
                   side_effect=fake_go_tarball) as gt_mock:
            with patch('crossbuild.go_build') as gb_mock:
                with patch('crossbuild.make_installer') as mi_mock:
                    build_win_client('baz/bar_1.2.3.tar.gz', '/foo')
        args, kwargs = gt_mock.call_args
        self.assertEqual(('baz/bar_1.2.3.tar.gz', ), args)
        args, kwargs = gb_mock.call_args
        self.assertEqual(
            ('github.com/juju/juju/cmd/juju',
             '/foo/golang-%s' % GOLANG_VERSION, 'baz/bar_1.2.3',
             '386', 'windows'),
            args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        self.assertEqual(
            ('baz/bar_1.2.3/bin/windows_386/juju.exe',
             '1.2.3', 'baz/bar_1.2.3', os.getcwd()),
            mi_mock.call_args[0])

    def test_make_installer_default(self):
        with temp_dir() as base_dir:
            iss_dir = os.path.join(base_dir, ISS_DIR)
            iss_output_dir = os.path.join(iss_dir, 'Output')
            os.makedirs(iss_output_dir)
            with patch('shutil.move') as mv_mock:
                with patch('crossbuild.run_command') as rc_mock:
                    make_installer(
                        'foo/juju.exe', '1.2.3', base_dir, os.getcwd())
        self.assertEqual(2, mv_mock.call_count)
        # The juju.exe was moved to the iss dir.
        self.assertEqual(
            ('foo/juju.exe', iss_dir), mv_mock.mock_calls[0][1])
        args, kwargs = rc_mock.call_args
        self.assertEqual((['xvfb-run', 'wine', ISCC_CMD, 'setup.iss'], ), args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        # The installer was moved to the working dir.
        self.assertEqual(
            ('%s/juju-setup-1.2.3.exe' % iss_output_dir, os.getcwd()),
            mv_mock.mock_calls[1][1])

    def test_make_installer_with_dry_run(self):
        with temp_dir() as base_dir:
            iss_dir = os.path.join(base_dir, ISS_DIR)
            iss_output_dir = os.path.join(iss_dir, 'output')
            os.makedirs(iss_output_dir)
            with patch('shutil.move') as mv_mock:
                with patch('crossbuild.run_command') as rc_mock:
                    make_installer(
                        'foo/juju.exe', '1.2.3', base_dir, os.getcwd(),
                        dry_run=True, verbose=True)
        # The juju.exe was moved to the install dir, but the installer
        # was not move to the working dir.
        self.assertEqual(1, mv_mock.call_count)
        self.assertEqual(
            ('foo/juju.exe', iss_dir), mv_mock.mock_calls[0][1])
        # The installer is created in a tmp dir, so dry_run is always False.
        self.assertEqual(
            {'dry_run': False, 'verbose': True},
            rc_mock.call_args[1])

    def test_build_win_agent(self):
        with patch('crossbuild.go_tarball',
                   side_effect=fake_go_tarball) as gt_mock:
            with patch('crossbuild.go_build') as gb_mock:
                with patch('crossbuild.make_agent_tarball') as mt_mock:
                    build_win_agent('baz/bar_1.2.3.tar.gz', '/foo')
        args, kwargs = gt_mock.call_args
        self.assertEqual(('baz/bar_1.2.3.tar.gz', ), args)
        args, kwargs = gb_mock.call_args
        self.assertEqual(
            ('github.com/juju/juju/cmd/jujud',
             '/foo/golang-%s' % GOLANG_VERSION, 'baz/bar_1.2.3',
             'amd64', 'windows'),
            args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        self.assertEqual(
            ('win2012', 'baz/bar_1.2.3/bin/windows_amd64/jujud.exe',
             '1.2.3',
             os.getcwd()),
            mt_mock.call_args[0])

    def test_make_agent_tarball(self):
        with temp_dir() as base_dir:
            agent_dir = os.path.join(base_dir, 'foo')
            os.makedirs(agent_dir)
            jujud_binary = os.path.join(agent_dir, 'jujud.exe')
            with open(jujud_binary, 'w') as jb:
                jb.write('jujud')
            make_agent_tarball('win2012', jujud_binary, '1.2.3', base_dir)
            agent_tarball_path = os.path.join(
                base_dir, 'juju-1.2.3-win2012-amd64.tgz')
            self.assertTrue(os.path.isfile(agent_tarball_path))
            with tarfile.open(agent_tarball_path, 'r:gz') as tar:
                self.assertEqual(['jujud.exe'], tar.getnames())

    def test_make_agent_tarball_with_dry_run(self):
        with patch('tarfile.open') as mock:
            make_agent_tarball(
                'win2012', 'foo/jujud.exe', '1.2.3', './bar', dry_run=True)
        self.assertEqual(0, mock.call_count)

    def test_build_osx_client(self):
        with patch('crossbuild.go_tarball',
                   side_effect=fake_go_tarball) as gt_mock:
            with patch('crossbuild.go_build') as gb_mock:
                with patch('crossbuild.make_client_tarball') as mt_mock:
                    build_osx_client('baz/bar_1.2.3.tar.gz', '/foo')
        args, kwargs = gt_mock.call_args
        self.assertEqual(('baz/bar_1.2.3.tar.gz', ), args)
        args, kwargs = gb_mock.call_args
        self.assertEqual(
            ('github.com/juju/juju/cmd/...',
             '/foo/golang-%s' % GOLANG_VERSION, 'baz/bar_1.2.3',
             'amd64', 'darwin'),
            args)
        self.assertEqual({'dry_run': False, 'verbose': False}, kwargs)
        if sys.platform == 'darwin':
            cross_bin = 'bin'
        else:
            cross_bin = 'bin/darwin_amd64'
        self.assertEqual(
            ('osx',
             ['baz/bar_1.2.3/%s/juju' % cross_bin,
              'baz/bar_1.2.3/%s/juju-metadata' % cross_bin,
              'baz/bar_1.2.3/src/github.com/juju/juju/'
              'scripts/win-installer/README.txt',
              'baz/bar_1.2.3/src/github.com/juju/juju/LICENCE',
              ],
             '1.2.3', os.getcwd()),
            mt_mock.call_args[0])

    def test_make_client_tarball(self):
        with temp_dir() as base_dir:
            cmd_dir = os.path.join(base_dir, 'foo')
            os.makedirs(cmd_dir)
            juju_binary = os.path.join(cmd_dir, 'juju')
            readme_file = os.path.join(cmd_dir, 'README.txt')
            for path in [juju_binary, readme_file]:
                with open(path, 'w') as jb:
                    jb.write('juju')
            os.chmod(juju_binary, 0o775)
            os.chmod(readme_file, 0o664)
            make_client_tarball(
                'osx', [juju_binary, readme_file], '1.2.3', base_dir)
            osx_tarball_path = os.path.join(base_dir, 'juju-1.2.3-osx.tar.gz')
            self.assertTrue(os.path.isfile(osx_tarball_path))
            with tarfile.open(osx_tarball_path, 'r:gz') as tar:
                self.assertEqual(
                    ['juju-bin', 'juju-bin/juju', 'juju-bin/README.txt'],
                    tar.getnames())
                self.assertEqual(
                    0o775, tar.getmember('juju-bin').mode)
                self.assertEqual(
                    0o775, tar.getmember('juju-bin/juju').mode)
                self.assertEqual(
                    0o664, tar.getmember('juju-bin/README.txt').mode)

    @autopatch('crossbuild.make_agent_tarball')
    @autopatch('crossbuild.make_client_tarball')
    @autopatch('crossbuild.go_build')
    @autopatch('crossbuild.go_tarball', side_effect=fake_go_tarball)
    def test_build_centos(self, gt_mock, gb_mock, mt_mock, at_mock):
        build_centos('baz/bar_1.2.3.tar.gz', '/foo')
        gt_mock.assert_called_once_with('baz/bar_1.2.3.tar.gz')
        gb_mock.assert_called_once_with(
            'github.com/juju/juju/cmd/...',
            '/foo/golang-%s' % GOLANG_VERSION, 'baz/bar_1.2.3',
            'amd64', 'linux', dry_run=False, verbose=False)
        bin_paths = [
            'baz/bar_1.2.3/bin/juju',
            'baz/bar_1.2.3/bin/jujud',
            'baz/bar_1.2.3/bin/juju-metadata',
            'baz/bar_1.2.3/src/github.com/juju/juju/'
            'scripts/win-installer/README.txt',
            'baz/bar_1.2.3/src/github.com/juju/juju/LICENCE',
            ]
        mt_mock.assert_called_once_with(
            'centos7', bin_paths, '1.2.3', os.getcwd(),
            dry_run=False, verbose=False)
        at_mock.assert_called_once_with(
            'centos7', 'baz/bar_1.2.3/bin/jujud', '1.2.3', os.getcwd(),
            dry_run=False, verbose=False)
