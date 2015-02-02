from assess_bootstrap import parse_args

from unittest import TestCase

class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['foo', 'bar'])
        self.assertEqual(args.juju, 'foo')
        self.assertEqual(args.env, 'bar')
        self.assertEqual(args.debug, False)

    def test_parse_args_debug(self):
        args = parse_args(['foo', 'bar', '--debug'])
        self.assertEqual(args.debug, True)

