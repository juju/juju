from contextlib import contextmanager
import os
from subprocess import CalledProcessError
from textwrap import dedent
from unittest import TestCase

from mock import patch

from assess_recovery import (
    make_client,
    parse_new_state_server_from_error,
    parse_args,
)
from jujupy import _temp_env as temp_env
from utility import temp_dir


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


class TestMakeClient(TestCase):

    @contextmanager
    def make_client_cxt(self):
        td = temp_dir()
        te = temp_env({'environments': {'foo': {
            'orig-name': 'foo', 'name': 'foo'}}})
        with td as juju_path, te, patch('subprocess.Popen',
                side_effect=ValueError):
            with patch('subprocess.check_output') as co_mock:
                co_mock.return_value = '1.18'
                yield juju_path

    def test_make_client(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, False, 'foo', 'bar')
        self.assertEqual(client.full_path, os.path.join(juju_path, 'juju'))
        self.assertEqual(client.debug, False)
        self.assertEqual(client.env.config['orig-name'], 'foo')
        self.assertEqual(client.env.config['name'], 'bar')
        self.assertEqual(client.env.environment, 'bar')

    def test_make_client_debug(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, True, 'foo', 'bar')
        self.assertEqual(client.debug, True)

    def test_make_client_no_temp_env_name(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, False, 'foo', None)
        self.assertEqual(client.full_path, os.path.join(juju_path, 'juju'))
        self.assertEqual(client.env.config['orig-name'], 'foo')
        self.assertEqual(client.env.config['name'], 'foo')
        self.assertEqual(client.env.environment, 'foo')


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

    def test_parse_args_temp_env_name(self):
        args = parse_args(['foo', 'bar', 'baz'])
        self.assertIs(args.temp_env_name, None)
        args = parse_args(['foo', 'bar', 'baz', 'qux'])
        self.assertEqual(args.temp_env_name, 'qux')
