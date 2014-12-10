from unittest import TestCase
from argparse import Namespace

from mock import (
    call,
    patch,
    )

from industrial_test import (
    FULL,
    suites,
    )
from schedule_reliability_tests import (
    build_job,
    parse_args,
    )
from test_utility import write_config
from utility import (
    parse_error,
    temp_dir,
    )


class TestParseArgs(TestCase):

    def test_parse_args(self):
        self.assertEqual(
            parse_args(['foo']),
            Namespace(root_dir='foo', suite=FULL, jobs=None))

    def test_parse_args_suite_supports_known_suites(self):
        for suite in suites.keys():
            self.assertEqual(parse_args(['foo', '--suite', suite]),
                             Namespace(root_dir='foo', suite=suite, jobs=None))

    def test_parse_args_bad_suite(self):
        with parse_error(self) as stderr:
            parse_args(['foo', '--suite', 'foo'])
        self.assertRegexpMatches(stderr.getvalue(),
                                 ".*invalid choice: 'foo'.*")

    def test_parse_jobs(self):
        self.assertEqual(
            parse_args(['foo', 'bar']),
            Namespace(root_dir='foo', suite=FULL, jobs=['bar']))
        self.assertEqual(
            parse_args(['foo', 'bar', 'baz']),
            Namespace(root_dir='foo', suite=FULL, jobs=['bar', 'baz']))


class TestBuildJob(TestCase):

    def test_build_job(self):
        with patch('schedule_reliability_tests.Jenkins') as jenkins_mock:
            with temp_dir() as root:
                write_config(root, 'foo', 'quxxx')
                build_job(root, 'foo', ['bar', 'baz'], 'qux')
        calls = jenkins_mock.return_value.build_job.mock_calls
        expected = [
            call('foo', {'suite': 'qux', 'attempts': '10',
                         'new_juju_dir': 'bar'}, token='quxxx'),
            call('foo', {'suite': 'qux', 'attempts': '10',
                         'new_juju_dir': 'baz'}, token='quxxx'),
            ]
        self.assertEqual(calls, expected)
