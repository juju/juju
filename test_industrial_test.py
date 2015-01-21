__metaclass__ = type

from argparse import Namespace
from contextlib import contextmanager
import os
from tempfile import NamedTemporaryFile
from textwrap import dedent
from unittest import TestCase

from boto.ec2.securitygroup import SecurityGroup
from mock import (
    call,
    MagicMock,
    patch,
    )
import yaml

from industrial_test import (
    BACKUP,
    BackupRestoreAttempt,
    BootstrapAttempt,
    DENSITY,
    DeployManyAttempt,
    DeployManyFactory,
    DestroyEnvironmentAttempt,
    EnsureAvailabilityAttempt,
    FULL,
    IndustrialTest,
    make_substrate,
    maybe_write_json,
    MultiIndustrialTest,
    parse_args,
    QUICK,
    StageAttempt,
    SteppedStageAttempt,
    )
from jujuconfig import get_euca_env
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    _temp_env,
    )
from substrate import AWSAccount
from test_jujupy import assert_juju_call
from test_substrate import (
    get_aws_env,
    get_os_config,
    make_os_security_group_instance,
    make_os_security_groups,
    )
from test_utility import parse_error


def iter_steps_validate_info(test, stage, client):
    """Proxy a steps iterator to and compare with get_test_info output.

    Unexpected steps, or steps in the wrong order will raise an exception.

    :param test: A unittest.TestCase
    :param stage: The SteppedStageAttempt to test.
    :param client: The EnvJujuClient to use for iter_steps.
    """
    step_iter = stage.iter_steps(client)
    test_ids = stage.get_test_info().keys()
    result = step_iter.next()
    unexpected = True
    for test_id in test_ids:
        while result['test_id'] == test_id:
            unexpected = False
            yield result
            result = step_iter.next()
            unexpected = True
    test.assertFalse(
        unexpected, 'Unexpected test_id: {}'.format(result['test_id']))


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
        with parse_error(self) as stderr:
            args = parse_args(['rai', 'new-juju'])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        args = parse_args(['rai', 'new-juju', QUICK])
        self.assertEqual(args.env, 'rai')
        self.assertEqual(args.new_juju_path, 'new-juju')
        self.assertEqual(args.suite, QUICK)

    def test_parse_args_attempts(self):
        args = parse_args(['rai', 'new-juju', QUICK])
        self.assertEqual(args.attempts, 2)
        args = parse_args(['rai', 'new-juju', '--attempts', '3', QUICK])
        self.assertEqual(args.attempts, 3)

    def test_parse_args_json_file(self):
        args = parse_args(['rai', 'new-juju', QUICK])
        self.assertIs(args.json_file, None)
        args = parse_args(['rai', 'new-juju', '--json-file', 'foobar', QUICK])
        self.assertEqual(args.json_file, 'foobar')

    def test_parse_args_suite(self):
        args = parse_args(['rai', 'new-juju', 'full'])
        self.assertEqual(args.suite, FULL)
        args = parse_args(['rai', 'new-juju', QUICK])
        self.assertEqual(args.suite, QUICK)
        args = parse_args(['rai', 'new-juju', DENSITY])
        self.assertEqual(args.suite, DENSITY)
        args = parse_args(['rai', 'new-juju', BACKUP])
        self.assertEqual(args.suite, BACKUP)
        with parse_error(self) as stderr:
            args = parse_args(['rai', 'new-juju', 'foo'])
        self.assertRegexpMatches(
            stderr.getvalue(), ".*argument suite: invalid choice: 'foo'.*")

    def test_parse_args_agent_url(self):
        args = parse_args(['rai', 'new-juju', QUICK])
        self.assertEqual(args.new_agent_url, None)
        args = parse_args(['rai', 'new-juju', '--new-agent-url',
                           'http://example.org', QUICK])
        self.assertEqual(args.new_agent_url, 'http://example.org')


class FakeStepAttempt:

    def __init__(self, result):
        self.result = result

    def iter_test_results(self, old, new):
        return iter(self.result)


class FakeAttempt(FakeStepAttempt):

    def __init__(self, old_result, new_result, test_id='foo-id'):
        super(FakeAttempt, self).__init__([(test_id, old_result, new_result)])

    def do_stage(self, old_client, new_client):
        return self.result[0]


class FakeAttemptClass:

    def __init__(self, title, *result):
        self.title = title
        self.test_id = '{}-id'.format(title)
        self.result = result

    def get_test_info(self):
        return {self.test_id: {'title': self.title}}

    def __call__(self):
        return FakeAttempt(*self.result, test_id=self.test_id)


