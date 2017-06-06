"""Tests for assess_log_forward module."""

import argparse
from mock import patch
import re
import StringIO

import assess_log_forward as alf
from tests import (
    parse_error,
    TestCase,
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = alf.parse_args(
            ['an-env', '/bin/juju', '/tmp/logs', 'an-env-mod'])
        self.assertEqual(
            args,
            argparse.Namespace(
                env='an-env',
                juju_bin='/bin/juju',
                temp_env_name='an-env-mod',
                debug=False,
                agent_stream=None,
                agent_url=None,
                bootstrap_host=None,
                keep_env=False,
                logs='/tmp/logs',
                machine=[],
                region=None,
                series=None,
                to=None,
                upload_tools=False,
                verbose=20,
                deadline=None,
                ))

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                alf.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn(
            'Test log forwarding of logs.',
            fake_stdout.getvalue())


class TestAssertRegex(TestCase):

    def test_default_message_check(self):
        self.assertTrue(
            alf.get_assert_regex('').endswith('.*$"'))

    def test_fails_when_uuid_doesnt_match(self):
        uuid = 'fail'
        check = alf.get_assert_regex(uuid)
        failing_string = 'abc'

        self.assertIsNone(re.search(check, failing_string))

    def test_succeeds_with_matching_uuid(self):
        uuid = '1234567812345678'
        short_uuid = uuid[:-8]
        check = alf.get_assert_regex(
            uuid, message='abc').rstrip('"').strip('"')
        success = 'Jul 13 00:00:00 machine-0.{} '\
                  'jujud-machine-agent-{} abc'.format(
                      uuid, short_uuid)

        self.assertIsNotNone(
            re.search(check, success))


class TestAddPortToIP(TestCase):
    def test_adds_port_to_ipv4(self):
        ip_address = '192.168.1.1'
        port = '123'
        expected = '192.168.1.1:123'
        self.assertEqual(
            alf.add_port_to_ip(ip_address, port),
            expected
        )

    def test_adds_port_to_ipv6(self):
        ip_address = '1fff:0:a88:85a3::ac1f'
        port = '123'
        expected = '[1fff:0:a88:85a3::ac1f]:123'
        self.assertEqual(
            alf.add_port_to_ip(ip_address, port),
            expected
        )

    def test_raises_ValueError_on_invalid_address(self):
        with self.assertRaises(ValueError):
            alf.add_port_to_ip('abc', 'abc')
