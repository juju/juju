"""Tests for build_package script."""

from mock import (
    Mock,
    patch
)
import os
from textwrap import dedent
import unittest

from build_package import (
    build_binary,
    BUILD_DEB_TEMPLATE,
    build_in_lxc,
    CREATE_LXC_TEMPLATE,
    get_args,
    main,
    move_debs,
    parse_dsc,
    setup_local,
    setup_lxc,
    SourceFile,
    teardown_lxc,
)
from utils import (
    autopatch,
    temp_dir,
)


DSC_CONTENT = dedent("""\
    Format: 3.0 (quilt)
    Source: juju-core
    Binary: juju-core, juju, juju-local, juju-local-kvm
    Architecture: any all
    Version: 1.24-beta1-0ubuntu1~14.04.1~juju1
    Maintainer: Curtis Hovey <curtis.hovey@canonical.com>
    Homepage: http://launchpad.net/juju-core
    Standards-Version: 3.9.5
    Build-Depends: debhelper (>= 7.0.50~), golang-go, lsb-release, python
    Package-List:
     juju deb devel extra
     juju-core deb devel extra
     juju-local deb devel extra
     juju-local-kvm deb devel extra
    Checksums-Sha1:
     1234 9876 juju-core_1.24-beta1.orig.tar.gz
     2345 4321 juju-core_1.24-beta1-0ubuntu1~14.04.1~juju1.debian.tar.gz
    Checksums-Sha256:
     3456 9876 juju-core_1.24-beta1.orig.tar.gz
     4567 4321 juju-core_1.24-beta1-0ubuntu1~14.04.1~juju1.debian.tar.gz
    Files:
     5678 9876 juju-core_1.24-beta1.orig.tar.gz
     6789 4321 juju-core_1.24-beta1-0ubuntu1~14.04.1~juju1.debian.tar.gz
    Testsuite: autopkgtest
    """)


def make_source_files(workspace, dsc_name):
    dsc_path = os.path.join(workspace, dsc_name)
    dsc_file = SourceFile(None, None, 'my.dsc', dsc_path)
    orig_file = SourceFile(
        '3456', '9876', 'juju-core_1.24-beta1.orig.tar.gz',
        '%s/juju-core_1.24-beta1.orig.tar.gz' % workspace)
    deb_file = SourceFile(
        '4567', '4321',
        'juju-core_1.24-beta1-0ubuntu1~14.04.1~juju1.debian.tar.gz',
        '%s/juju-core_1.24-beta1-0ubuntu1~14.04.1~juju1.debian.tar.gz' %
        workspace)
    source_files = [dsc_file, orig_file, deb_file]
    for sf in source_files:
        with open(sf.path, 'w') as f:
            if '.dsc' in sf.name:
                f.write(DSC_CONTENT)
            else:
                f.write(sf.name)
    return source_files


