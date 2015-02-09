from subprocess import CalledProcessError
from textwrap import dedent
from unittest import TestCase

from assess_recovery import (
    parse_new_state_server_from_error,
    parse_args,
)


class AssessRecoveryTestCase(TestCase):

    def test_parse_new_state_server_from_error(self):
        output = dedent("""
            Waiting for address
            Attempting to connect to 10.0.0.202:22
            Attempting to connect to 1.2.3.4:22
            The fingerprint for the ECDSA key sent by the remote host is
            """)
        error = CalledProcessError(1, ['foo'], output)
        address = parse_new_state_server_from_error(error)
        self.assertEqual('1.2.3.4', address)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['foo', 'bar', 'baz'])
        self.assertEqual(args.juju_path, 'foo')
        self.assertEqual(args.env_name, 'bar')
        self.assertEqual(args.logs, 'baz')
        self.assertEqual(args.charm_prefix, '')
        self.assertEqual(args.strategy, 'backup')
        self.assertEqual(args.debug, False)

    def test_parse_args_ha(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha'])
        self.assertEqual(args.strategy, 'ha')

    def test_parse_args_ha_backup(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha-backup'])
        self.assertEqual(args.strategy, 'ha-backup')

    def test_parse_args_backup(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha', '--backup'])
        self.assertEqual(args.strategy, 'backup')

    def test_parse_args_charm_prefix(self):
        args = parse_args(['foo', 'bar', 'baz', '--charm-prefix', 'qux'])
        self.assertEqual(args.charm_prefix, 'qux')

    def test_parse_args_debug(self):
        args = parse_args(['foo', 'bar', 'baz', '--debug'])
        self.assertEqual(args.debug, True)
