from mock import (
    Mock,
    patch
)
from unittest import TestCase

from lp_copy_packages import (
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
        self.assertFalse(args.dry_run)

    def test_main(self):
        with patch('lp_copy_packages.copy_packages', autospec=True,
                   return_value=0) as cp_mock:
            return_code = main(['1.2.3', 'proposed'])
        self.assertEqual(0, return_code)
        args, kwargs = cp_mock.call_args
        self.assertIsInstance(args[0], Launchpad)
        self.assertEqual(('1.2.3', 'proposed', False), args[1:])

    def test_get_archives_devel(self):
        from_team_mock = Mock(getPPAByName=Mock())
        to_team_mock = Mock(getPPAByName=Mock())
        lp = Mock()
        lp.people = {'juju-packaging': from_team_mock, 'juju': to_team_mock}
        # juju-packaging devel to juju devel
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
        from_team_mock.getPPAByName.assert_called_with_any(name='proposed')
        from_team_mock.getPPAByName.assert_called_with(name='stable')
