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
from jujuci import Credentials
from schedule_reliability_tests import (
    build_job,
    parse_args,
    )
from test_utility import (
    parse_error,
    write_config,
    )
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args, credentials = parse_args([
            'foo', '--user', 'jrandom', '--password', 'password1'])
        self.assertEqual(
            args,
            Namespace(root_dir='foo', suite=FULL, jobs=None, user='jrandom',
                      password='password1'))
        self.assertEqual(credentials,
                         Credentials(user='jrandom', password='password1'))

    def test_parse_args_suite_supports_known_suites(self):
        for suite in suites.keys():
            self.assertEqual(parse_args(['foo', '--suite', suite])[0],
                             Namespace(root_dir='foo', suite=suite, jobs=None,
                                       user=None, password=None))

    def test_parse_args_bad_suite(self):
        with parse_error(self) as stderr:
            parse_args(['foo', '--suite', 'foo'])
        self.assertRegexpMatches(stderr.getvalue(),
                                 ".*invalid choice: 'foo'.*")

    def test_parse_jobs(self):
        self.assertEqual(
            parse_args(['foo', 'bar'])[0],
            Namespace(root_dir='foo', suite=FULL, jobs=['bar'], user=None,
                      password=None))
        self.assertEqual(
            parse_args(['foo', 'bar', 'baz'])[0],
            Namespace(root_dir='foo', suite=FULL, jobs=['bar', 'baz'],
                      user=None, password=None))


class TestBuildJob(TestCase):

    def test_build_job(self):
        with patch('schedule_reliability_tests.Jenkins') as jenkins_mock:
            with temp_dir() as root:
                write_config(root, 'foo', 'quxxx')
                build_job(Credentials('jrandom', 'password1'), root, 'foo',
                          ['bar', 'baz'], 'qux')
        jenkins_mock.assert_called_once_with(
            'http://localhost:8080', 'jrandom', 'password1')
        calls = jenkins_mock.return_value.build_job.mock_calls
        expected = [
            call('foo', {'suite': 'qux', 'attempts': '10',
                         'new_juju_dir': 'bar'}, token='quxxx'),
            call('foo', {'suite': 'qux', 'attempts': '10',
                         'new_juju_dir': 'baz'}, token='quxxx'),
            ]
        self.assertEqual(calls, expected)
