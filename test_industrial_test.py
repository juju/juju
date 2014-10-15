__metaclass__ = type

from contextlib import contextmanager
import os.path
from StringIO import StringIO
from unittest import TestCase

from mock import patch
import yaml

from industrial_test import (
    BootstrapAttempt,
    IndustrialTest,
    parse_args,
    StageAttempt,
    )
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


class TestParseArgs(TestCase):

    def test_parse_args(self):
        with parse_error(self) as stderr:
            args = parse_args([])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        with parse_error(self) as stderr:
            args = parse_args(['env'])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        args = parse_args(['rai', 'new-juju'])
        self.assertEqual(args.env, 'rai')
        self.assertEqual(args.new_juju_path, 'new-juju')


class FakeAttempt:

    def __init__(self, *result):
        self.result = result

    def do_stage(self):
        return self.result


class TestIndustrialTest(TestCase):

    def test_init(self):
        old_client = object()
        new_client = object()
        attempt_list = []
        industrial = IndustrialTest(old_client, new_client, attempt_list)
        self.assertIs(old_client, industrial.old_client)
        self.assertIs(new_client, industrial.new_client)
        self.assertIs(attempt_list, industrial.stage_attempts)

    def test_from_args(self):
        side_effect = lambda x, y=None: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       return_value='env-name2'):
                industrial = IndustrialTest.from_args(
                    'env-name', 'new-juju-path', [])
        self.assertIsInstance(industrial, IndustrialTest)
        self.assertEqual(industrial.old_client, ('env-name2', None))
        self.assertEqual(industrial.new_client, ('env-name2', 'new-juju-path'))

    def test_run_attempt_success(self):
        old_client = object()
        new_client = object()
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(True, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_attempt()
        self.assertItemsEqual(result, [(True, True), (True, True)])
        self.assertEqual(len(cc_mock.mock_calls), 0)

    def test_run_attempt_old_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(False, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_attempt()
            self.assertItemsEqual(result, [(False, True)])
        assert_juju_call(self, cc_mock, old_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'new', '--force', '-y'), 1)

    def test_run_attempt_new_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(True, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_attempt()
            self.assertItemsEqual(result, [(True, False)])
        assert_juju_call(self, cc_mock, old_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'new', '--force', '-y'), 1)

    def test_run_attempt_both_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(False, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_attempt()
            self.assertItemsEqual(result, [(False, False)])
        assert_juju_call(self, cc_mock, old_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'new', '--force', '-y'), 1)

    def test_destroy_both_even_with_exception(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(False, False), FakeAttempt(True, True)])
        with patch.object(old_client, 'destroy_environment',
                          side_effect=Exception) as oc_mock:
            with patch.object(new_client, 'destroy_environment',
                              side_effect=Exception) as nc_mock:
                with self.assertRaises(Exception):
                    list(industrial.run_attempt())
        oc_mock.assert_called_once_with()
        nc_mock.assert_called_once_with()


class TestStageAttempt(TestCase):

    def test_do_stage(self):

        class StubSA(StageAttempt):

            def __init__(self):
                super(StageAttempt, self).__init__()
                self.did_op = []

            def do_operation(self, client):
                self.did_op.append(client)

            def get_result(self, client):
                return self.did_op.index(client)


        attempt = StubSA()
        old = object()
        new = object()
        result = attempt.do_stage(old, new)
        self.assertEqual([old, new], attempt.did_op)
        self.assertEqual(result, (0, 1))


class FakeEnvJujuClient(EnvJujuClient):

    def __init__(self, name='steve'):
        super(FakeEnvJujuClient, self).__init__(
            SimpleEnvironment(name), '1.2', '/jbin/juju')

    def wait_for_started(self):
        with patch('sys.stdout'):
            super(FakeEnvJujuClient, self).wait_for_started(0.01)

    def juju(self, *args, **kwargs):
        # Suppress stdout for juju commands.
        with patch('sys.stdout'):
            return super(FakeEnvJujuClient, self).juju(*args, **kwargs)



def assert_juju_call(test_case, mock_method, client, expected_args,
                     call_index=None):
    if call_index is None:
        test_case.assertEqual(len(mock_method.mock_calls), 1)
        call_index = 0
    empty, args, kwargs = mock_method.mock_calls[call_index]
    test_case.assertEqual(args, (expected_args,))
    test_case.assertEqual(kwargs.keys(), ['env'])
    bin_dir = os.path.dirname(client.full_path)
    test_case.assertRegexpMatches(kwargs['env']['PATH'],
    r'^{}\:'.format(bin_dir))


class TestBootstrapAttempt(TestCase):

    def test_do_operation(self):
        client = FakeEnvJujuClient()
        bootstrap = BootstrapAttempt()
        with patch('subprocess.check_call') as mock_cc:
            bootstrap.do_operation(client)
        assert_juju_call(self, mock_cc, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'steve',
            '--constraints', 'mem=2G'))

    def test_get_result_true(self):
        bootstrap = BootstrapAttempt()
        client = FakeEnvJujuClient()
        output = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            self.assertTrue(bootstrap.get_result(client))

    def test_get_result_false(self):
        bootstrap = BootstrapAttempt()
        client = FakeEnvJujuClient()
        output = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'pending'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            self.assertFalse(bootstrap.get_result(client))