class StubJujuClient:

    def destroy_environment(self, delete_jenv=False):
        pass


@contextmanager
def temp_env(name, config=None):
    if config is None:
        config = {}
    environments = {'environments': {name: config}}
    with _temp_env(environments):
        yield


class TestMultiIndustrialTest(TestCase):

    def test_from_args(self):
        args = Namespace(env='foo', new_juju_path='new-path', attempts=7,
                         suite=QUICK, new_agent_url=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(mit.env, 'foo')
        self.assertEqual(mit.new_juju_path, 'new-path')
        self.assertEqual(mit.attempt_count, 7)
        self.assertEqual(mit.max_attempts, 14)
        self.assertEqual(
            mit.stages, [BootstrapAttempt, DestroyEnvironmentAttempt])
        args = Namespace(env='bar', new_juju_path='new-path2', attempts=6,
                         suite=FULL, new_agent_url=None)
        with temp_env('bar'):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(mit.env, 'bar')
        self.assertEqual(mit.new_juju_path, 'new-path2')
        self.assertEqual(mit.attempt_count, 6)
        self.assertEqual(mit.max_attempts, 12)
        self.assertEqual(
            mit.stages, [
                BootstrapAttempt, DeployManyAttempt, BackupRestoreAttempt,
                EnsureAvailabilityAttempt, DestroyEnvironmentAttempt])

    def test_from_args_maas(self):
        args = Namespace(env='foo', new_juju_path='new-path', attempts=7,
                         suite=DENSITY, new_agent_url=None)
        with temp_env('foo', {'type': 'maas'}):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(
            mit.stages, [
                BootstrapAttempt, DeployManyFactory(2, 2),
                DestroyEnvironmentAttempt])

    def test_get_stages(self):
        self.assertEqual(
            MultiIndustrialTest.get_stages(QUICK, {'type': 'foo'}),
            [BootstrapAttempt, DestroyEnvironmentAttempt])

        self.assertEqual(
            MultiIndustrialTest.get_stages(FULL, {'type': 'foo'}), [
                BootstrapAttempt, DeployManyAttempt, BackupRestoreAttempt,
                EnsureAvailabilityAttempt, DestroyEnvironmentAttempt])
        self.assertEqual(
            MultiIndustrialTest.get_stages(DENSITY, {'type': 'foo'}), [
                BootstrapAttempt, DeployManyAttempt,
                DestroyEnvironmentAttempt])
        self.assertEqual(
            MultiIndustrialTest.get_stages(BACKUP, {'type': 'foo'}), [
                BootstrapAttempt, BackupRestoreAttempt,
                DestroyEnvironmentAttempt])

    def test_get_stages_maas(self):
        self.assertEqual(
            MultiIndustrialTest.get_stages(QUICK, {'type': 'maas'}),
            [BootstrapAttempt, DestroyEnvironmentAttempt])
        self.assertEqual(
            MultiIndustrialTest.get_stages(FULL, {'type': 'maas'}), [
                BootstrapAttempt, DeployManyFactory(2, 2),
                BackupRestoreAttempt, EnsureAvailabilityAttempt,
                DestroyEnvironmentAttempt])
        self.assertEqual(
            MultiIndustrialTest.get_stages(DENSITY, {'type': 'maas'}), [
                BootstrapAttempt, DeployManyFactory(2, 2),
                DestroyEnvironmentAttempt])

    def test_density_suite(self):
        args = Namespace(env='foo', new_juju_path='new-path', attempts=7,
                         suite=DENSITY, new_agent_url=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(
            mit.stages, [BootstrapAttempt, DeployManyAttempt,
                         DestroyEnvironmentAttempt])

    def test_backup_suite(self):
        args = Namespace(env='foo', new_juju_path='new-path', attempts=7,
                         suite=BACKUP, new_agent_url=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(
            mit.stages, [BootstrapAttempt, BackupRestoreAttempt,
                         DestroyEnvironmentAttempt])

    def test_from_args_new_agent_url(self):
        args = Namespace(env='foo', new_juju_path='new-path', attempts=7,
                         suite=QUICK, new_agent_url='http://example.net')
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args)
        self.assertEqual(mit.new_agent_url, 'http://example.net')

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
        self.assertEqual(results, {'results': [
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'destroy environment', 'test_id': 'destroy-env'},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'check substrate clean', 'test_id': 'substrate-clean'},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'bootstrap', 'test_id': 'bootstrap'},
        ]})

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

    def test_make_industrial_test_new_agent_url(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [],
                                  new_agent_url='http://example.com')
        side_effect = lambda x, y=None: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                industrial = mit.make_industrial_test()
        self.assertEqual(
            industrial.new_client, (
                SimpleEnvironment('foo-env-new', {
                    'tools-metadata-url': 'http://example.com'}),
                'bar-path')
            )

    def test_update_results(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 2)
        results = mit.make_results()
        mit.update_results([('destroy-env', True, False)], results)
        expected = {'results': [
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 1, 'new_failures': 1, 'old_failures': 0},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 0, 'new_failures': 0, 'old_failures': 0},
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 0,
             'new_failures': 0, 'old_failures': 0},
            ]}
        self.assertEqual(results, expected)
        mit.update_results(
            [('destroy-env', True, True), ('substrate-clean', True, True),
             ('bootstrap', False, True)],
            results)
        self.assertEqual(results, {'results': [
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 2, 'new_failures': 1, 'old_failures': 0},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 1, 'new_failures': 0, 'old_failures': 0},
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 1,
             'new_failures': 0, 'old_failures': 1},
            ]})
        mit.update_results(
            [('destroy-env', False, False), ('substrate-clean', True, True),
             ('bootstrap', False, False)],
            results)
        expected = {'results': [
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 2, 'new_failures': 1, 'old_failures': 0},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 2, 'new_failures': 0, 'old_failures': 0},
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 2,
             'new_failures': 1, 'old_failures': 2},
            ]}
        self.assertEqual(results, expected)

    def test_run_tests(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            FakeAttemptClass('foo', True, True),
            FakeAttemptClass('bar', True, False),
            ], 5, 10)
        side_effect = lambda x, y=None: StubJujuClient()
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                results = mit.run_tests()
        self.assertEqual(results, {'results': [
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 0},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 5},
            ]})

    def test_run_tests_max_attempts(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            FakeAttemptClass('foo', True, False),
            FakeAttemptClass('bar', True, False),
            ], 5, 6)
        side_effect = lambda x, y=None: StubJujuClient()
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                results = mit.run_tests()
        self.assertEqual(results, {'results': [
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 5},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 0,
             'old_failures': 0, 'new_failures': 0},
            ]})

    def test_run_tests_max_attempts_less_than_attempt_count(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            FakeAttemptClass('foo', True, False),
            FakeAttemptClass('bar', True, False),
            ], 5, 4)
        side_effect = lambda x, y=None: StubJujuClient()
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                results = mit.run_tests()
        self.assertEqual(results, {'results': [
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 4,
             'old_failures': 0, 'new_failures': 4},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 0,
             'old_failures': 0, 'new_failures': 0},
            ]})

    def test_results_table(self):
        results = [
            {'title': 'foo', 'attempts': 5, 'old_failures': 1,
             'new_failures': 2},
            {'title': 'bar', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4},
            ]
        self.assertEqual(
            ''.join(MultiIndustrialTest.results_table(results)),
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
        industrial = IndustrialTest(old_client, new_client, [
            FakeAttempt(True, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [('foo-id', True, True),
                                           ('foo-id', True, True)])
        self.assertEqual(len(cc_mock.mock_calls), 0)

    def test_run_stages_old_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client, [
            FakeAttempt(False, True), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [('foo-id', False, True)])
        assert_juju_call(self, cc_mock, old_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'new', '--force', '-y'), 1)

    def test_run_stages_new_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client, [
            FakeAttempt(True, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [('foo-id', True, False)])
        assert_juju_call(self, cc_mock, old_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'new', '--force', '-y'), 1)

    def test_run_stages_both_fail(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client, [
            FakeAttempt(False, False), FakeAttempt(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [('foo-id', False, False)])
        assert_juju_call(self, cc_mock, old_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'old', '--force', '-y'), 0)
        assert_juju_call(self, cc_mock, new_client,
                         ('timeout', '600.00s', 'juju', '--show-log',
                          'destroy-environment', 'new', '--force', '-y'), 1)

    def test_run_stages_recover_failure(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        fsa = FakeStepAttempt([('foo', True, False), ('bar', True, True)])
        industrial = IndustrialTest(old_client, new_client, [
            fsa, FakeAttempt(True, True)])
        self.assertEqual(list(industrial.run_stages()), [
            ('foo', True, False), ('bar', True, True), ('foo-id', True, True)])

    def test_run_stages_failure_in_last_step(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        fsa = FakeStepAttempt([('foo', True, True), ('bar', False, True)])
        industrial = IndustrialTest(old_client, new_client, [
            fsa, FakeAttempt(True, True)])
        with patch.object(old_client, 'destroy_environment'):
            with patch.object(new_client, 'destroy_environment'):
                self.assertEqual(list(industrial.run_stages()), [
                    ('foo', True, True), ('bar', False, True)])

    def test_destroy_both_even_with_exception(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client, [
            FakeAttempt(False, False), FakeAttempt(True, True)])
        with patch.object(old_client, 'destroy_environment',
                          side_effect=Exception) as oc_mock:
            with patch.object(new_client, 'destroy_environment',
                              side_effect=Exception) as nc_mock:
                with self.assertRaises(Exception):
                    industrial.destroy_both()
        oc_mock.assert_called_once_with(delete_jenv=True)
        nc_mock.assert_called_once_with(delete_jenv=True)

    def test_run_attempt(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        attempt = FakeAttempt(True, True)
        industrial = IndustrialTest(old_client, new_client, [attempt])

        def iter_test_results():
            raise Exception
            yield

        with patch.object(attempt, 'iter_test_results', iter_test_results):
            with patch('logging.exception') as le_mock:
                with patch.object(industrial, 'destroy_both') as db_mock:
                    with self.assertRaises(SystemExit):
                        industrial.run_attempt()
        le_mock.assert_called_once()
        self.assertEqual(db_mock.mock_calls, [call(), call()])


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

    def test_get_test_info(self):

        class StubSA(StageAttempt):

            test_id = 'foo-bar'

            title = 'baz'

        self.assertEqual(StubSA.get_test_info(), {'foo-bar': {'title': 'baz'}})

    def test_iter_test_results(self):

        did_operation = [False]

        class StubSA(StageAttempt):

            test_id = 'foo-id'

            def do_stage(self, old, new):

                did_operation[0] = True
                return True, True

        for result in StubSA().iter_test_results(None, None):
            self.assertEqual(result, ('foo-id', True, True))


class TestSteppedStageAttempt(TestCase):

    def test__iter_for_result_premature_results(self):
        iterator = iter([{'test_id': 'foo-id', 'result': True}])
        with self.assertRaisesRegexp(ValueError, 'Result before declaration.'):
            list(SteppedStageAttempt._iter_for_result(iterator))

    def test__iter_for_result_many(self):
        iterator = iter([
            {'test_id': 'foo-id'},
            {'test_id': 'foo-id', 'result': True},
            {'test_id': 'bar-id'},
            {'test_id': 'bar-id', 'result': False},
            ])
        output = list(SteppedStageAttempt._iter_for_result(iterator))
        self.assertEqual(output, [
            None, {'test_id': 'foo-id', 'result': True}, None,
            {'test_id': 'bar-id', 'result': False}])

    def test__iter_for_result_exception(self):
        error = ValueError('Bad value')

        def iterator():
            yield {'test_id': 'foo-id'}
            raise error

        with patch('logging.exception') as le_mock:
            output = list(SteppedStageAttempt._iter_for_result(iterator()))
        self.assertEqual(output,
                         [None, {'test_id': 'foo-id', 'result': False}])
        le_mock.assert_called_once_with(error)

    def test_iter_for_result_id_change(self):
        iterator = iter([
            {'test_id': 'foo-id'}, {'test_id': 'bar-id'}])
        with self.assertRaisesRegexp(ValueError, 'ID changed without result.'):
            list(SteppedStageAttempt._iter_for_result(iterator))

    def test_iter_for_result_id_change_same_dict(self):

        def iterator():
            result = {'test_id': 'foo-id'}
            yield result
            result['test_id'] = 'bar-id'
            yield result

        with self.assertRaisesRegexp(ValueError, 'ID changed without result.'):
            list(SteppedStageAttempt._iter_for_result(iterator()))

    def test_iter_for_result_id_change_result(self):
        iterator = iter([
            {'test_id': 'foo-id'}, {'test_id': 'bar-id', 'result': True}])
        with self.assertRaisesRegexp(ValueError, 'ID changed without result.'):
            list(SteppedStageAttempt._iter_for_result(iterator))

    def test__iter_test_results_success(self):
        old_iter = iter([
            None, {'test_id': 'foo-id', 'result': True}])
        new_iter = iter([
            None, {'test_id': 'foo-id', 'result': False}])

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {'foo-id': {'title': 'foo-id'}}

        self.assertItemsEqual(
            StubSA._iter_test_results(old_iter, new_iter),
            [('foo-id', True, False)])

    def test__iter_test_results_interleaved(self):
        # Using a single iterator for both proves that they are interleaved.
        # Otherwise, we'd get Result before declaration.
        both_iter = iter([
            None, None,
            {'test_id': 'foo-id', 'result': True},
            {'test_id': 'foo-id', 'result': False},
            ])

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {'foo-id': {'title': 'foo-id'}}

        self.assertItemsEqual(
            StubSA._iter_test_results(both_iter, both_iter),
            [('foo-id', True, False)])

    def test__iter_test_results_id_mismatch(self):
        old_iter = iter([
            None, {'test_id': 'foo-id', 'result': True}])
        new_iter = iter([
            None, {'test_id': 'bar-id', 'result': False}])
        with self.assertRaisesRegexp(ValueError, 'Test id mismatch.'):
            list(SteppedStageAttempt._iter_test_results(old_iter, new_iter))

    def test__iter_test_results_many(self):
        old_iter = iter([
            None, {'test_id': 'foo-id', 'result': True},
            None, {'test_id': 'bar-id', 'result': False},
            ])
        new_iter = iter([
            None, {'test_id': 'foo-id', 'result': False},
            None, {'test_id': 'bar-id', 'result': False},
            ])

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {
                    'foo-id': {'title': 'foo-id'},
                    'bar-id': {'title': 'bar-id'},
                }
        self.assertItemsEqual(
            StubSA._iter_test_results(old_iter, new_iter),
            [('foo-id', True, False), ('bar-id', False, False)])

    def test_iter_test_results(self):
        old = FakeEnvJujuClient()
        new = FakeEnvJujuClient()
        error = ValueError('asdf')

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {
                    'test-1': {'title': 'test-1'},
                    'test-2': {'title': 'test-2'},
                    }

            def iter_steps(self, client):
                yield {'test_id': 'test-1'}
                yield {'test_id': 'test-1', 'result': client is old}
                yield {'test_id': 'test-2'}
                if client is not new:
                    raise error
                else:
                    yield {'test_id': 'test-2', 'result': True}

        with patch('logging.exception') as le_mock:
            self.assertItemsEqual(
                StubSA().iter_test_results(old, new),
                [('test-1', True, False), ('test-2', False, True)])
        le_mock.assert_called_once_with(error)


class FakeEnvJujuClient(EnvJujuClient):

    def __init__(self, name='steve'):
        super(FakeEnvJujuClient, self).__init__(
            SimpleEnvironment(name, {'type': 'fake'}), '1.2', '/jbin/juju')

    def wait_for_started(self, start=None):
        with patch('sys.stdout'):
            return super(FakeEnvJujuClient, self).wait_for_started(0.01,
                                                                   start=start)

    def wait_for_ha(self):
        with patch('sys.stdout'):
            return super(FakeEnvJujuClient, self).wait_for_ha(0.01)

    def juju(self, *args, **kwargs):
        # Suppress stdout for juju commands.
        with patch('sys.stdout'):
            return super(FakeEnvJujuClient, self).juju(*args, **kwargs)


class TestBootstrapAttempt(TestCase):

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        bootstrap = BootstrapAttempt()
        boot_iter = iter_steps_validate_info(self, bootstrap, client)
        self.assertEqual(boot_iter.next(), {'test_id': 'bootstrap'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(boot_iter.next(), {'test_id': 'bootstrap'})
        assert_juju_call(self, mock_cc, client, (
            'juju', '--show-log', 'bootstrap', '-e', 'steve',
            '--constraints', 'mem=2G'))
        statuses = (yaml.safe_dump(x) for x in [
            {'machines': {'0': {'agent-state': 'pending'}}, 'services': {}},
            {'machines': {'0': {'agent-state': 'started'}}, 'services': {}},
            ])
        with patch('subprocess.check_output',
                   side_effect=lambda x, **y: statuses.next()) as mock_co:
            self.assertEqual(boot_iter.next(),
                             {'test_id': 'bootstrap', 'result': True})
        for num in range(2):
            assert_juju_call(self, mock_co, client, (
                'juju', '--show-log', 'status', '-e', 'steve'), num,
                assign_stderr=True)


class TestDestroyEnvironmentAttempt(TestCase):

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
        with patch('subprocess.call') as mock_cc:
            with patch.object(destroy_env, 'get_security_groups') as gsg_mock:
                self.assertEqual(iterator.next(), {
                    'test_id': 'destroy-env', 'result': True})
        gsg_mock.assert_called_once_with(client)
        assert_juju_call(self, mock_cc, client, (
            'timeout', '600.00s', 'juju', '--show-log', 'destroy-environment',
            'steve', '-y'))
        self.assertEqual(iterator.next(), {'test_id': 'substrate-clean'})
        with patch.object(destroy_env, 'check_security_groups') as csg_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'substrate-clean', 'result': True})
        csg_mock.assert_called_once_with(client, gsg_mock.return_value)

    def test_iter_test_results(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        with patch('subprocess.check_call'):
            output = list(destroy_env.iter_test_results(client, client))
        self.assertEqual(output, [
            ('destroy-env', True, True), ('substrate-clean', True, True)])

    @staticmethod
    def get_aws_client():
        client = FakeEnvJujuClient()
        client.env = get_aws_env()
        return client

    @staticmethod
    def get_openstack_client():
        client = FakeEnvJujuClient()
        client.env.config = get_os_config()
        return client

    def test_get_security_groups_aws(self):
        client = self.get_aws_client()
        destroy_env = DestroyEnvironmentAttempt()
        yaml_instances = yaml.safe_dump({'machines': {
            'foo': {'instance-id': 'foo-id'},
            }})
        aws_instances = [
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='foo', name='bar'),
                ])]),
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='baz', name='qux'),
                SecurityGroup(id='quxx-id', name='quxx'),
                ])]),
        ]
        with patch(
                'substrate.AWSAccount.get_ec2_connection') as gec_mock:
            with patch('subprocess.check_output', return_value=yaml_instances):
                gai_mock = gec_mock.return_value.get_all_instances
                gai_mock.return_value = aws_instances
                self.assertEqual(destroy_env.get_security_groups(client), {
                    'baz': 'qux', 'foo': 'bar', 'quxx-id': 'quxx'
                    })
        gec_mock.assert_called_once_with()
        gai_mock.assert_called_once_with(instance_ids=['foo-id'])

    def test_get_security_groups_openstack(self):
        client = self.get_openstack_client()
        destroy_env = DestroyEnvironmentAttempt()
        yaml_instances = yaml.safe_dump({'machines': {
            'foo': {'instance-id': 'bar-id'},
            'bar': {'instance-id': 'baz-qux-id'},
            }})
        os_instances = [
            make_os_security_group_instance(['bar']),
            make_os_security_group_instance(['baz', 'qux']),
        ]
        with patch(
                'substrate.OpenStackAccount.get_client') as gc_mock:
            os_client = gc_mock.return_value
            os_client.servers.list.return_value = os_instances
            security_groups = make_os_security_groups(['bar', 'baz', 'qux'])
            os_client.security_groups.list.return_value = security_groups
            with patch('subprocess.check_output', return_value=yaml_instances):
                self.assertEqual(destroy_env.get_security_groups(client), {
                    'baz-id': 'baz', 'bar-id': 'bar', 'qux-id': 'qux'
                    })

        gc_mock.assert_called_once_with()
        os_client.servers.list.assert_called_once_with()

    def test_get_security_groups_non_aws(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        self.assertIs(destroy_env.get_security_groups(client), None)

    def test_check_security_groups_match(self):
        client = self.get_aws_client()
        destroy_env = DestroyEnvironmentAttempt()
        output = (
            'GROUP\tfoo-id\t\tfoo-group\n'
            'GROUP\tbaz-id\t\tbaz-group\n'
        )
        with patch('subprocess.check_output', return_value=output) as co_mock:
            with self.assertRaisesRegexp(
                Exception, (
                    r'Security group\(s\) not cleaned up: foo-group.')):
                    with patch('industrial_test.until_timeout',
                               lambda x: iter([None])):
                        destroy_env.check_security_groups(
                            client, {'foo-id': 'foo', 'bar-id': 'bar'})
        env = AWSAccount.from_config(client.env.config).get_environ()
        co_mock.assert_called_once_with(
            ['euca-describe-groups', '--filter', 'description=juju group'],
            env=env)

    def test_check_security_groups_no_match(self):
        client = self.get_aws_client()
        destroy_env = DestroyEnvironmentAttempt()
        output = (
            'GROUP\tfoo-id\t\tfoo-group\n'
            'GROUP\tbaz-id\t\tbaz-group\n'
        )
        with patch('subprocess.check_output', return_value=output) as co_mock:
                destroy_env.check_security_groups(
                    client, {'bar-id': 'bar'})
        env = AWSAccount.from_config(client.env.config).get_environ()
        co_mock.assert_called_once_with(
            ['euca-describe-groups', '--filter', 'description=juju group'],
            env=env)

    def test_check_security_groups_non_aws(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        with patch('subprocess.check_output') as co_mock:
                destroy_env.check_security_groups(
                    client, {'bar-id': 'bar'})
        self.assertEqual(co_mock.call_count, 0)


class TestEnsureAvailabilityAttempt(TestCase):

    def setUp(self):
        patcher = patch('jujupy.pause')
        self.addCleanup(patcher.stop)
        self.pause_mock = patcher.start()

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


class TestDeployManyAttempt(TestCase):

    def predict_add_machine_calls(self, deploy_many):
        for host in range(1, deploy_many.host_count + 1):
            for container in range(deploy_many.container_count):
                target = 'lxc:{}'.format(host)
                service = 'ubuntu{}x{}'.format(host, container)
                yield ('juju', '--show-log', 'deploy', '-e', 'steve', '--to',
                       target, 'ubuntu', service)

    def predict_remove_machine_calls(self, deploy_many):
        total_guests = deploy_many.host_count * deploy_many.container_count
        for guest in range(100, total_guests + 100):
            yield ('juju', '--show-log', 'remove-machine', '-e', 'steve',
                   '--force', str(guest))

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        deploy_many = DeployManyAttempt(9, 11)
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-e', 'steve'), index)

        status = yaml.safe_dump({
            'machines': dict((str(x), {'agent-state': 'started'})
                             for x in range(deploy_many.host_count + 1)),
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'add-machine-many', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        with patch('subprocess.check_output', return_value=status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'ensure-machines', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'deploy-many'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many'})

        calls = self.predict_add_machine_calls(deploy_many)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)
        service_names = []
        for host in range(1, deploy_many.host_count + 1):
            for container in range(deploy_many.container_count):
                service_names.append('ubuntu{}x{}'.format(host, container))
        services = dict((service_name, {
            'units': {
                'foo': {'machine': str(num + 100), 'agent-state': 'started'}
                }})
            for num, service_name in enumerate(service_names))
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': services,
            })
        with patch('subprocess.check_output', return_value=status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many', 'result': True})

        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'remove-machine-many-lxc'})
        with patch('subprocess.check_output', return_value=status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'remove-machine-many-lxc'})
        calls = self.predict_remove_machine_calls(deploy_many)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)
        statuses = (yaml.safe_dump(x) for x in [
            {'machines': {'100': {'agent-state': 'started'}}, 'services': {}},
            {'machines': {}, 'services': {}}])
        with patch('subprocess.check_output',
                   side_effect=lambda x, **y: statuses.next()) as mock_co:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-lxc', 'result': True})
        for num in range(2):
            assert_juju_call(self, mock_co, client, (
                'juju', '--show-log', 'status', '-e', 'steve'), num,
                assign_stderr=True)
        self.assertEqual(deploy_iter.next(), {
            'test_id': 'remove-machine-many-instance'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-instance'})
        for num in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'remove-machine', '-e', 'steve',
                str(num + 1)), num)

        statuses = (yaml.safe_dump(x) for x in [
            {'machines': {'1': {'agent-state': 'started'}}, 'services': {}},
            {'machines': {}, 'services': {}}])

        with patch('subprocess.check_output',
                   side_effect=lambda x, **y: statuses.next()) as mock_co:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-instance', 'result': True})
        for num in range(2):
            assert_juju_call(self, mock_co, client, (
                'juju', '--show-log', 'status', '-e', 'steve'), num,
                assign_stderr=True)

    @patch('logging.error')
    def test_iter_step_failure(self, le_mock):
        deploy_many = DeployManyAttempt()
        client = FakeEnvJujuClient()
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-e', 'steve'), index)

        status = yaml.safe_dump({
            'machines': dict((str(x), {'agent-state': 'started'})
                             for x in range(deploy_many.host_count + 1)),
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'add-machine-many', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        with patch('subprocess.check_output', return_value=status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'ensure-machines', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'deploy-many'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many'})
        output = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'pending'},
                },
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output):
            with self.assertRaisesRegexp(
                    Exception,
                    'Timed out waiting for agents to start in steve.'):
                deploy_iter.next()

    @patch('logging.error')
    def test_iter_step_add_machine_failure(self, le_mock):
        deploy_many = DeployManyAttempt()
        client = FakeEnvJujuClient()
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = yaml.safe_dump({
            'machines': {'0': {'agent-state': 'started'}},
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-e', 'steve'), index)

        status = yaml.safe_dump({
            'machines': dict((str(x), {'agent-state': 'pending'})
                             for x in range(deploy_many.host_count + 1)),
            'services': {},
            })
        with patch('subprocess.check_output', return_value=status):
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'add-machine-many', 'result': False})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        status = yaml.safe_dump({
            'machines': dict((str(x), {'agent-state': 'started'})
                             for x in range(deploy_many.host_count + 1)),
            'services': {},
            })
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual({'test_id': 'ensure-machines'},
                             deploy_iter.next())
        for x in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'destroy-machine', '-e', 'steve',
                '--force', str((x + 1))), x * 2)
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-e', 'steve'), x * 2 + 1)
        with patch('subprocess.check_output', return_value=status):
            self.assertEqual({'test_id': 'ensure-machines', 'result': True},
                             deploy_iter.next())
        self.assertEqual({'test_id': 'deploy-many'}, deploy_iter.next())
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual({'test_id': 'deploy-many'}, deploy_iter.next())
        calls = self.predict_add_machine_calls(deploy_many)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)


