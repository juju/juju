"""Tests for build_package script."""

from mock import (
    Mock,
    patch
)
import os
from StringIO import StringIO
from textwrap import dedent
import unittest

from build_package import (
    _JujuSeries,
    build_binary,
    BUILD_DEB_TEMPLATE,
    build_in_lxc,
    build_source,
    BUILD_SOURCE_TEMPLATE,
    CREATE_LXC_TEMPLATE,
    create_source_package,
    create_source_package_branch,
    CREATE_SPB_TEMPLATE,
    DEBS_NOT_FOUND,
    DEFAULT_SPB,
    DEFAULT_SPB2,
    get_args,
    juju_series,
    main,
    make_changelog_message,
    make_deb_shell_env,
    make_ubuntu_version,
    move_debs,
    parse_dsc,
    print_series_info,
    setup_local,
    setup_lxc,
    Series,
    sign_source_package,
    SourceFile,
    teardown_lxc,
)
from utils import (
    autopatch,
    temp_dir,
)


class JujuSeriesTestCase(unittest.TestCase):

    def test_init(self):
        juju_series = _JujuSeries()
        self.assertEqual(
            Series('14.10', 'utopic', 'HISTORIC'),
            juju_series.all['utopic'])

    def test_get_devel_version(self):
        juju_series = _JujuSeries()
        [devel] = [s.version for s in juju_series.all.values()
                   if s.status == 'DEVEL']
        self.assertEqual(devel, juju_series.get_devel_version())

    def test_get_living_names(self):
        juju_series = _JujuSeries()
        self.assertEqual(
            ['precise', 'trusty', 'wily', 'xenial'],
            juju_series.get_living_names())

    def test_get_name(self):
        juju_series = _JujuSeries()
        self.assertEqual('trusty', juju_series.get_name('14.04'))
        with self.assertRaises(KeyError):
            juju_series.get_version('13.01')

    def test_get_name_from_package_version(self):
        juju_series = _JujuSeries()
        self.assertEqual(
            'xenial',
            juju_series.get_name_from_package_version(
                '1.26-alpha3-0ubuntu1~16.04.1~juju1'))
        self.assertEqual(
            'trusty',
            juju_series.get_name_from_package_version(
                '1.25.0-0ubuntu1~14.04.1~juju1'))
        self.assertIs(
            None,
            juju_series.get_name_from_package_version('1.25.0-0ubuntu1'))

    def test_get_version(self):
        juju_series = _JujuSeries()
        self.assertEqual('14.04', juju_series.get_version('trusty'))


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

    def test_get_args_source(self):
        shell_env = {'DEBEMAIL': 'me@email', 'DEBFULLNAME': 'me'}
        with patch.dict('os.environ', shell_env):
            args = get_args(
                ['prog', 'source', 'my_1.25.0.tar.gz', '~/workspace', 'trusty',
                 '123', '456'])
        self.assertEqual('source', args.command)
        self.assertEqual('my_1.25.0.tar.gz', args.tar_file)
        self.assertEqual('~/workspace', args.location)
        self.assertEqual('trusty', args.series)
        self.assertEqual(['123', '456'], args.bugs)
        self.assertEqual('me@email', args.debemail)
        self.assertEqual('me', args.debfullname)
        self.assertIsNone(args.gpgcmd)
        self.assertEqual(DEFAULT_SPB, args.branch)
        self.assertEqual('1', args.upatch)
        self.assertFalse(args.verbose)

    def test_get_args_source_default_spb2_branch(self):
        shell_env = {'DEBEMAIL': 'me@email', 'DEBFULLNAME': 'me'}
        with patch.dict('os.environ', shell_env):
            args = get_args(
                ['prog', 'source', 'my_2.0-a.tar.gz', '~/workspace', 'trusty',
                 '123', '456'])
        self.assertEqual('source', args.command)
        self.assertEqual('my_2.0-a.tar.gz', args.tar_file)
        self.assertEqual(DEFAULT_SPB2, args.branch)

    def test_get_args_source_with_branch(self):
        shell_env = {'DEBEMAIL': 'me@email', 'DEBFULLNAME': 'me'}
        with patch.dict('os.environ', shell_env):
            args = get_args(
                ['prog', 'source', 'my_2.0-a.tar.gz', '~/workspace', 'trusty',
                 '123', '456', '--branch', '~/my-branch'])
        self.assertEqual('source', args.command)
        self.assertEqual('my_2.0-a.tar.gz', args.tar_file)
        self.assertEqual('~/my-branch', args.branch)

    def test_get_args_source_with_living(self):
        with patch('build_package.juju_series.get_living_names', autospec=True,
                   return_value=['precise', 'trusty']) as js_mock:
            args = get_args(
                ['prog', 'source', 'my.tar.gz', '~/workspace', 'LIVING',
                 '123', '456'])
        self.assertEqual(['precise', 'trusty'], args.series)
        self.assertEqual(1, js_mock.call_count)
        args = get_args(
            ['prog', 'source', 'my.tar.gz', '~/workspace', 'LIVING',
             '123', '456'])
        self.assertEqual(juju_series.get_living_names(), args.series)

    def test_main_source(self):
        with patch('build_package.build_source', autospec=True,
                   return_value=0) as bs_mock:
            code = main([
                'prog', 'source',
                '--debemail', 'me@email', '--debfullname', 'me',
                'my.tar.gz', '~/workspace', 'trusty', '123', '456'])
        self.assertEqual(0, code)
        bs_mock.assert_called_with(
            'my.tar.gz', '~/workspace', 'trusty', ['123', '456'],
            debemail='me@email', debfullname='me', gpgcmd=None,
            branch=DEFAULT_SPB, upatch='1', verbose=False,
            date=None, build=None, revid=None)

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

    @autopatch('build_package.move_debs', return_value=False)
    @autopatch('build_package.teardown_lxc', return_value=True)
    @autopatch('build_package.build_in_lxc')
    @autopatch('build_package.setup_lxc', return_value='trusty-i386')
    @autopatch('build_package.setup_local', return_value='build_dir')
    @autopatch('build_package.parse_dsc', return_value=['orig', 'debian'])
    def test_build_binary_debs_not_found(
            self, pd_mock, sl_mock, l_mock, bl_mock, tl_mock, md_mock):
        code = build_binary(
            'my.dsc', '~/workspace', 'trusty', 'i386', verbose=False)
        self.assertEqual(DEBS_NOT_FOUND, code)

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
            with patch('subprocess.call', return_value=0) as p_mock:
                code = build_in_lxc('trusty-i386', build_dir,
                                    ppa=None, verbose=False)
        self.assertEqual(0, code)
        oc_mock.assert_any_call(build_dir, 0o777)
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

    def test_move_debs_not_found(self):
        with temp_dir() as workspace:
            build_dir = os.path.join(workspace, 'juju-build-trusty-i386')
            os.makedirs(build_dir)
            found = move_debs(build_dir, workspace)
            self.assertFalse(found)

    @autopatch('build_package.create_source_package')
    @autopatch('build_package.create_source_package_branch',
               return_value='./spb_path')
    @autopatch('build_package.setup_local',
               side_effect=['./spb_dir', './precise_dir', './trusty_dir'])
    def test_build_source(self, sl_mock, spb_mock, csp_mock):
        return_code = build_source(
            './my_1.2.3.tar.gz', './workspace', ['precise', 'trusty'], ['987'],
            debemail=None, debfullname=None, gpgcmd='my/gpg',
            branch='lp:branch', upatch=None, verbose=False)
        sl_mock.assert_any_call(
            './workspace', 'any', 'all',
            [SourceFile(None, None, 'my_1.2.3.tar.gz', './my_1.2.3.tar.gz')],
            verbose=False)
        spb_mock.assert_called_with(
            './spb_dir', '1.2.3', 'my_1.2.3.tar.gz', 'lp:branch')
        sl_mock.assert_any_call(
            './workspace', 'precise', 'all', [], verbose=False)
        sl_mock.assert_any_call(
            './workspace', 'trusty', 'all', [], verbose=False)
        self.assertEqual(0, return_code)

    @autopatch('subprocess.check_call')
    def test_create_source_package_branch(self, cc_mock):
        spb = create_source_package_branch(
            'juju-build-any-all', '1.2.3', 'juju-core_1.2.3.tar.gz',
            DEFAULT_SPB)
        self.assertEqual('juju-build-any-all/spb', spb)
        script = CREATE_SPB_TEMPLATE.format(
            branch=DEFAULT_SPB, spb='juju-build-any-all/spb',
            tarfile_path='juju-build-any-all/juju-core_1.2.3.tar.gz',
            version='1.2.3')
        cc_mock.assert_called_with(
            [script], shell=True, cwd='juju-build-any-all')

    def test_make_ubuntu_version(self):
        ubuntu_version = make_ubuntu_version('trusty', '1.2.3')
        self.assertEqual('1.2.3-0ubuntu1~14.04.1~juju1', ubuntu_version)
        ubuntu_version = make_ubuntu_version('precise', '1.22-alpha1', '8')
        self.assertEqual('1.22-alpha1-0ubuntu1~12.04.8~juju1', ubuntu_version)

    def test_make_changelog_message(self):
        message = make_changelog_message('1.2.0')
        self.assertEqual('New upstream stable release.', message)
        message = make_changelog_message('1.2.3')
        self.assertEqual('New upstream stable point release.', message)
        message = make_changelog_message('1.2-a')
        self.assertEqual('New upstream devel release.', message)
        message = make_changelog_message('1.2.3', ['987', '876'])
        self.assertEqual(
            'New upstream stable point release. (LP #987, LP #876)',
            message)

    @autopatch('build_package.sign_source_package')
    @autopatch('subprocess.check_call')
    def test_create_source_package(self, cc_mock, ss_mock):
        create_source_package(
            '/juju-build-trusty-all', '/juju-build-any-all/spb', 'trusty',
            '1.2.3', upatch='1', bugs=['987'], gpgcmd=None,
            debemail='me@email', debfullname='me', verbose=False)
        script = BUILD_SOURCE_TEMPLATE.format(
            spb='/juju-build-any-all/spb',
            source='/juju-build-trusty-all/source',
            series='trusty', ubuntu_version='1.2.3-0ubuntu1~14.04.1~juju1',
            message='New upstream stable point release. (LP #987)')
        env = make_deb_shell_env('me@email', 'me')
        cc_mock.assert_called_with(
            [script], shell=True, cwd='/juju-build-trusty-all', env=env)
        self.assertEqual(0, ss_mock.call_count)

    @autopatch('build_package.sign_source_package')
    @autopatch('subprocess.check_call')
    def test_create_source_package_with_gpgcmd(self, cc_mock, ss_mock):
        create_source_package(
            '/juju-build-trusty-all', '/juju-build-any-all/spb', 'trusty',
            '1.2.3', upatch='1', bugs=['987'], gpgcmd='/my/gpgcmd',
            debemail='me@email', debfullname='me', verbose=False)
        script = BUILD_SOURCE_TEMPLATE.format(
            spb='/juju-build-any-all/spb',
            source='/juju-build-trusty-all/source',
            series='trusty', ubuntu_version='1.2.3-0ubuntu1~14.04.1~juju1',
            message='New upstream stable point release. (LP #987)')
        env = make_deb_shell_env('me@email', 'me')
        cc_mock.assert_called_with(
            [script], shell=True, cwd='/juju-build-trusty-all', env=env)
        ss_mock.assert_called_with(
            '/juju-build-trusty-all', '/my/gpgcmd', 'me@email', 'me')

    @autopatch('subprocess.check_call')
    def test_sign_source_package(self, cc_mock):
        sign_source_package(
            '/juju-build-trusty-all', '/my/gpgcmd', 'me@email', 'me')
        env = make_deb_shell_env('me@email', 'me')
        cc_mock.assert_called_with(
            ['debsign -p /my/gpgcmd *.changes'],
            shell=True, cwd='/juju-build-trusty-all', env=env)

    def test_get_args_print(self):
        args = get_args(
            ['prog', 'print', '--series-name-from-package-version',
             '1.25.0-0ubuntu1~16.04.1~juju1'])
        self.assertEqual('print', args.command)
        self.assertEqual(
            '1.25.0-0ubuntu1~16.04.1~juju1',
            args.series_name_from_package_version)

    def test_main_print(self):
        with patch('build_package.print_series_info', autospec=True,
                   return_value=0) as psi_mock:
            code = main(
                ['prog', 'print', '--series-name-from-package-version',
                 '1.25.0-0ubuntu1~16.04.1~juju1'])
        self.assertEqual(0, code)
        psi_mock.assert_called_with(
            package_version='1.25.0-0ubuntu1~16.04.1~juju1')

    @patch('sys.stdout', new_callable=StringIO)
    def test_print_series_info(self, so_mock):
        # Unmatched.
        code = print_series_info(
            package_version='1.25.0-0ubuntu1')
        self.assertEqual(1, code)
        self.assertEqual('', so_mock.getvalue())
        # Matched.
        code = print_series_info(
            package_version='1.25.0-0ubuntu1~16.04.1~juju1')
        self.assertEqual(0, code)
        self.assertEqual('xenial\n', so_mock.getvalue())
