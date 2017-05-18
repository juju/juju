from mock import (
    Mock,
    patch
)
import os
from unittest import TestCase

from lp_copy_packages import (
    copy_packages,
    get_archives,
    get_args,
    Launchpad,
    main,
)


class LPCopyPackagesTestCase(TestCase):

    def test_get_args(self):
        args = get_args(['1.2.3', 'proposed'])
        self.assertEqual('1.2.3', args.version)
        self.assertEqual('proposed', args.to_archive_name)
        self.assertIsNone(args.credentials)
        self.assertFalse(args.dry_run)

    def test_get_args_with_credentials(self):
        args = get_args(['-c', '~/my.creds', '1.2.3', 'proposed'])
        self.assertEqual('1.2.3', args.version)
        self.assertEqual('proposed', args.to_archive_name)
        self.assertEqual(os.path.expanduser('~/my.creds'), args.credentials)
        self.assertFalse(args.dry_run)

    def test_main(self):
        lp = object()
        with patch('lp_copy_packages.copy_packages', autospec=True,
                   return_value=0) as cp_mock:
            # Cannot autospec staticmethods.
            with patch.object(Launchpad, 'login_with',
                              return_value=lp) as lw_mock:
                return_code = main(['-c', 'my.creds', '1.2.3', 'proposed'])
        self.assertEqual(0, return_code)
        cp_mock.assert_called_with(lp, '1.2.3', 'proposed', False)
        lw_mock.assert_called_with(
            'lp-copy-packages', service_root='https://api.launchpad.net',
            version='devel', credentials_file='my.creds')

    def test_get_archives_devel(self):
        from_team_mock = Mock(getPPAByName=Mock())
        to_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': to_team_mock}
        from_archive, to_archive = get_archives(lp, 'devel')
        self.assertIsInstance(from_archive, Mock)
        self.assertIsInstance(to_archive, Mock)
        from_team_mock.getPPAByName.assert_called_with(name='devel')
        to_team_mock.getPPAByName.assert_called_with(name='devel')

    def test_get_archives_proposed(self):
        from_team_mock = Mock(getPPAByName=Mock())
        to_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': to_team_mock}
        from_archive, to_archive = get_archives(lp, 'proposed')
        from_team_mock.getPPAByName.assert_called_with(name='stable')
        to_team_mock.getPPAByName.assert_called_with(name='proposed')

    def test_get_archives_stable(self):
        from_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': from_team_mock}
        from_archive, to_archive = get_archives(lp, 'stable')
        from_team_mock.getPPAByName.assert_any_call(name='proposed')
        from_team_mock.getPPAByName.assert_called_with(name='stable')

    def test_get_archives_ubuntu_proposed(self):
        from_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': from_team_mock}
        from_archive, to_archive = get_archives(lp, '1.22')
        from_team_mock.getPPAByName.assert_any_call(
            name='1.22-proposed')
        from_team_mock.getPPAByName.assert_called_with(name='1.22')

    def test_get_archives_ubuntu_supported(self):
        from_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': from_team_mock}
        from_archive, to_archive = get_archives(lp, '1.22')
        from_team_mock.getPPAByName.assert_any_call(
            name='1.22-proposed')
        from_team_mock.getPPAByName.assert_called_with(name='1.22')

    def test_get_archives_unknown_destination_error(self):
        lp = Mock()
        with self.assertRaises(ValueError):
            get_archives(lp, '1.7')

    def test_copy_packages(self):
        lp = Mock()
        from_archive = Mock(getPublishedSources=Mock())
        from_archive.getPublishedSources.return_value = [
            Mock(source_package_version='1.2.3~0',
                 source_package_name='juju-2.0'),
            Mock(source_package_version='1.2.1~0',
                 source_package_name='juju-2.0')]
        to_archive = Mock(copyPackage=Mock())
        with patch('lp_copy_packages.get_archives', autospec=True,
                   return_value=(from_archive, to_archive)) as ga_mock:
            return_code = copy_packages(lp, '1.2.3', 'proposed', dry_run=False)
        self.assertEqual(0, return_code)
        ga_mock.assert_called_with(lp, 'proposed')
        from_archive.getPublishedSources.assert_called_with(
            status='Published', source_name='juju-2.0')
        self.assertEqual(1, to_archive.copyPackage.call_count)
        to_archive.copyPackage.assert_called_with(
            from_archive=from_archive, source_name='juju-2.0',
            version='1.2.3~0', to_pocket='Release',
            include_binaries=True, unembargo=True)