class TestDeployManyFactory(TestCase):

    def test_get_test_info(self):
        self.assertEqual(DeployManyFactory(2, 2).get_test_info(),
                         DeployManyAttempt.get_test_info())

    def test_call(self):
        factory = DeployManyFactory(4, 2)
        self.assertEqual(factory(), DeployManyAttempt(4, 2))


class TestBackupRestoreAttempt(TestCase):

    def test_get_test_info(self):
        self.assertEqual(
            BackupRestoreAttempt.get_test_info(),
            {'back-up-restore': {'title': 'Back-up / restore'}})

    def test_iter_steps(self):
        br_attempt = BackupRestoreAttempt()
        client = FakeEnvJujuClient()
        client.env = get_aws_env()
        environ = dict(os.environ)
        environ.update(get_euca_env(client.env.config))

        def check_output(*args, **kwargs):
            if args == (['juju', 'backup'],):
                return 'juju-backup-24.tgz'
            if args == (('juju', '--show-log', 'status', '-e', 'baz'),):
                return yaml.safe_dump({
                    'machines': {'0': {
                        'instance-id': 'asdf',
                        'dns-name': '128.100.100.128',
                        }}
                    })
            self.assertEqual([], args)
        iterator = iter_steps_validate_info(self, br_attempt, client)
        self.assertEqual(iterator.next(), {'test_id': 'back-up-restore'})
        with patch('subprocess.check_output',
                   side_effect=check_output) as co_mock:
            with patch('subprocess.check_call') as cc_mock:
                with patch('sys.stdout'):
                    self.assertEqual(
                        iterator.next(),
                        {'test_id': 'back-up-restore'})
        assert_juju_call(self, co_mock, client, ['juju', 'backup'], 0)
        self.assertEqual(
            cc_mock.mock_calls[0],
            call(['euca-terminate-instances', 'asdf'], env=environ))
        self.assertEqual(iterator.next(), {'test_id': 'back-up-restore'})
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout'):
                self.assertEqual(iterator.next(),
                                 {'test_id': 'back-up-restore'})
        assert_juju_call(
            self, cc_mock, client, (
                'juju', '--show-log', 'restore', '-e', 'baz',
                os.path.abspath('juju-backup-24.tgz')))
        output = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
                },
            'services': {},
            })
        with patch('subprocess.check_output', return_value=output) as co_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'back-up-restore', 'result': True})
        assert_juju_call(self, co_mock, client, (
            'juju', '--show-log', 'status', '-e', 'baz'), assign_stderr=True)


