from mock import patch
from unittest import TestCase

from lp_copy_packages import (
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
