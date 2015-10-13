from argparse import Namespace
from unittest import TestCase

from azure_publish_tools import (
    DELETE,
    get_option_parser,
    LIST,
    PUBLISH,
    )

class TestOptionParser(TestCase):

    def parse_args(self, args):
        parser = get_option_parser()
        return parser.parse_args(args)

    def test_list(self):
        args = self.parse_args(['list', 'mypurpose'])
        self.assertEqual(Namespace(
            command=LIST, purpose='mypurpose'), args)

    def test_publish(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=PUBLISH, purpose='mypurpose', dry_run=False, verbose=False,
            path=['mypath']), args)

    def test_publish_dry_run(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_publish_verbose(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_publish_path(self):
        args = self.parse_args(['publish', 'mypurpose', 'mypath', 'mypath2'])
        self.assertEqual(['mypath', 'mypath2'], args.path)

    def test_delete(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath'])
        self.assertEqual(Namespace(
            command=DELETE, purpose='mypurpose', dry_run=False, verbose=False,
            path=['mypath']), args)

    def test_delete_dry_run(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--dry-run'])
        self.assertIs(True, args.dry_run)

    def test_delete_verbose(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', '--verbose'])
        self.assertIs(True, args.verbose)

    def test_delete_path(self):
        args = self.parse_args(['delete', 'mypurpose', 'mypath', 'mypath2'])
        self.assertEqual(['mypath', 'mypath2'], args.path)