class TestMaybeWriteJson(TestCase):

    def test_maybe_write_json(self):
        with NamedTemporaryFile() as temp_file:
            maybe_write_json(temp_file.name, {})
            self.assertEqual('{}', temp_file.read())

    def test_maybe_write_json_none(self):
        maybe_write_json(None, {})

    def test_maybe_write_json_pretty(self):
        with NamedTemporaryFile() as temp_file:
            maybe_write_json(temp_file.name, {'a': ['b'], 'b': 'c'})
            expected = dedent("""\
                {
                  "a": [
                    "b"
                  ],\x20
                  "b": "c"
                }""")
            self.assertEqual(temp_file.read(), expected)


class TestMakeSubstrate(TestCase):

    def test_make_substrate_no_support(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {'type': 'foo'}),
                               '', '')
        self.assertIs(make_substrate(client, []), None)

    def test_make_substrate_no_requirements(self):
        client = EnvJujuClient(get_aws_env(), '', '')
        self.assertIs(type(make_substrate(client, [])), AWSAccount)

    def test_make_substrate_unsatisifed_requirements(self):
        client = EnvJujuClient(get_aws_env(), '', '')
        self.assertIs(make_substrate(client, ['foo']), None)
        self.assertIs(make_substrate(client, ['iter_security_groups', 'foo']),
                      None)

    def test_make_substrate_satisfied_requirements(self):
        client = EnvJujuClient(get_aws_env(), '', '')
        self.assertIs(type(make_substrate(client, ['iter_security_groups'])),
                      AWSAccount)
        self.assertIs(type(make_substrate(client, [
            'iter_security_groups', 'iter_instance_security_groups'])),
            AWSAccount)
