from argparse import Namespace
import os
from unittest import TestCase

from mock import (
    call,
    patch,
    )

from schedule_chaos_tests import(
    parse_args,
    main,
    start_job,
    )
from test_utility import (
    make_candidate_dir,
    write_config,
    )
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['--user', 'me', '--password', 'pw',
                           'chaos', '/foo/bar', '7'])
        self.assertEqual(args, Namespace(all=False, count=7, job='chaos',
                                         password='pw', root_dir='/foo/bar',
                                         user='me'))

    @patch('sys.stderr')
    def test_parse_args_missing_user(self, *args):
        with self.assertRaises(SystemExit) as am:
            parse_args(['--password', 'pw', 'chaos', '/foo/bar', '7'])
        exception = am.exception
        self.assertEqual(exception.code, 2)

    @patch('sys.stderr')
    def test_parse_args_missing_password(self, *args):
        with self.assertRaises(SystemExit) as am:
            parse_args(['--user', 'me', 'chaos', '/foo/bar', '7'])
        exception = am.exception
        self.assertEqual(exception.code, 2)


class TestStartJob(TestCase):

    @patch('schedule_chaos_tests.Jenkins')
    def test_start_job(self, j_mock):
        with temp_dir() as root:
            write_config(root, 'foo', 'token_str')
            start_job(root, 'foo', '/some/path', 'me', 'pw', 1)
        j_mock.assert_called_once_with(
            'http://juju-ci.vapour.ws:8080', 'me', 'pw')
        calls = j_mock.return_value.build_job.mock_calls
        expected = [
            call('foo', {'juju_bin': '/some/path', 'sequence_number': 1})]
        self.assertEqual(calls, expected)


class TestScheduleChaos(TestCase):

    @patch('schedule_chaos_tests.start_job')
    def test_schedule_chaos_tests(self, s_mock):
        with temp_dir() as root:
            make_candidate_dir(root, 'branch')
            main(['--user', 'me', '--password', 'pw', 'chaos', root, '3'])
        calls = s_mock.mock_calls
        juju_bin = os.path.join(root,
                                'candidate', 'branch', 'usr', 'foo', 'juju')
        expected = [
            call(root, 'chaos', juju_bin, 'me', 'pw', 0),
            call(root, 'chaos', juju_bin, 'me', 'pw', 1),
            call(root, 'chaos', juju_bin, 'me', 'pw', 2)]
        self.assertEqual(calls, expected)
