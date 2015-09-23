from argparse import Namespace
from unittest import TestCase

from assess_bootstrap import parse_args


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['foo', 'bar'])
        self.assertEqual(args, Namespace(
            juju='foo', env='bar', debug=False, region=None,
            temp_env_name=None))

    def test_parse_args_debug(self):
        args = parse_args(['foo', 'bar', '--debug'])
        self.assertEqual(args.debug, True)

    def test_parse_args_region(self):
        args = parse_args(['foo', 'bar', '--region', 'foo'])
        self.assertEqual(args.region, 'foo')

    def test_parse_args_temp_env_name(self):
        args = parse_args(['foo', 'bar', 'foo'])
        self.assertEqual(args.temp_env_name, 'foo')