class BuildPackageTestCase(unittest.TestCase):

    def test_get_args_binary(self):
        args = get_args(
            ['prog', 'binary', 'my.dsc', '~/workspace', 'trusty', 'i386'])
        self.assertEqual('binary', args.command)
        self.assertEqual('my.dsc', args.dsc)
        self.assertEqual('~/workspace', args.location)
        self.assertIs(None, args.ppa)
        self.assertFalse(args.verbose)

    def test_get_args_binary_with_ppa(self):
        args = get_args(
            ['prog', 'binary', '--ppa', 'ppa:juju/experimental',
             'my.dsc', '~/workspace', 'trusty', 'i386'])
        self.assertEqual('ppa:juju/experimental', args.ppa)

    def test_main_binary(self):
        with patch('build_package.build_binary', autospec=True,
                   return_value=0) as bb_mock:
            code = main(
                ['prog', 'binary', 'my.dsc', '~/workspace', 'trusty', 'i386'])
        self.assertEqual(0, code)
        bb_mock.assert_called_with(
            'my.dsc', '~/workspace', 'trusty', 'i386', ppa=None, verbose=False)

    @autopatch('build_package.move_debs', return_value=True)
    @autopatch('build_package.teardown_lxc', return_value=True)
    @autopatch('build_package.build_in_lxc')
    @autopatch('build_package.setup_lxc', return_value='trusty-i386')
    @autopatch('build_package.setup_local', return_value='build_dir')
    @autopatch('build_package.parse_dsc', return_value=['orig', 'debian'])
    def test_build_binary(self,
                          pd_mock, sl_mock, l_mock, bl_mock, tl_mock, md_mock):
        code = build_binary(
            'my.dsc', '~/workspace', 'trusty', 'i386', verbose=False)
        self.assertEqual(0, code)
        pd_mock.assert_called_with('my.dsc', verbose=False)
        sl_mock.assert_called_with(
            '~/workspace', 'trusty', 'i386', ['orig', 'debian'], verbose=False)
        l_mock.assert_called_with('trusty', 'i386', 'build_dir', verbose=False)
        bl_mock.assert_called_with(
            'trusty-i386', 'build_dir', ppa=None, verbose=False)
        tl_mock.assert_called_with('trusty-i386', verbose=False)
        md_mock.assert_called_with(
            'build_dir', '~/workspace', verbose=False)

    @patch('build_package.build_in_lxc', side_effect=Exception)
    @patch('build_package.setup_lxc', return_value='trusty-i386')
    @patch('build_package.setup_local')
    @patch('build_package.parse_dsc')
    def test_build_binary_teardown_lxc(self, p_mock, s_mock, l_mock, b_mock):
        with patch('build_package.teardown_lxc', autospec=True,
                   return_value=True) as tl_mock:
            with self.assertRaises(Exception):
                build_binary(
                    'my.dsc', '~/workspace', 'trusty', 'i386',
                    verbose=False)
        tl_mock.assert_called_with('trusty-i386', verbose=False)

    def test_parse_dsc(self):
        with temp_dir() as workspace:
            expected_files = make_source_files(workspace, 'my.dsc')
            dsc_path = os.path.join(workspace, 'my.dsc')
            source_files = parse_dsc(dsc_path, verbose=False)
        self.assertEqual(expected_files, source_files)

    def test_setup_local(self):
        with temp_dir() as workspace:
            source_files = make_source_files(workspace, 'my.dsc')
            build_dir = setup_local(
                workspace, 'trusty', 'i386', source_files, verbose=False)
            self.assertEqual(
                os.path.join(workspace, 'juju-build-trusty-i386'),
                build_dir)
            self.assertTrue(os.path.isdir(build_dir))

    def test_setup_lxc(self):
        with patch('subprocess.check_output') as co_mock:
            container = setup_lxc(
                'trusty', 'i386', '/build-dir', verbose=False)
        self.assertEqual('trusty-i386', container)
        lxc_script = CREATE_LXC_TEMPLATE.format(
            container='trusty-i386', series='trusty', arch='i386',
            build_dir='/build-dir')
        co_mock.assert_called_with([lxc_script], shell=True)

    @autopatch('subprocess.check_call')
    @autopatch('os.chmod')
    def test_build_in_lxc(self, oc_mock, cc_mock):
        with temp_dir() as workspace:
            source_files = make_source_files(workspace, 'my.dsc')
            build_dir = setup_local(
                workspace, 'trusty', 'i386', source_files, verbose=False)
            proc = Mock(returncode=0)
            with patch('subprocess.Popen', return_value=proc) as p_mock:
                code = build_in_lxc('trusty-i386', build_dir,
                                    ppa=None, verbose=False)
        self.assertEqual(0, code)
        oc_mock.assert_any_call(build_dir, 0o777)
        proc.communicate.assert_called_with()
        cc_mock.assert_any_call(
            ['sudo', 'lxc-start', '-d', '-n', 'trusty-i386'])
        build_script = BUILD_DEB_TEMPLATE.format(
            container='trusty-i386', ppa='# No PPA added.')
        p_mock.assert_called_with([build_script], shell=True)
        cc_mock.assert_any_call(['sudo', 'lxc-stop', '-n', 'trusty-i386'])
        user = os.environ.get('USER', 'jenkins')
        cc_mock.assert_any_call(['sudo', 'chown', '-R', user, build_dir])
        oc_mock.assert_any_call(build_dir, 0o775)

    @autopatch('subprocess.check_call')
    @autopatch('os.chmod')
    def test_build_in_lxc_with_ppa(self, oc_mock, cc_mock):
        with temp_dir() as workspace:
            source_files = make_source_files(workspace, 'my.dsc')
            build_dir = setup_local(
                workspace, 'trusty', 'i386', source_files, verbose=False)
            proc = Mock(returncode=0)
            with patch('subprocess.Popen', return_value=proc) as p_mock:
                build_in_lxc('trusty-i386', build_dir,
                             ppa='ppa:juju/golang', verbose=False)
        build_script = BUILD_DEB_TEMPLATE.format(
            container='trusty-i386',
            ppa='deb http://ppa.launchpad.net/juju/golang/ubuntu trusty main')
        p_mock.assert_called_with([build_script], shell=True)

    @patch.dict(os.environ, {'USER': 'bingo'})
    @autopatch('subprocess.check_call')
    @autopatch('os.chmod')
    def test_build_in_lxc_stop_lxc(self, oc_mock, cc_mock):
        with temp_dir() as workspace:
            source_files = make_source_files(workspace, 'my.dsc')
            build_dir = setup_local(
                workspace, 'trusty', 'i386', source_files, verbose=False)
            with patch('subprocess.Popen', side_effect=Exception):
                with self.assertRaises(Exception):
                    build_in_lxc('trusty-i386', build_dir, verbose=False)
        cc_mock.assert_any_call(['sudo', 'lxc-stop', '-n', 'trusty-i386'])
        cc_mock.assert_any_call(['sudo', 'chown', '-R', 'bingo', build_dir])
        oc_mock.assert_any_call(build_dir, 0o775)

    def test_teardown_lxc(self):
        with patch('subprocess.check_call') as cc_mock:
            teardown_lxc('trusty-i386', verbose=False)
        cc_mock.assert_called_with(
            ['sudo', 'lxc-destroy', '-n', 'trusty-i386'])

    def test_move_debs(self):
        with temp_dir() as workspace:
            build_dir = os.path.join(workspace, 'juju-build-trusty-i386')
            os.makedirs(build_dir)
            deb_file = os.path.join(build_dir, 'my.deb')
            with open(deb_file, 'w') as f:
                f.write('deb')
            found = move_debs(build_dir, workspace)
            self.assertTrue(found)
            self.assertFalse(os.path.isfile(deb_file))
            self.assertTrue(os.path.isfile(os.path.join(workspace, 'my.deb')))
