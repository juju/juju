__metaclass__ = type

from contextlib import contextmanager
import os.path
from StringIO import StringIO
from textwrap import dedent
from unittest import TestCase

from mock import patch
import yaml

from industrial_test import (
    BootstrapAttempt,
    DestroyEnvironmentAttempt,
    EnsureAvailabilityAttempt,
    IndustrialTest,
    MultiIndustrialTest,
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

    def test_parse_args_attempts(self):
        args = parse_args(['rai', 'new-juju'])
        self.assertEqual(args.attempts, 2)
        args = parse_args(['rai', 'new-juju', '--attempts', '3'])
        self.assertEqual(args.attempts, 3)


class FakeAttempt:

    def __init__(self, *result):
        self.result = result

    def do_stage(self, old_client, new_client):
        return self.result


class FakeAttemptClass:

    def __init__(self, title, *result):
        self.title = title
        self.result = result

    def __call__(self):
        return FakeAttempt(*self.result)


class TestMultiIndustrialTest(TestCase):

    def test_init(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 5)
        self.assertEqual(mit.env, 'foo-env')
        self.assertEqual(mit.new_juju_path, 'bar-path')
        self.assertEqual(mit.stages, [DestroyEnvironmentAttempt,
                                      BootstrapAttempt])
        self.assertEqual(mit.attempt_count, 5)

    def test_make_results(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 5)
        results = mit.make_results()
        self.assertEqual(results, [
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'destroy environment'},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'bootstrap'},
        ])

    def test_make_industrial_test(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 5)
        side_effect = lambda x, y=None: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                industrial = mit.make_industrial_test()
        self.assertEqual(industrial.old_client,
                         (SimpleEnvironment('foo-env-old', {}), None))
        self.assertEqual(industrial.new_client,
                         (SimpleEnvironment('foo-env-new', {}), 'bar-path'))
        self.assertEqual(len(industrial.stage_attempts), 2)
        for stage, attempt in zip(mit.stages, industrial.stage_attempts):
            self.assertIs(type(attempt), stage)

    def test_update_results(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 2)
        results = mit.make_results()
        mit.update_results([(True, False)], results)
        self.assertEqual(results, [
            {'title': 'destroy environment', 'attempts': 1, 'new_failures': 1,
             'old_failures': 0},
            {'title': 'bootstrap', 'attempts': 0, 'new_failures': 0,
             'old_failures': 0},
            ])
        mit.update_results([(True, True), (False, True)], results)
        self.assertEqual(results, [
            {'title': 'destroy environment', 'attempts': 2, 'new_failures': 1,
             'old_failures': 0},
            {'title': 'bootstrap', 'attempts': 1, 'new_failures': 0,
             'old_failures': 1},
            ])
        mit.update_results([(False, False), (False, False)], results)
        self.assertEqual(results, [
            {'title': 'destroy environment', 'attempts': 2, 'new_failures': 1,
             'old_failures': 0},
            {'title': 'bootstrap', 'attempts': 2, 'new_failures': 1,
             'old_failures': 2},
            ])

    def test_run_tests(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            FakeAttemptClass('foo', True, True),
            FakeAttemptClass('bar', True, False),
            ], 5)
        class StubJujuClient:

            def destroy_environment(self):
                pass
        side_effect = lambda x, y=None: StubJujuClient()
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                results = mit.run_tests()
        self.assertEqual(results, [
            {'title': 'foo', 'attempts': 5, 'old_failures': 0,
             'new_failures': 0},
            {'title': 'bar', 'attempts': 5, 'old_failures': 0,
             'new_failures': 5},
            ])

    def test_results_table(self):
        results = [
            {'title': 'foo', 'attempts': 5, 'old_failures': 1,
             'new_failures': 2},
            {'title': 'bar', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4},
            ]
        self.assertEqual(''.join(MultiIndustrialTest.results_table(results)),
            dedent("""\
                old failure | new failure | attempt | title
                          1 |           2 |       5 | foo
                          3 |           4 |       5 | bar
            """))

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
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                industrial = IndustrialTest.from_args(
                    'foo', 'new-juju-path', [])
        self.assertIsInstance(industrial, IndustrialTest)
        self.assertEqual(industrial.old_client,
                         (SimpleEnvironment('foo-old', {}), None))
        self.assertEqual(industrial.new_client,
                         (SimpleEnvironment('foo-new', {}), 'new-juju-path'))
        self.assertNotEqual(industrial.old_client[0].environment,
                            industrial.new_client[0].environment)

    def test_run_stages(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(True, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [(True, True), (True, True)])
        self.assertEqual(len(cc_mock.mock_calls), 0)

    def test_run_stages_old_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(False, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [(False, True)])
        assert_juju_call(self, cc_mock, old_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'new', '--force', '-y'), 1)

    def test_run_stages_new_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(True, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [(True, False)])
        assert_juju_call(self, cc_mock, old_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('juju', '--show-log', 'destroy-environment',
                          'new', '--force', '-y'), 1)

    def test_run_stages_both_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client,
                       [FakeAttempt(False, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
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
        attempt = industrial.run_stages()
        with patch.object(old_client, 'destroy_environment',
                          side_effect=Exception) as oc_mock:
            with patch.object(new_client, 'destroy_environment',
                              side_effect=Exception) as nc_mock:
                with self.assertRaises(Exception):
                    list(attempt)
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
            SimpleEnvironment(name, {'type': 'fake'}), '1.2', '/jbin/juju')

    def wait_for_started(self):
        with patch('sys.stdout'):
            super(FakeEnvJujuClient, self).wait_for_started(0.01)

    def wait_for_ha(self):
        with patch('sys.stdout'):
            super(FakeEnvJujuClient, self).wait_for_ha(0.01)

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

    def test_do_operation_exception(self):
        client = FakeEnvJujuClient()
        bootstrap = BootstrapAttempt()
        with patch('subprocess.check_call', side_effect=Exception
                ) as mock_cc:
            bootstrap.do_operation(client)
        assert_juju_call(self, mock_cc, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'steve',
            '--constraints', 'mem=2G'))
        output = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            self.assertFalse(bootstrap.get_result(client))

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


class TestDestroyEnvironmentAttempt(TestCase):

    def test_do_operation(self):
        client = FakeEnvJujuClient()
        bootstrap = DestroyEnvironmentAttempt()
        with patch('subprocess.check_call') as mock_cc:
            bootstrap.do_operation(client)
        assert_juju_call(self, mock_cc, client, (
            'juju', '--show-log', 'destroy-environment', '-y', 'steve'))


class TestEnsureAvailabilityAttempt(TestCase):

    def test__operation(self):
        client = FakeEnvJujuClient()
        ensure_av = EnsureAvailabilityAttempt()
        with patch('subprocess.check_call') as mock_cc:
            ensure_av._operation(client)
        assert_juju_call(self, mock_cc, client, (
            'juju', '--show-log', 'ensure-availability', '-e', 'steve', '-n',
            '3'))

    def test__result_true(self):
        ensure_av = EnsureAvailabilityAttempt()
        client = FakeEnvJujuClient()
        output = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
                '2': {'state-server-member-status': 'has-vote'},
                },
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            self.assertTrue(ensure_av.get_result(client))

    def test__result_false(self):
        ensure_av = EnsureAvailabilityAttempt()
        client = FakeEnvJujuClient()
        output = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
                },
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            with self.assertRaisesRegexp(
                Exception, 'Timed out waiting for voting to be enabled.'):
                    ensure_av._result(client)
