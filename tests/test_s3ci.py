from argparse import Namespace
from unittest import TestCase

from s3ci import parse_args
from tests import parse_error


class TestParseArgs(TestCase):

    def test_get_juju_bin_defaults(self):
        args = parse_args(['get-juju-bin', 'myconfig', '3275'])
        self.assertEqual(Namespace(
            command='get-juju-bin', config='myconfig', revision_build='3275',
            workspace='.'),
            args)

    def test_get_juju_bin_workspace(self):
        args = parse_args(['get-juju-bin', 'myconfig', '3275', 'myworkspace'])
        self.assertEqual('myworkspace', args.workspace)

    def test_get_juju_bin_too_few(self):
        with parse_error(self) as stderr:
            parse_args(['get-juju-bin', 'myconfig'])
        self.assertRegexpMatches(stderr.getvalue(), 'too few arguments$')
