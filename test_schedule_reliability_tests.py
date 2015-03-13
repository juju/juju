from unittest import TestCase
from argparse import Namespace

from mock import (
    call,
    patch,
    )

from industrial_test import (
    FULL,
    suites,
    UPGRADE,
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
            Namespace(root_dir='foo', suite=[], jobs=None, user='jrandom',
                      password='password1'))
        self.assertEqual(credentials,
                         Credentials(user='jrandom', password='password1'))

    def test_parse_args_suite_supports_known_suites(self):
        for suite in suites.keys():
            self.assertEqual(
                parse_args([
                    'foo', '--suite', suite, '--user', 'u', '--password',
                    'p'])[0],
                Namespace(root_dir='foo', suite=[suite], jobs=None,
                          user='u', password='p'))

    def test_parse_args_bad_suite(self):
        with parse_error(self) as stderr:
            parse_args(['foo', '--suite', 'foo'])
        self.assertRegexpMatches(stderr.getvalue(),
                                 ".*invalid choice: 'foo'.*")

    def test_parse_args_multi_suite(self):
        args = parse_args(['foo', '--suite', FULL, '--suite', UPGRADE,
                           '--user=u', '--password=p'])[0]
        self.assertEqual(args.suite, [FULL, UPGRADE])

    def test_parse_jobs(self):
        self.assertEqual(
            parse_args(['foo', 'bar', '--user', 'jrandom', '--password',
                        'password1'])[0],
            Namespace(root_dir='foo', suite=[], jobs=['bar'], user='jrandom',
                      password='password1'))
        self.assertEqual(
            parse_args(['foo', 'bar', 'baz', '--user', 'jrandom',
                        '--password', 'password1'])[0],
            Namespace(root_dir='foo', suite=[], jobs=['bar', 'baz'],
                      user='jrandom', password='password1'))


class TestBuildJob(TestCase):

    def test_build_job(self):
        jenkins_cxt = patch('schedule_reliability_tests.Jenkins')
        with jenkins_cxt as jenkins_mock, temp_dir() as root:
            write_config(root, 'foo', 'quxxx')
            build_job(Credentials('jrandom', 'password1'), root, 'foo',
                      ['bar', 'baz'], ['qux'])
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

    def test_build_job_multi_suite(self):
        jenkins_cxt = patch('schedule_reliability_tests.Jenkins')
        with jenkins_cxt as jenkins_mock, temp_dir() as root:
            write_config(root, 'foo', 'bar')
            build_job(Credentials('jrandom', 'password1'), root, 'foo',
                      ['baz'], ['qux', 'quxx'])
        jenkins_mock.return_value.build_job.assert_called_once_with(
            'foo', {'suite': 'qux,quxx', 'attempts': '10',
                    'new_juju_dir': 'baz'}, token='bar')
