# coding=utf-8
from argparse import Namespace
from collections import OrderedDict
from contextlib import (
    closing,
    contextmanager,
    )
import os
from tempfile import (
    mkdtemp,
    NamedTemporaryFile,
    )
from textwrap import dedent

from boto.ec2.securitygroup import SecurityGroup
from mock import (
    call,
    MagicMock,
    patch,
    )
import yaml

from industrial_test import (
    AttemptSuite,
    AttemptSuiteFactory,
    BACKUP,
    BackupRestoreAttempt,
    BootstrapAttempt,
    CannotUpgradeToClient,
    CannotUpgradeToOldClient,
    DENSITY,
    DeployManyAttempt,
    DestroyEnvironmentAttempt,
    EnsureAvailabilityAttempt,
    FULL,
    IndustrialTest,
    make_substrate_manager,
    maybe_write_json,
    MultiIndustrialTest,
    parse_args,
    PrepareUpgradeJujuAttempt,
    QUICK,
    StageInfo,
    SteppedStageAttempt,
    UpgradeCharmAttempt,
    UpgradeJujuAttempt,
    )
from jujuconfig import get_euca_env
from jujupy import (
    EnvJujuClient,
    EnvJujuClient1X,
    get_timeout_prefix,
    JujuData,
    KVM_MACHINE,
    LXC_MACHINE,
    LXD_MACHINE,
    SimpleEnvironment,
    Status,
    _temp_env,
    )
from substrate import AWSAccount
from tests import (
    FakeHomeTestCase,
    parse_error,
    TestCase,
    use_context,
)
from tests.test_deploy_stack import FakeBootstrapManager
from test_jujupy import (
    assert_juju_call,
    fake_juju_client,
    FakePopen,
    observable_temp_file,
    )
from test_substrate import (
    get_aws_env,
    get_os_config,
    make_os_security_group_instance,
    make_os_security_groups,
    )
from utility import (
    LoggedException,
    temp_dir,
    )


__metaclass__ = type


def get_aws_juju_data():
    aws_env = get_aws_env()
    return JujuData(aws_env.environment, aws_env.config)


class JujuPyTestCase(FakeHomeTestCase):

    def setUp(self):
        super(JujuPyTestCase, self).setUp()
        patcher = patch('jujupy.pause')
        self.addCleanup(patcher.stop)
        patcher.start()

        patcher = patch('jujupy.GroupReporter._write')
        self.addCleanup(patcher.stop)
        patcher.start()

        patcher = patch('jujupy.until_timeout', return_value=[2, 1])
        self.addCleanup(patcher.stop)
        patcher.start()


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


def patch_status(client, *statuses):
    """
    Replace calls to EnvJujuClient.get_status with preformed output.

    If client is None, all clients will have the given status for the duration
    of the patch, otherwise only the given client is modified.

    If more than one status argument is passed, they will be returned in
    sequence, otherwise every call will return the single given status.

    Triva, the plural of status is Latin is statūs.
    """
    kwargs = {}
    if len(statuses) > 1:
        kwargs['side_effect'] = (Status.from_text(yaml.safe_dump(s))
                                 for s in statuses).next
    else:
        kwargs['return_value'] = Status.from_text(yaml.safe_dump(statuses[0]))
    if client is not None:
        return patch.object(client, 'get_status', autospec=True, **kwargs)
    return patch('jujupy.EnvJujuClient.get_status', autospec=True, **kwargs)


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
        with parse_error(self) as stderr:
            args = parse_args(['rai', 'new-juju', QUICK])
        self.assertRegexpMatches(
            stderr.getvalue(), '.*error: too few arguments.*')
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertEqual(args.env, 'rai')
        self.assertEqual(args.new_juju_path, 'new-juju')
        self.assertEqual(args.log_dir, 'log-dir')
        self.assertEqual(args.suite, [QUICK])
        self.assertIs(args.agent_stream, None)

    def test_parse_args_attempts(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertEqual(args.attempts, 2)
        args = parse_args(['rai', 'new-juju', '--attempts', '3', QUICK,
                           'log-dir'])
        self.assertEqual(args.attempts, 3)

    def test_parse_args_json_file(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertIs(args.json_file, None)
        args = parse_args(['rai', 'new-juju', '--json-file', 'foobar', QUICK,
                           'log-dir'])
        self.assertEqual(args.json_file, 'foobar')

    def test_parse_args_suite(self):
        args = parse_args(['rai', 'new-juju', 'full', 'log-dir'])
        self.assertEqual(args.suite, [FULL])
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertEqual(args.suite, [QUICK])
        args = parse_args(['rai', 'new-juju', DENSITY, 'log-dir'])
        self.assertEqual(args.suite, [DENSITY])
        args = parse_args(['rai', 'new-juju', BACKUP, 'log-dir'])
        self.assertEqual(args.suite, [BACKUP])
        with parse_error(self) as stderr:
            args = parse_args(['rai', 'new-juju', 'foo', 'log-dir'])
        self.assertRegexpMatches(
            stderr.getvalue(), ".*argument suite: invalid choice: 'foo'.*")

    def test_parse_args_multi_suite(self):
        args = parse_args(['rai', 'new-juju', 'full,quick', 'log-dir'])
        self.assertEqual(args.suite, [FULL, QUICK])
        with parse_error(self) as stderr:
            args = parse_args(['rai', 'new-juju', 'full,foo', 'log-dir'])
        self.assertRegexpMatches(
            stderr.getvalue(), ".*argument suite: invalid choice: 'foo'.*")

    def test_parse_args_agent_url(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertEqual(args.new_agent_url, None)
        args = parse_args(['rai', 'new-juju', '--new-agent-url',
                           'http://example.org', QUICK, 'log-dir'])
        self.assertEqual(args.new_agent_url, 'http://example.org')

    def test_parse_args_debug(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertEqual(args.debug, False)
        args = parse_args(['rai', 'new-juju', '--debug', QUICK, 'log-dir'])
        self.assertEqual(args.debug, True)

    def test_parse_args_old_stable(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir',
                           '--old-stable', 'asdf'])
        self.assertEqual(args.old_stable, 'asdf')
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertIs(args.old_stable, None)

    def test_parse_args_agent_stream(self):
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir',
                           '--agent-stream', 'asdf'])
        self.assertEqual(args.agent_stream, 'asdf')
        args = parse_args(['rai', 'new-juju', QUICK, 'log-dir'])
        self.assertIs(args.old_stable, None)


class FakeStepAttempt:

    def __init__(self, result, new_path=None):
        self.result = result
        self.stage = StageInfo(result[0][0], '{} title'.format(result[0][0]))
        self.new_path = new_path

    @classmethod
    def from_result(cls, old, new, test_id='foo-id', new_path=None):
        """Alternate constructor for backwards-compatibility.

        Allows tests that used FakeAttempt to be adapted with minimal changes.
        """
        return cls([(test_id, old, new)], new_path)

    def __eq__(self, other):
        return (
            type(self) == type(other) and self.result == other.result)

    def get_test_info(self):
        return {self.result[0][0]: {'title': self.result[0][0]}}

    def get_bootstrap_client(self, client):
        return client

    def iter_test_results(self, old, new):
        return iter(self.result)

    def iter_steps(self, client):
        yield self.stage.as_result()
        if self.new_path is not None and client.full_path == self.new_path:
            result_value = self.result[0][2]
        else:
            result_value = self.result[0][1]
        if isinstance(result_value, BaseException):
            raise result_value
        yield self.stage.as_result(result_value)


class FakeAttemptClass:
    """Instances of this class behave like classes, not instances.

    Methods like factory, that would be classmethods on a normal class, are
    normal methods on FakeAttemptClass.
    """

    def factory(self, upgrade_sequence, attempt_stream):
        return self()

    def __init__(self, title, *result, **kwargs):
        self.title = title
        self.test_id = '{}-id'.format(title)
        self.result = result
        self.new_path = kwargs.get('new_path')

    def get_test_info(self):
        return {self.test_id: {'title': self.title}}

    def __call__(self):
        return FakeStepAttempt.from_result(*self.result, test_id=self.test_id,
                                           new_path=self.new_path)


@contextmanager
def temp_env(name, config=None):
    if config is None:
        config = {}
    environments = {'environments': {name: config}}
    with _temp_env(environments):
        yield


def fake_bootstrap_manager(self, temp_env_name, client, *args, **kwargs):
    return FakeBootstrapManager(client)


class TestMultiIndustrialTest(TestCase):

    def test_from_args(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', attempts=7, suite=[DENSITY],
            log_dir='log-dir', new_agent_url=None, debug=False,
            old_stable=None, agent_stream=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args, QUICK)
        self.assertEqual(mit.env, 'foo')
        self.assertEqual(mit.new_juju_path, 'new-path')
        self.assertEqual(mit.attempt_count, 7)
        self.assertEqual(mit.max_attempts, 14)
        self.assertEqual(mit.log_parent_dir, 'log-dir')
        self.assertIs(mit.agent_stream, None)
        self.assertEqual(
            mit.stages, AttemptSuiteFactory([]))
        args = Namespace(
            env='bar', new_juju_path='new-path2', attempts=6, suite=[FULL],
            log_dir='log-dir2', new_agent_url=None, debug=False,
            old_stable=None, agent_stream=None)
        with temp_env('bar'):
            mit = MultiIndustrialTest.from_args(args, FULL)
        self.assertEqual(mit.env, 'bar')
        self.assertEqual(mit.new_juju_path, 'new-path2')
        self.assertEqual(mit.attempt_count, 6)
        self.assertEqual(mit.max_attempts, 12)
        self.assertEqual(mit.log_parent_dir, 'log-dir2')
        self.assertIs(mit.agent_stream, None)
        self.assertEqual(
            mit.stages, AttemptSuiteFactory([
                UpgradeCharmAttempt, DeployManyAttempt,
                BackupRestoreAttempt, EnsureAvailabilityAttempt]))

    def test_from_args_maas(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', log_dir='log-dir',
            attempts=7, new_agent_url=None, debug=False, old_stable=None,
            agent_stream=None)
        with temp_env('foo', {'type': 'maas'}):
            mit = MultiIndustrialTest.from_args(args, DENSITY)
        self.assertEqual(
            mit.stages, AttemptSuiteFactory([DeployManyAttempt]))

    def test_from_args_debug(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', log_dir='log-dir',
            attempts=7, new_agent_url=None, debug=False, old_stable=None,
            agent_stream=None)
        with temp_env('foo', {'type': 'maas'}):
            mit = MultiIndustrialTest.from_args(args, DENSITY)
            self.assertEqual(mit.debug, False)
            args.debug = True
            mit = MultiIndustrialTest.from_args(args, DENSITY)
            self.assertEqual(mit.debug, True)

    def test_from_args_really_old_path(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', log_dir='log-dir',
            attempts=7, new_agent_url=None, debug=False,
            old_stable='really-old-path', agent_stream=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args, FULL)
        self.assertEqual(mit.really_old_path, 'really-old-path')
        args = Namespace(
            env='bar', new_juju_path='new-path2', log_dir='log-dir',
            attempts=6, new_agent_url=None, debug=False, old_stable=None,
            agent_stream=None)
        with temp_env('bar'):
            mit = MultiIndustrialTest.from_args(args, FULL)
        self.assertIs(mit.really_old_path, None)

    def test_from_args_agent_stream(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', log_dir='log-dir',
            attempts=7, new_agent_url=None, debug=False, old_stable=None,
            agent_stream='foo-stream')
        with temp_env('foo', {'type': 'maas'}):
            mit = MultiIndustrialTest.from_args(args, DENSITY)
            self.assertEqual(mit.debug, False)
            args.debug = True
            mit = MultiIndustrialTest.from_args(args, DENSITY)
            self.assertEqual(mit.agent_stream, 'foo-stream')

    def test_density_suite(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', attempts=7,
            log_dir='log-dir', new_agent_url=None, debug=False,
            old_stable=None, agent_stream=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args, DENSITY)
        self.assertEqual(
            mit.stages, AttemptSuiteFactory([DeployManyAttempt]))

    def test_backup_suite(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', attempts=7,
            log_dir='log-dir', new_agent_url=None, debug=False,
            old_stable=None, agent_stream=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args, BACKUP)
        self.assertEqual(
            mit.stages, AttemptSuiteFactory([BackupRestoreAttempt]))

    def test_from_args_new_agent_url(self):
        args = Namespace(
            env='foo', new_juju_path='new-path', attempts=7,
            log_dir='log-dir', new_agent_url='http://example.net',
            debug=False, old_stable=None, agent_stream=None)
        with temp_env('foo'):
            mit = MultiIndustrialTest.from_args(args, suite=QUICK)
        self.assertEqual(mit.new_agent_url, 'http://example.net')

    def test_init(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', [
            DestroyEnvironmentAttempt, BootstrapAttempt], 'log-dir', 5)
        self.assertEqual(mit.env, 'foo-env')
        self.assertEqual(mit.new_juju_path, 'bar-path')
        self.assertEqual(mit.stages, [DestroyEnvironmentAttempt,
                                      BootstrapAttempt])
        self.assertEqual(mit.attempt_count, 5)
        self.assertEqual(mit.log_parent_dir, 'log-dir')

    def test_make_results(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', AttemptSuiteFactory([
            DestroyEnvironmentAttempt]), 5)
        results = mit.make_results()
        self.assertEqual(results, {'results': [
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'bootstrap', 'test_id': 'bootstrap', 'report_on': True},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'report_on': False},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'destroy environment', 'test_id': 'destroy-env',
             'report_on': True},
            {'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'report_on': True},
        ]})

    def test_make_results_report_on(self):
        class NoReportOn:

            @staticmethod
            def get_test_info():
                return {'no-report': {
                    'title': 'No report', 'report_on': False}}

        mit = MultiIndustrialTest('foo-env', 'bar-path', AttemptSuiteFactory([
            NoReportOn]), 5)
        results = mit.make_results()
        self.assertEqual(results, {'results': [
            {
                'test_id': 'bootstrap',
                'title': 'bootstrap',
                'report_on': True,
                'attempts': 0,
                'old_failures': 0,
                'new_failures': 0,
            },
            {
                'test_id': 'prepare-suite',
                'title': 'Prepare suite tests',
                'report_on': False,
                'attempts': 0,
                'old_failures': 0,
                'new_failures': 0,
            },
            {
                'test_id': 'no-report',
                'title': 'No report',
                'report_on': False,
                'attempts': 0,
                'old_failures': 0,
                'new_failures': 0,
            },
            {
                'test_id': 'destroy-env',
                'title': 'destroy environment',
                'report_on': True,
                'attempts': 0,
                'old_failures': 0,
                'new_failures': 0,
            },
            {
                'test_id': 'substrate-clean',
                'title': 'check substrate clean',
                'report_on': True,
                'attempts': 0,
                'old_failures': 0,
                'new_failures': 0,
            },
        ]})

    @staticmethod
    @contextmanager
    def patch_client(by_version):
        with patch('jujupy.EnvJujuClient.by_version', side_effect=by_version):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                with patch.object(EnvJujuClient, 'get_full_path',
                                  side_effect=lambda: 'juju'):
                    yield

    def test_make_industrial_test(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path', AttemptSuiteFactory([
            DestroyEnvironmentAttempt]), 'log-dir', 5)
        with self.patch_client(lambda x, y=None, debug=False: (x, y)):
            industrial = mit.make_industrial_test()
        self.assertEqual(industrial.old_client, (
            SimpleEnvironment('foo-env-old', {'name': 'foo-env-old'}), None))
        self.assertEqual(industrial.new_client, (
            SimpleEnvironment('foo-env-new', {'name': 'foo-env-new'}),
            'bar-path'))
        self.assertEqual(len(industrial.stage_attempts), 1)
        self.assertEqual([mit.stages], [sa.attempt_list for sa in
                         industrial.stage_attempts])

    def test_make_industrial_test_new_agent_url(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path',
                                  AttemptSuiteFactory([]), 'log-dir',
                                  new_agent_url='http://example.com')
        with self.patch_client(lambda x, y=None, debug=False: (x, y)):
            industrial = mit.make_industrial_test()
        self.assertEqual(
            industrial.new_client, (
                SimpleEnvironment('foo-env-new', {
                    'name': 'foo-env-new',
                    'tools-metadata-url': 'http://example.com',
                    }),
                'bar-path')
            )

    def test_make_industrial_test_debug(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path',
                                  AttemptSuiteFactory([]), 'log-dir',
                                  new_agent_url='http://example.com')

        def side_effect(x, y=None, debug=False):
            return debug

        with self.patch_client(side_effect):
            industrial = mit.make_industrial_test()
        self.assertEqual(industrial.new_client, False)
        self.assertEqual(industrial.old_client, False)
        mit.debug = True
        with self.patch_client(side_effect):
            industrial = mit.make_industrial_test()
        self.assertEqual(industrial.new_client, True)
        self.assertEqual(industrial.old_client, True)

    def test_update_results(self):
        mit = MultiIndustrialTest('foo-env', 'bar-path',
                                  AttemptSuiteFactory([]), 2)
        results = mit.make_results()
        mit.update_results([('bootstrap', True, False)], results)
        expected = {'results': [
            {'title': 'bootstrap', 'test_id': 'bootstrap',
             'attempts': 1, 'new_failures': 1, 'old_failures': 0,
             'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 0,
             'new_failures': 0, 'old_failures': 0, 'report_on': False},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 0,
             'new_failures': 0, 'old_failures': 0, 'report_on': True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 0, 'new_failures': 0, 'old_failures': 0,
             'report_on': True},
            ]}
        self.assertEqual(results, expected)
        mit.update_results([
            ('bootstrap', True, True), ('prepare-suite', True, True),
            ('destroy-env', False, True), ('substrate-clean', True, True)
            ], results)
        self.assertEqual(results, {'results': [
            {'title': 'bootstrap', 'test_id': 'bootstrap',
             'attempts': 2, 'new_failures': 1, 'old_failures': 0,
             'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 1, 'new_failures': 0, 'old_failures': 0,
             'report_on': False},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 1, 'new_failures': 0, 'old_failures': 1, 'report_on':
             True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 1, 'new_failures': 0, 'old_failures': 0,
             'report_on': True},
            ]})
        mit.update_results(
            [('bootstrap', False, False), ('prepare-suite', True, True),
             ('destroy-env', False, False), ('substrate-clean', True, True)],
            results)
        expected = {'results': [
            {'title': 'bootstrap', 'test_id': 'bootstrap',
             'attempts': 2, 'new_failures': 1, 'old_failures': 0,
             'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 2, 'new_failures': 0, 'old_failures': 0,
             'report_on': False},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 2, 'new_failures': 1, 'old_failures': 2,
             'report_on': True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 2, 'new_failures': 0, 'old_failures': 0,
             'report_on': True},
            ]}
        self.assertEqual(results, expected)

    def test_run_tests(self):
        log_dir = use_context(self, temp_dir())
        mit = MultiIndustrialTest('foo-env', 'bar-path', AttemptSuiteFactory([
            FakeAttemptClass('foo', True, True, new_path='bar-path'),
            FakeAttemptClass('bar', True, False, new_path='bar-path'),
            ]), log_dir, 5, 10)

        def side_effect(env, full_path=None, debug=False):
            return fake_juju_client(None, full_path, debug)

        with self.patch_client(side_effect):
            with patch('industrial_test.BootstrapManager',
                       side_effect=fake_bootstrap_manager):
                results = mit.run_tests()
        self.assertEqual(results, {'results': [
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 5,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 5, 'old_failures': 0, 'new_failures': 0,
             'report_on': False},
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 5, 'report_on': True},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            ]})

    def test_run_tests_max_attempts(self):
        log_dir = use_context(self, temp_dir())
        mit = MultiIndustrialTest('foo-env', 'bar-path', AttemptSuiteFactory([
            FakeAttemptClass('foo', True, False, new_path='bar-path'),
            FakeAttemptClass('bar', True, False, new_path='bar-path'),
            ]), log_dir, 5, 6)

        def side_effect(env, full_path=None, debug=False):
            return fake_juju_client(None, full_path, debug)

        with self.patch_client(side_effect):
            with patch('industrial_test.BootstrapManager',
                       side_effect=fake_bootstrap_manager):
                results = mit.run_tests()
        self.assertEqual(results, {'results': [
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 5,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 5, 'old_failures': 0, 'new_failures': 0,
             'report_on': False},
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 5,
             'old_failures': 0, 'new_failures': 5, 'report_on': True},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 0,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            ]})

    def test_run_tests_max_attempts_less_than_attempt_count(self):
        log_dir = use_context(self, temp_dir())
        mit = MultiIndustrialTest(
            'foo-env', 'bar-path', AttemptSuiteFactory([
                FakeAttemptClass('foo', True, False, new_path='bar-path'),
                FakeAttemptClass('bar', True, False, new_path='bar-path')],
                ),
            log_dir, 5, 4)

        def side_effect(env, full_path=None, debug=False):
            return fake_juju_client(None, full_path, debug)

        with self.patch_client(side_effect):
            with patch('industrial_test.BootstrapManager',
                       side_effect=fake_bootstrap_manager):
                results = mit.run_tests()
        expected = [
            {'title': 'bootstrap', 'test_id': 'bootstrap', 'attempts': 4,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'Prepare suite tests', 'test_id': 'prepare-suite',
             'attempts': 4, 'old_failures': 0, 'new_failures': 0,
             'report_on': False},
            {'title': 'foo', 'test_id': 'foo-id', 'attempts': 4,
             'old_failures': 0, 'new_failures': 4, 'report_on': True},
            {'title': 'bar', 'test_id': 'bar-id', 'attempts': 0,
             'old_failures': 0, 'new_failures': 0, 'report_on': True},
            {'title': 'destroy environment', 'test_id': 'destroy-env',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            {'title': 'check substrate clean', 'test_id': 'substrate-clean',
             'attempts': 0, 'old_failures': 0, 'new_failures': 0,
             'report_on': True},
            ]
        self.assertEqual(results, {'results': expected})

    @staticmethod
    def get_results_1():
        return {
            'results': [
                {'title': 'foo', 'attempts': 5, 'old_failures': 1,
                 'new_failures': 2, 'test_id': 'foo-id'},
                {'title': 'bar', 'attempts': 5, 'old_failures': 3,
                 'new_failures': 4, 'report_on': True, 'test_id': 'bar-id'},
                {'title': 'baz', 'attempts': 5, 'old_failures': 3,
                 'new_failures': 4, 'report_on': False, 'test_id': 'baz-id'},
                ]}

    def test_combine_results_noop(self):
        new_results = MultiIndustrialTest.combine_results([
            self.get_results_1()])
        self.assertEqual(new_results, self.get_results_1())

    def test_combine_results_append(self):
        results_2 = {'results': [
            {'title': 'qux', 'test_id': 'quxx-id', 'attempts': 2,
             'old_failures': 2, 'new_failures': 1}]}
        new_results = MultiIndustrialTest.combine_results(
            [self.get_results_1(), results_2])
        self.assertEqual(new_results['results'][:3],
                         self.get_results_1()['results'])
        self.assertEqual(new_results['results'][3:], results_2['results'])

    def test_combine_results_add(self):
        results_2 = {'results': [
            {'test_id': 'foo-id', 'title': 'foo6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1}]}
        new_results = MultiIndustrialTest.combine_results(
            [self.get_results_1(), results_2])
        self.assertEqual(new_results, {'results': [
            {'title': 'foo', 'attempts': 8, 'old_failures': 3,
             'new_failures': 3, 'test_id': 'foo-id', 'report_on': True},
            {'title': 'bar', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4, 'report_on': True, 'test_id': 'bar-id'},
            {'title': 'baz', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4, 'report_on': False, 'test_id': 'baz-id'},
            ]})

    def test_combine_results_report_on(self):
        results_1 = {'results': [
            {'test_id': 'foo-id', 'title': 'foo6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1},
            {'test_id': 'bar-id', 'title': 'bar6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1},
            {'test_id': 'baz-id', 'title': 'baz', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1, 'report_on': False},
            {'test_id': 'qux-id', 'title': 'qux', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1, 'report_on': False},
            ]}
        results_2 = {'results': [
            {'test_id': 'foo-id', 'title': 'foo6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1, 'report_on': True},
            {'test_id': 'bar-id', 'title': 'foo6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1, 'report_on': False},
            {'test_id': 'baz-id', 'title': 'foo6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1},
            {'test_id': 'qux-id', 'title': 'qux6', 'attempts': 3,
             'old_failures': 2, 'new_failures': 1, 'report_on': False},
            ]}
        new_results = MultiIndustrialTest.combine_results(
            [results_1, results_2])
        self.assertEqual(new_results['results'][0].get('report_on', True),
                         True)
        self.assertEqual(new_results['results'][1].get('report_on', True),
                         True)
        self.assertEqual(new_results['results'][2].get('report_on', True),
                         True)
        self.assertEqual(new_results['results'][3].get('report_on', False),
                         False)

    def test_results_table(self):
        results = [
            {'title': 'foo', 'attempts': 5, 'old_failures': 1,
             'new_failures': 2},
            {'title': 'bar', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4, 'report_on': True},
            {'title': 'baz', 'attempts': 5, 'old_failures': 3,
             'new_failures': 4, 'report_on': False},
            ]
        self.assertEqual(
            ''.join(MultiIndustrialTest.results_table(results)),
            dedent("""\
                old failure | new failure | attempt | title
                          1 |           2 |       5 | foo
                          3 |           4 |       5 | bar
            """))


class TestIndustrialTest(JujuPyTestCase):

    def test_init(self):
        old_client = object()
        new_client = object()
        attempt_list = []
        industrial = IndustrialTest(old_client, new_client, attempt_list)
        self.assertIs(old_client, industrial.old_client)
        self.assertIs(new_client, industrial.new_client)
        self.assertIs(attempt_list, industrial.stage_attempts)

    def test_from_args(self):
        def side_effect(x, y=None, debug=False):
            return (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                industrial = IndustrialTest.from_args(
                    'foo', 'new-juju-path', [])
        self.assertIsInstance(industrial, IndustrialTest)
        self.assertEqual(industrial.old_client, (
            SimpleEnvironment('foo-old', {'name': 'foo-old'}), None))
        self.assertEqual(industrial.new_client, (
            SimpleEnvironment('foo-new', {'name': 'foo-new'}),
            'new-juju-path'))
        self.assertNotEqual(industrial.old_client[0].environment,
                            industrial.new_client[0].environment)

    def test_from_args_debug(self):
        def side_effect(x, y=None, debug=False):
            return debug
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config'):
                industrial = IndustrialTest.from_args(
                    'foo', 'new-juju-path', [], debug=False)
                self.assertEqual(industrial.old_client, False)
                self.assertEqual(industrial.new_client, False)
                industrial = IndustrialTest.from_args(
                    'foo', 'new-juju-path', [], debug=True)
                self.assertEqual(industrial.old_client, True)
                self.assertEqual(industrial.new_client, True)

    def test_run_stages(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        industrial = IndustrialTest(old_client, new_client, [
            FakeStepAttempt.from_result(True, True),
            FakeStepAttempt.from_result(True, True)])
        with patch('subprocess.call') as cc_mock:
            result = industrial.run_stages()
            self.assertItemsEqual(result, [('foo-id', True, True),
                                           ('foo-id', True, True)])
        self.assertEqual(len(cc_mock.mock_calls), 0)

    def test_run_stages_old_fail(self):
        old_client = fake_juju_client()
        new_client = fake_juju_client(full_path='bar-path')
        industrial = IndustrialTest(old_client, new_client, [
            FakeStepAttempt.from_result(False, True),
            FakeStepAttempt.from_result(True, True)])
        suite_factory = AttemptSuiteFactory([
            FakeAttemptClass('foo', False, True, new_path='bar-path'),
            FakeAttemptClass('bar', True, True, new_path='bar-path')])
        log_dir = use_context(self, temp_dir())
        suite = suite_factory.factory([], log_dir, None)
        industrial = IndustrialTest(old_client, new_client, [suite])
        with patch('industrial_test.BootstrapManager',
                   fake_bootstrap_manager):
            result = industrial.run_stages()
            self.assertItemsEqual(result, [
                ('bootstrap', True, True),
                ('prepare-suite', True, True),
                ('foo-id', False, True)])
        self.assertEqual('controller-killed',
                         old_client._backend.controller_state.state)
        self.assertEqual('controller-killed',
                         new_client._backend.controller_state.state)

    def test_run_stages_new_fail(self):
        old_client = fake_juju_client()
        new_client = fake_juju_client(full_path='bar-path')
        log_dir = use_context(self, temp_dir())
        suite_factory = AttemptSuiteFactory([
            FakeAttemptClass('foo', True, False, new_path='bar-path'),
            FakeAttemptClass('bar', True, True, new_path='bar-path')])
        suite = suite_factory.factory([], log_dir, None)
        industrial = IndustrialTest(old_client, new_client, [suite])
        with patch('industrial_test.BootstrapManager',
                   fake_bootstrap_manager):
            result = industrial.run_stages()
            self.assertItemsEqual(result, [
                ('bootstrap', True, True),
                ('prepare-suite', True, True),
                ('foo-id', True, False)])
        self.assertEqual('controller-killed',
                         old_client._backend.controller_state.state)
        self.assertEqual('controller-killed',
                         new_client._backend.controller_state.state)

    def test_run_stages_both_fail(self):
        old_client = fake_juju_client()
        new_client = fake_juju_client()
        log_dir = use_context(self, temp_dir())
        suite = AttemptSuiteFactory([
            FakeAttemptClass('foo', False, False),
            FakeAttemptClass('bar', True, True)]).factory([], log_dir,
                                                          'foo-stream')
        industrial = IndustrialTest(old_client, new_client, [suite])
        with patch('industrial_test.BootstrapManager',
                   fake_bootstrap_manager):
            result = industrial.run_stages()
            self.assertItemsEqual(result, [
                ('bootstrap', True, True),
                ('prepare-suite', True, True),
                ('foo-id', False, False)])
        self.assertEqual('controller-killed',
                         old_client._backend.controller_state.state)
        self.assertEqual('controller-killed',
                         new_client._backend.controller_state.state)

    def test_run_stages_recover_failure(self):
        old_client = fake_juju_client()
        new_client = fake_juju_client()
        fsa = FakeStepAttempt([('foo', True, False), ('bar', True, True)])
        industrial = IndustrialTest(old_client, new_client, [
            fsa, FakeStepAttempt.from_result(True, True)])
        self.assertEqual(list(industrial.run_stages()), [
            ('foo', True, False), ('bar', True, True), ('foo-id', True, True)])

    def test_run_stages_failure_in_last_step(self):
        old_client = FakeEnvJujuClient('old')
        new_client = FakeEnvJujuClient('new')
        fsa = FakeStepAttempt([('foo', True, True), ('bar', False, True)])
        industrial = IndustrialTest(old_client, new_client, [
            fsa, FakeStepAttempt.from_result(True, True)])
        with patch.object(old_client, 'kill_controller'):
            with patch.object(new_client, 'kill_controller'):
                self.assertEqual(list(industrial.run_stages()), [
                    ('foo', True, True), ('bar', False, True)])

    def test_run_stages_raises_cannot_upgrade_to_old_client(self):
        old = FakeEnvJujuClient()
        new = FakeEnvJujuClient()
        industrial = IndustrialTest(old, new, [PrepareUpgradeJujuAttempt({})])
        with self.assertRaises(CannotUpgradeToOldClient):
            list(industrial.run_stages())

    def test_run_attempt(self):
        old_client = fake_juju_client()
        new_client = fake_juju_client()
        attempt = FakeStepAttempt.from_result(True, True)
        log_dir = use_context(self, temp_dir())
        suite = AttemptSuiteFactory([attempt]).factory([], log_dir, None)
        industrial = IndustrialTest(old_client, new_client,
                                    [suite])

        def iter_test_results(old, new):
            raise Exception
            yield

        with patch.object(attempt, 'iter_test_results',
                          iter_test_results):
            with patch('logging.exception') as le_mock:
                with patch('industrial_test.BootstrapManager',
                           fake_bootstrap_manager):
                    industrial.run_attempt()
        self.assertEqual(2, le_mock.call_count)
        self.assertEqual('controller-killed',
                         old_client._backend.controller_state.state)
        self.assertEqual('controller-killed',
                         new_client._backend.controller_state.state)


class TestSteppedStageAttempt(JujuPyTestCase):

    def test__iter_for_result_premature_results(self):
        iterator = (x for x in [{'test_id': 'foo-id', 'result': True}])
        with self.assertRaisesRegexp(ValueError, 'Result before declaration.'):
            list(SteppedStageAttempt._iter_for_result(iterator))

    def test__iter_for_result_many(self):
        iterator = (x for x in [
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
        iterator = (x for x in [
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
        iterator = (x for x in [
            {'test_id': 'foo-id'}, {'test_id': 'bar-id', 'result': True}])
        with self.assertRaisesRegexp(ValueError, 'ID changed without result.'):
            list(SteppedStageAttempt._iter_for_result(iterator))

    def test__iter_test_results_success(self):
        old_iter = (x for x in [
            None, {'test_id': 'foo-id', 'result': True}])
        new_iter = (x for x in [
            None, {'test_id': 'foo-id', 'result': False}])

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {'foo-id': {'title': 'foo-id'}}

        self.assertItemsEqual(
            StubSA()._iter_test_results(old_iter, new_iter),
            [('foo-id', True, False)])

    def test__iter_test_results_interleaved(self):
        # Using a single iterator for both proves that they are interleaved.
        # Otherwise, we'd get Result before declaration.
        both_iter = (x for x in [
            None, None,
            {'test_id': 'foo-id', 'result': True},
            {'test_id': 'foo-id', 'result': False},
            ])

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_test_info():
                return {'foo-id': {'title': 'foo-id'}}

        self.assertItemsEqual(
            StubSA()._iter_test_results(both_iter, both_iter),
            [('foo-id', True, False)])

    def test__iter_test_results_id_mismatch(self):
        old_iter = (x for x in [
            None, {'test_id': 'foo-id', 'result': True}])
        new_iter = (x for x in [
            None, {'test_id': 'bar-id', 'result': False}])
        with self.assertRaises(LoggedException) as exc:
            list(SteppedStageAttempt()._iter_test_results(old_iter, new_iter))
        self.assertEqual(ValueError('Test id mismatch: foo-id bar-id').args,
                         exc.exception.exception.args)

    def test__iter_test_results_many(self):
        old_iter = (x for x in [
            None, {'test_id': 'foo-id', 'result': True},
            None, {'test_id': 'bar-id', 'result': False},
            ])
        new_iter = (x for x in [
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
            StubSA()._iter_test_results(old_iter, new_iter),
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

    def test_factory(self):

        class StubSA(SteppedStageAttempt):

            def __init__(self):
                super(StubSA, self).__init__()

        self.assertIs(type(StubSA.factory(['a', 'b', 'c'], None)), StubSA)

    def test_get_test_info(self):

        class StubSA(SteppedStageAttempt):

            @staticmethod
            def get_stage_info():
                return [StageInfo('foo-id', 'Foo title'),
                        StageInfo('bar-id', 'Bar title', report_on=False)]

        self.assertEqual(StubSA.get_test_info(), OrderedDict([
            ('foo-id', {'title': 'Foo title', 'report_on': True}),
            ('bar-id', {'title': 'Bar title', 'report_on': False})]))


def FakeEnvJujuClient(name='steve', version='1.2', full_path='/jbin/juju'):
    return EnvJujuClient(
            JujuData(name, {'type': 'fake', 'region': 'regionx'}),
            version, full_path)


class FakeEnvJujuClient1X(EnvJujuClient1X):

    def __init__(self, name='steve', version='1.2', full_path='/jbin/juju'):
        super(FakeEnvJujuClient1X, self).__init__(
            SimpleEnvironment(name, {'type': 'fake'}), version, full_path)


class TestBootstrapAttempt(JujuPyTestCase):

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        bootstrap = BootstrapAttempt()
        boot_iter = iter_steps_validate_info(self, bootstrap, client)
        self.assertEqual(boot_iter.next(), {'test_id': 'bootstrap'})
        with observable_temp_file() as config_file:
            with patch('subprocess.Popen') as popen_mock:
                self.assertEqual(boot_iter.next(), {'test_id': 'bootstrap'})
            assert_juju_call(self, popen_mock, client, (
                'juju', '--show-log', 'bootstrap', '--constraints', 'mem=2G',
                'steve', 'fake/regionx', '--config', config_file.name,
                '--default-model', 'steve', '--agent-version', '1.2'))
            statuses = [
                {'machines': {'0': {'agent-state': 'pending'}},
                 'applications': {}},
                {'machines': {'0': {'agent-state': 'started'}},
                 'applicaions': {}},
            ]
            popen_mock.return_value.wait.return_value = 0
            self.assertEqual(boot_iter.next(), {'test_id': 'bootstrap'})
        with patch_status(client, *statuses) as gs_mock:
            self.assertEqual(boot_iter.next(),
                             {'test_id': 'bootstrap', 'result': True})
        self.assertEqual(2, gs_mock.call_count)


class TestDestroyEnvironmentAttempt(JujuPyTestCase):

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
        with patch.object(client, 'get_jes_command',
                          return_value='kill-controller'):
            with patch.object(destroy_env, 'get_security_groups') as gsg_mock:
                with patch('subprocess.call', return_value=0) as mock_cc:
                    self.assertEqual(iterator.next(), {
                        'test_id': 'destroy-env', 'result': True})
        gsg_mock.assert_called_once_with(client)
        assert_juju_call(self, mock_cc, client, get_timeout_prefix(600) + (
            'juju', '--show-log', 'kill-controller', 'steve', '-y'))
        self.assertEqual(iterator.next(), {'test_id': 'substrate-clean'})
        with patch.object(destroy_env, 'check_security_groups') as csg_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'substrate-clean', 'result': True})
        csg_mock.assert_called_once_with(client, gsg_mock.return_value)

    def test_iter_steps_non_jes(self):
        client = FakeEnvJujuClient1X()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
        with patch.object(client, 'is_jes_enabled', return_value=False):
            with patch.object(destroy_env, 'get_security_groups') as gsg_mock:
                with patch('subprocess.call', return_value=0) as mock_cc:
                    self.assertEqual(iterator.next(), {
                        'test_id': 'destroy-env', 'result': True})
        gsg_mock.assert_called_once_with(client)
        assert_juju_call(self, mock_cc, client, get_timeout_prefix(600) + (
            'juju', '--show-log', 'destroy-environment', 'steve', '-y'))
        self.assertEqual(iterator.next(), {'test_id': 'substrate-clean'})
        with patch.object(destroy_env, 'check_security_groups') as csg_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'substrate-clean', 'result': True})
        csg_mock.assert_called_once_with(client, gsg_mock.return_value)

    def test_iter_test_results(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        with patch('subprocess.call'):
            with patch.object(client, 'get_jes_command',
                              return_value='kill-controller'):
                output = list(destroy_env.iter_test_results(client, client))
        self.assertEqual(output, [
            ('destroy-env', True, True), ('substrate-clean', True, True)])

    def test_iter_steps_failure(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
        with patch('subprocess.call', return_value=1) as mock_cc:
            with patch.object(client, 'get_jes_command',
                              return_value='kill-controller'):
                with patch.object(destroy_env,
                                  'get_security_groups') as gsg_mock:
                    with patch.object(client, 'kill_controller',
                                      side_effect=Exception) as kc_mock:
                        with self.assertRaises(Exception):
                            iterator.next()
        kc_mock.assert_called_once_with()
        gsg_mock.assert_called_once_with(client)
        self.assertEqual(0, mock_cc.call_count)
        with self.assertRaises(StopIteration):
            iterator.next()

    def test_iter_steps_failure_non_jes(self):
        client = FakeEnvJujuClient1X()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
        with patch('subprocess.call', return_value=1) as mock_cc:
            with patch.object(client, 'is_jes_enabled', return_value=False):
                with patch.object(destroy_env,
                                  'get_security_groups') as gsg_mock:
                    self.assertEqual(iterator.next(), {
                        'test_id': 'destroy-env', 'result': False})
        gsg_mock.assert_called_once_with(client)
        assert_juju_call(self, mock_cc, client, get_timeout_prefix(600) + (
            'juju', '--show-log', 'destroy-environment', 'steve', '-y'))
        with self.assertRaises(StopIteration):
            iterator.next()

    def test_iter_steps_kill_controller(self):
        client = fake_juju_client()
        client.bootstrap()
        destroy_env = DestroyEnvironmentAttempt()
        iterator = iter_steps_validate_info(self, destroy_env, client)
        with closing(iterator):
            self.assertEqual({'test_id': 'destroy-env'}, iterator.next())
            self.assertEqual(iterator.next(), {
                'test_id': 'destroy-env', 'result': True})
        self.assertEqual('controller-killed',
                         client._backend.controller_state.state)

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
        status = {'machines': {
            'foo': {'instance-id': 'foo-id'},
        }}
        aws_instances = [
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='foo', name='bar'),
                ])]),
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='baz', name='qux'),
                SecurityGroup(id='quxx-id', name='quxx'),
                ])]),
        ]
        aws_client = MagicMock()
        aws_client.get_all_instances.return_value = aws_instances
        with patch(
                'substrate.ec2.connect_to_region') as gec_mock:
            with patch_status(client, status):
                gai_mock = gec_mock.return_value.get_all_instances
                gai_mock.return_value = aws_instances
                self.assertEqual(destroy_env.get_security_groups(client), {
                    'baz': 'qux', 'foo': 'bar', 'quxx-id': 'quxx'
                    })
        self.assert_ec2_connection_call(gec_mock)
        gai_mock.assert_called_once_with(instance_ids=['foo-id'])

    def test_get_security_groups_openstack(self):
        client = self.get_openstack_client()
        destroy_env = DestroyEnvironmentAttempt()
        status = {'machines': {
            'foo': {'instance-id': 'bar-id'},
            'bar': {'instance-id': 'baz-qux-id'},
        }}
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
            with patch_status(client, status):
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
        aws_client = MagicMock()
        aws_client.get_all_security_groups.return_value = list(
            self.make_group())
        with patch('substrate.ec2.connect_to_region',
                   return_value=aws_client) as ctr_mock:
            with self.assertRaisesRegexp(
                Exception, (
                    r'Security group\(s\) not cleaned up: foo-group.')):
                    with patch('industrial_test.until_timeout',
                               lambda x: iter([None])):
                        destroy_env.check_security_groups(
                            client, {'foo-id': 'foo', 'bar-id': 'bar'})
        aws_client.get_all_security_groups.assert_called_once_with(
            filters={'description': 'juju group'})
        self.assert_ec2_connection_call(ctr_mock)

    def make_group(self):
        for name in ['foo', 'baz']:
            group = MagicMock()
            group.name = name + '-group'
            group.id = name + '-id'
            yield group

    def test_check_security_groups_no_match(self):
        client = self.get_aws_client()
        destroy_env = DestroyEnvironmentAttempt()
        aws_client = MagicMock()
        aws_client.get_all_security_groups.return_value = list(
            self.make_group())
        with patch('substrate.ec2.connect_to_region',
                   return_value=aws_client) as ctr_mock:
                destroy_env.check_security_groups(
                    client, {'bar-id': 'bar'})
        aws_client.get_all_security_groups.assert_called_once_with(
            filters={'description': 'juju group'})
        self.assert_ec2_connection_call(ctr_mock)

    def assert_ec2_connection_call(self, ctr_mock):
        ctr_mock.assert_called_once_with(
            'ca-west', aws_access_key_id='skeleton-key',
            aws_secret_access_key='secret-skeleton-key')

    def test_check_security_groups_non_aws(self):
        client = FakeEnvJujuClient()
        destroy_env = DestroyEnvironmentAttempt()
        with patch('subprocess.check_output') as co_mock:
                destroy_env.check_security_groups(
                    client, {'bar-id': 'bar'})
        self.assertEqual(co_mock.call_count, 0)


class TestEnsureAvailabilityAttempt(JujuPyTestCase):

    def test_iter_steps(self):
        client = FakeEnvJujuClient()
        admin_client = client.get_admin_client()
        ensure_av = EnsureAvailabilityAttempt()
        ensure_iter = iter_steps_validate_info(self, ensure_av, client)
        self.assertEqual(ensure_iter.next(), {
            'test_id': 'ensure-availability-n3'})
        with patch('subprocess.check_call') as cc_mock:
            with patch.object(client, 'get_admin_client',
                              return_value=admin_client, autospec=True):
                self.assertEqual(ensure_iter.next(), {
                    'test_id': 'ensure-availability-n3'})
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'enable-ha', '-m',
            admin_client.env.environment, '-n', '3'))
        status = {
            'machines': {
                '0': {'controller-member-status': 'has-vote'},
                '1': {'controller-member-status': 'has-vote'},
                '2': {'controller-member-status': 'has-vote'},
                },
            'applications': {},
        }
        with patch_status(admin_client, status) as gs_mock:
            self.assertEqual(ensure_iter.next(), {
                'test_id': 'ensure-availability-n3', 'result': True})
        gs_mock.assert_called_once_with(admin=True)

    def test_iter_steps_failure(self):
        client = FakeEnvJujuClient()
        ensure_av = EnsureAvailabilityAttempt()
        ensure_iter = iter_steps_validate_info(self, ensure_av, client)
        ensure_iter.next()
        with patch('subprocess.check_call'):
            admin_client = client.get_admin_client()
            with patch.object(client, 'get_admin_client',
                              return_value=admin_client, autospec=True):
                ensure_iter.next()
        status = {
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
                },
            'applications': {},
        }
        with patch_status(admin_client, status) as gs_mock:
            with self.assertRaisesRegexp(
                    Exception, 'Timed out waiting for voting to be enabled.'):
                ensure_iter.next()
        self.assertEqual(2, gs_mock.call_count)


class TestDeployManyAttempt(JujuPyTestCase):

    def predict_add_machine_calls(self, deploy_many, machine_type):
        for host in range(1, deploy_many.host_count + 1):
            for container in range(deploy_many.container_count):
                target = '{}:{}'.format(machine_type, host)
                service = 'ubuntu{}x{}'.format(host, container)
                yield ('juju', '--show-log', 'deploy', '-m', 'steve',
                       'ubuntu', service, '--to', target, '--series', 'angsty')

    def predict_remove_machine_calls(self, deploy_many):
        total_guests = deploy_many.host_count * deploy_many.container_count
        for guest in range(100, total_guests + 100):
            yield ('juju', '--show-log', 'remove-machine', '-m', 'steve',
                   '--force', str(guest))

    def test_iter_steps(self):
        machine_started = {'juju-status': {'current': 'idle'}}
        unit_started = {'agent-status': {'current': 'idle'}}
        client = FakeEnvJujuClient()
        client.env.config['default-series'] = 'angsty'
        self.do_iter_steps(client, LXD_MACHINE, machine_started, unit_started)

    def test_iter_steps_1x(self):
        started_state = {'agent-state': 'started'}
        client = FakeEnvJujuClient()
        with patch.object(EnvJujuClient, 'supported_container_types',
                          frozenset([KVM_MACHINE, LXC_MACHINE])):
            client.env.config['default-series'] = 'angsty'
            self.do_iter_steps(client, LXC_MACHINE, started_state,
                               started_state)

    def do_iter_steps(self, client, machine_type, machine_started,
                      unit_started):
        deploy_many = DeployManyAttempt(9, 11)
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = {
            'machines': {'0': dict(machine_started)},
            'applications': {},
        }
        with patch_status(client, status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-m', 'steve'), index)

        status = {
            'machines': dict((str(x), dict(machine_started))
                             for x in range(deploy_many.host_count + 1)),
            'applications': {},
        }
        with patch_status(client, status):
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'add-machine-many', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        with patch_status(client, status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'ensure-machines', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'deploy-many'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many'})

        calls = self.predict_add_machine_calls(deploy_many, machine_type)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)
        service_names = []
        for host in range(1, deploy_many.host_count + 1):
            for container in range(deploy_many.container_count):
                service_names.append('ubuntu{}x{}'.format(host, container))
        applications = {}
        for num, service_name in enumerate(service_names):
            foo = {'machine': str(num + 100)}
            foo.update(unit_started)
            units = {
                'foo': foo,
                }
            applications[service_name] = {'units': units}
        status = {
            'machines': {'0': dict(machine_started)},
            'applications': applications,
        }
        with patch_status(client, status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many', 'result': True})

        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'remove-machine-many-container'})
        with patch_status(client, status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'remove-machine-many-container'})
        calls = self.predict_remove_machine_calls(deploy_many)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)
        statuses = [
            {'machines': {'100': dict(machine_started)}, 'applications': {}},
            {'machines': {}, 'applications': {}},
        ]
        with patch_status(client, *statuses) as status_mock:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-container', 'result': True})
        self.assertEqual(2, status_mock.call_count)
        self.assertEqual(deploy_iter.next(), {
            'test_id': 'remove-machine-many-instance'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-instance'})
        for num in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'remove-machine', '-m', 'steve',
                str(num + 1)), num)

        statuses = [
            {'machines': {'1': dict(machine_started)}, 'applications': {}},
            {'machines': {}, 'applications': {}},
        ]
        with patch_status(client, *statuses) as status_mock:
            self.assertEqual(
                deploy_iter.next(),
                {'test_id': 'remove-machine-many-instance', 'result': True})
        self.assertEqual(2, status_mock.call_count)

    def test_iter_step_failure(self):
        deploy_many = DeployManyAttempt()
        client = FakeEnvJujuClient()
        client.env.config['default-series'] = 'angsty'
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = {
            'machines': {'0': {'agent-state': 'started'}},
            'applications': {},
        }
        with patch_status(client, status):
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-m', 'steve'), index)

        status = {
            'machines': dict((str(x), {'agent-state': 'started'})
                             for x in range(deploy_many.host_count + 1)),
            'applications': {},
        }
        with patch_status(client, status):
                self.assertEqual(
                    deploy_iter.next(),
                    {'test_id': 'add-machine-many', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        with patch_status(client, status):
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'ensure-machines', 'result': True})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'deploy-many'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'deploy-many'})
        status = {
            'machines': {
                '0': {'agent-state': 'pending'},
                },
            'applications': {},
        }
        with patch_status(client, status):
            with self.assertRaisesRegexp(
                    Exception,
                    'Timed out waiting for agents to start in steve.'):
                deploy_iter.next()

    def test_iter_step_add_machine_failure(self):
        deploy_many = DeployManyAttempt()
        client = FakeEnvJujuClient()
        client.env.config['default-series'] = 'angsty'
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        self.assertEqual(deploy_iter.next(), {'test_id': 'add-machine-many'})
        status = {
            'machines': {'0': {'agent-state': 'started'}},
            'applications': {},
        }
        with patch_status(client, status) as gs_mock:
            with patch('subprocess.check_call') as mock_cc:
                self.assertEqual(deploy_iter.next(),
                                 {'test_id': 'add-machine-many'})
        for index in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-m', 'steve'), index)
        gs_mock.assert_called_once_with()

        status = {
            'machines': dict((str(x), {'agent-state': 'pending'})
                             for x in range(deploy_many.host_count + 1)),
            'applications': {},
        }
        with patch_status(client, status) as gs_mock:
            self.assertEqual(deploy_iter.next(),
                             {'test_id': 'add-machine-many', 'result': False})
        self.assertEqual(deploy_iter.next(),
                         {'test_id': 'ensure-machines'})
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual({'test_id': 'ensure-machines'},
                             deploy_iter.next())
        for x in range(deploy_many.host_count):
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'remove-machine', '-m', 'steve',
                '--force', str((x + 1))), x * 2)
            assert_juju_call(self, mock_cc, client, (
                'juju', '--show-log', 'add-machine', '-m', 'steve'), x * 2 + 1)

        status = {
            'machines': dict((str(x), {'agent-state': 'started'})
                             for x in range(deploy_many.host_count + 1)),
            'applications': {},
        }
        with patch_status(client, status) as gs_mock:
            self.assertEqual({'test_id': 'ensure-machines', 'result': True},
                             deploy_iter.next())
        self.assertEqual({'test_id': 'deploy-many'}, deploy_iter.next())
        with patch('subprocess.check_call') as mock_cc:
            self.assertEqual({'test_id': 'deploy-many'}, deploy_iter.next())
        calls = self.predict_add_machine_calls(deploy_many, LXD_MACHINE)
        for num, args in enumerate(calls):
            assert_juju_call(self, mock_cc, client, args, num)

    def get_wait_until_removed_timeout(self, container_type):
        deploy_many = DeployManyAttempt()
        client = fake_juju_client()
        client.bootstrap()
        deploy_iter = iter_steps_validate_info(self, deploy_many, client)
        with patch('industrial_test.wait_until_removed') as wur_mock:
            with patch.object(client, 'preferred_container',
                              return_value=container_type):
                list(deploy_iter)
        return wur_mock.mock_calls[0][2]['timeout']

    def test_wait_until_removed_timeout_lxd(self):
        self.assertEqual(60, self.get_wait_until_removed_timeout(LXD_MACHINE))

    def test_wait_until_removed_timeout_lxc(self):
        self.assertEqual(30, self.get_wait_until_removed_timeout(LXC_MACHINE))


class TestBackupRestoreAttempt(JujuPyTestCase):

    def test_get_test_info(self):
        self.assertEqual(
            BackupRestoreAttempt.get_test_info(),
            {'back-up-restore': {'title': 'Back-up / restore'}})

    def test_iter_steps(self):
        br_attempt = BackupRestoreAttempt()
        client = FakeEnvJujuClient()
        aws_env = get_aws_env()
        client.env.environment = aws_env.environment
        client.env.config = aws_env.config
        client.env.juju_home = aws_env.juju_home
        admin_client = client.get_admin_client()
        environ = dict(os.environ)
        environ.update(get_euca_env(client.env.config))

        def check_output(*args, **kwargs):
            if args == (('juju', '--show-log', 'create-backup', '-m',
                         admin_client.env.environment,),):
                return FakePopen('juju-backup-24.tgz', '', 0)
            self.assertEqual([], args)
        initial_status = {
            'machines': {'0': {
                'instance-id': 'asdf',
                'dns-name': '128.100.100.128',
                }}
        }
        iterator = iter_steps_validate_info(self, br_attempt, client)
        self.assertEqual(iterator.next(), {'test_id': 'back-up-restore'})
        with patch_status(admin_client, initial_status) as gs_mock:
            with patch('subprocess.Popen',
                       side_effect=check_output) as co_mock:
                with patch('subprocess.check_call') as cc_mock:
                    with patch.object(client, 'get_admin_client',
                                      return_value=admin_client,
                                      autospec=True):
                        with patch('sys.stdout'):
                            self.assertEqual(
                                iterator.next(),
                                {'test_id': 'back-up-restore'})
        assert_juju_call(self, co_mock, client, (
            'juju', '--show-log', 'create-backup', '-m',
            admin_client.env.environment), 0)
        self.assertEqual(
            cc_mock.mock_calls[0],
            call(['euca-terminate-instances', 'asdf'], env=environ))
        with patch('deploy_stack.wait_for_port'):
            with patch('deploy_stack.print_now', autospec=True) as pn_mock:
                self.assertEqual(iterator.next(),
                                 {'test_id': 'back-up-restore'})
        pn_mock.assert_called_with('Closed.')
        with patch.object(admin_client, 'restore_backup') as rb_mock:
            self.assertEqual(iterator.next(), {'test_id': 'back-up-restore'})
        rb_mock.assert_called_once_with(
            os.path.abspath('juju-backup-24.tgz'))
        with patch('os.unlink') as ul_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'back-up-restore'})
        ul_mock.assert_called_once_with(os.path.abspath('juju-backup-24.tgz'))
        final_status = {
            'machines': {
                '0': {'agent-state': 'started'},
                },
            'applications': {},
        }
        with patch_status(admin_client, final_status) as gs_mock:
            self.assertEqual(iterator.next(),
                             {'test_id': 'back-up-restore', 'result': True})
        gs_mock.assert_called_once_with()


class TestPrepareUpgradeJujuAttempt(JujuPyTestCase):

    def test_factory(self):
        uj_attempt = PrepareUpgradeJujuAttempt.factory(
            ['a', 'b', 'c'], None)
        self.assertIs(type(uj_attempt), PrepareUpgradeJujuAttempt)
        self.assertEqual(uj_attempt.bootstrap_paths, {'b': 'a', 'c': 'b'})

    def test_factory_empty(self):
        with self.assertRaisesRegexp(
                ValueError, 'Not enough paths for upgrade.'):
            PrepareUpgradeJujuAttempt.factory(['a'], None)
        with self.assertRaisesRegexp(
                ValueError, 'Not enough paths for upgrade.'):
            PrepareUpgradeJujuAttempt.factory([], None)

    def test_get_bootstrap_client(self):
        client = fake_juju_client(full_path='c', debug=True)
        puj_attempt = PrepareUpgradeJujuAttempt.factory(['a', 'b', 'c'], None)

        def by_version(env, path, debug):
            return fake_juju_client(env, path, debug)

        with patch.object(client, 'by_version', by_version):
            bootstrap_client = puj_attempt.get_bootstrap_client(client)

        self.assertIsNot(bootstrap_client, client)
        self.assertIs(client.debug, bootstrap_client.debug)
        self.assertIs(client.env, bootstrap_client.env)
        self.assertEqual('b', bootstrap_client.full_path)

    def test_iter_steps(self):
        future_client = FakeEnvJujuClient(full_path='/future/juju')
        present_client = FakeEnvJujuClient(full_path='/present/juju')
        puj_attempt = PrepareUpgradeJujuAttempt(
            {future_client.full_path: present_client.full_path})
        puj_iterator = iter_steps_validate_info(self, puj_attempt,
                                                future_client)
        with patch('subprocess.check_output', return_value='2.0-alpha3-a-b'):
            self.assertEqual({'test_id': 'prepare-upgrade-juju'},
                             puj_iterator.next())
        with observable_temp_file() as config_file:
            with patch('subprocess.Popen') as po_mock:
                self.assertEqual({'test_id': 'prepare-upgrade-juju'},
                                 puj_iterator.next())
            assert_juju_call(self, po_mock, present_client, (
                'juju', '--show-log', 'bootstrap', '--constraints', 'mem=2G',
                'steve', 'fake/regionx', '--config', config_file.name,
                '--agent-version', '2.0-alpha3'))
            po_mock.return_value.wait.return_value = 0
            self.assertEqual(puj_iterator.next(),
                             {'test_id': 'prepare-upgrade-juju'})
        b_status = {
            'machines': {'0': {'agent-state': 'started'}},
            'applications': {},
        }
        with patch_status(None, b_status):
            self.assertEqual(
                puj_iterator.next(),
                {'test_id': 'prepare-upgrade-juju', 'result': True})

    def test_iter_steps_no_previous_client(self):
        uj_attempt = PrepareUpgradeJujuAttempt({})
        client = FakeEnvJujuClient(full_path='/present/juju')
        uj_iterator = uj_attempt.iter_steps(client)
        with self.assertRaises(CannotUpgradeToClient) as exc_context:
            uj_iterator.next()
        self.assertIs(exc_context.exception.client, client)


class TestUpgradeJujuAttempt(JujuPyTestCase):

    def test_iter_steps(self):
        future_client = FakeEnvJujuClient(full_path='/future/juju')
        uj_attempt = UpgradeJujuAttempt()
        uj_iterator = iter_steps_validate_info(self, uj_attempt, future_client)
        self.assertEqual(uj_iterator.next(), {'test_id': 'upgrade-juju'})
        with patch('subprocess.check_call') as cc_mock:
            self.assertEqual({'test_id': 'upgrade-juju'}, uj_iterator.next())
        assert_juju_call(self, cc_mock, future_client, (
            'juju', '--show-log', 'upgrade-juju', '-m', 'steve', '--version',
            future_client.get_matching_agent_version()))
        version_status = {
            'machines': {'0': {
                'agent-version': future_client.get_matching_agent_version()}},
            'applications': {},
        }
        with patch_status(None, version_status):
            self.assertEqual({'test_id': 'upgrade-juju', 'result': True},
                             uj_iterator.next())


class TestUpgradeCharmAttempt(JujuPyTestCase):

    def assert_hook(self, hook_path, content):
        with open(hook_path) as hook_file:
            self.assertEqual(hook_file.read(), content)
            mode = os.fstat(hook_file.fileno()).st_mode
        self.assertEqual(0o755, mode & 0o777)

    def test_iter_steps(self):
        client = FakeEnvJujuClient(version='2.0.0', full_path='/future/juju')
        self._iter_steps(client)

    def test_iter_steps_juju_1x(self):
        client = FakeEnvJujuClient1X(version='1.25.0',
                                     full_path='/future/juju')
        self._iter_steps(client)

    def _iter_steps(self, client):
        self.assertEqual(client.full_path, '/future/juju')
        uc_attempt = UpgradeCharmAttempt()
        uc_iterator = iter_steps_validate_info(self, uc_attempt, client)
        self.assertEqual(uc_iterator.next(),
                         {'test_id': 'prepare-upgrade-charm'})
        temp_repository = mkdtemp()
        with patch('utility.mkdtemp', return_value=temp_repository):
            with patch('subprocess.check_call') as cc_mock:
                self.assertEqual(uc_iterator.next(),
                                 {'test_id': 'prepare-upgrade-charm'})
        metadata_path = os.path.join(
            temp_repository, 'trusty', 'mycharm', 'metadata.yaml')
        with open(metadata_path) as metadata_file:
            metadata = yaml.safe_load(metadata_file)
        self.assertEqual(metadata['name'], 'mycharm')
        self.assertIn('summary', metadata)
        self.assertIn('description', metadata)
        if client.version.startswith('1.'):
            self.assertIn('series', metadata)
            charm_path = os.path.join('local:trusty', 'mycharm')
            assert_juju_call(self, cc_mock, client, (
                'juju', '--show-log', 'deploy', '-e', 'steve', charm_path,
                '--repository', temp_repository))
            option = '-e'
        else:
            self.assertIn('series', metadata)
            charm_path = os.path.join(temp_repository, 'trusty', 'mycharm')
            assert_juju_call(self, cc_mock, client, (
                'juju', '--show-log', 'deploy', '-m', 'steve', charm_path))
            option = '-m'
        self.assertNotIn('min-juju-version', metadata)
        status = {
            'machines': {'0': {'agent-state': 'started'}},
            'applications': {},
        }
        with patch_status(client, status):
            self.assertEqual(uc_iterator.next(),
                             {'test_id': 'prepare-upgrade-charm'})
        hooks_path = os.path.join(temp_repository, 'trusty', 'mycharm',
                                  'hooks')
        upgrade_path = os.path.join(hooks_path, 'upgrade-charm')
        config_changed = os.path.join(hooks_path, 'config-changed')
        self.assertFalse(os.path.exists(config_changed))
        self.assertFalse(os.path.exists(upgrade_path))
        self.assertEqual(
            uc_iterator.next(),
            {'test_id': 'prepare-upgrade-charm', 'result': True})
        self.assert_hook(upgrade_path, dedent("""\
            #!/bin/sh
            open-port 42
            """))
        self.assert_hook(config_changed, dedent("""\
            #!/bin/sh
            open-port 34
            """))
        self.assertEqual(uc_iterator.next(), {'test_id': 'upgrade-charm'})
        with patch('subprocess.check_call') as cc_mock:
            self.assertEqual(uc_iterator.next(), {'test_id': 'upgrade-charm'})
        if client.version.startswith('1.'):
            assert_juju_call(self, cc_mock, client, (
                'juju', '--show-log', 'upgrade-charm', option, 'steve',
                'mycharm', '--repository', temp_repository))
        else:
            assert_juju_call(self, cc_mock, client, (
                'juju', '--show-log', 'upgrade-charm', option, 'steve',
                'mycharm', '--path', os.path.join(temp_repository, 'trusty',
                                                  'mycharm')))
        status = {
            'machines': {'0': {'agent-state': 'started'}},
            'applications': {'mycharm': {'units': {'mycharm/0': {
                'open-ports': ['42/tcp', '34/tcp'],
                }}}},
        }
        with patch_status(client, status):
            self.assertEqual(
                uc_iterator.next(),
                {'test_id': 'upgrade-charm', 'result': True})


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


class TestMakeSubstrate(JujuPyTestCase):

    def test_make_substrate_manager_no_support(self):
        client = EnvJujuClient(JujuData('foo', {'type': 'foo'}),
                               '', '')
        with make_substrate_manager(client, []) as substrate:
            self.assertIs(substrate, None)

    def test_make_substrate_no_requirements(self):
        client = EnvJujuClient(get_aws_juju_data(), '', '')
        with make_substrate_manager(client, []) as substrate:
            self.assertIs(type(substrate), AWSAccount)

    def test_make_substrate_manager_unsatisifed_requirements(self):
        client = EnvJujuClient(get_aws_juju_data(), '', '')
        with make_substrate_manager(client, ['foo']) as substrate:
            self.assertIs(substrate, None)
        with make_substrate_manager(
                client, ['iter_security_groups', 'foo']) as substrate:
            self.assertIs(substrate, None)

    def test_make_substrate_satisfied_requirements(self):
        client = EnvJujuClient(get_aws_juju_data(), '', '')
        with make_substrate_manager(
                client, ['iter_security_groups']) as substrate:
            self.assertIs(type(substrate), AWSAccount)
        with make_substrate_manager(
                client, ['iter_security_groups',
                         'iter_instance_security_groups']
                ) as substrate:
            self.assertIs(type(substrate), AWSAccount)


class TestStageInfo(TestCase):

    def test_ctor(self):
        si = StageInfo('foo-id', 'Foo title')
        self.assertEqual(si.stage_id, 'foo-id')
        self.assertEqual(si.title, 'Foo title')
        self.assertEqual(si.report_on, True)

        si = StageInfo('foo-id', 'Foo title', False)
        self.assertEqual(si.report_on, False)

    def test_as_tuple(self):
        si = StageInfo('foo-id', 'Foo title')
        self.assertEqual(
            si.as_tuple(),
            ('foo-id', {'title': 'Foo title', 'report_on': True}))
        si = StageInfo('bar-id', 'Bar title', False)
        self.assertEqual(
            si.as_tuple(),
            ('bar-id', {'title': 'Bar title', 'report_on': False}))

    def test_as_result(self):
        si = StageInfo('foo-id', 'Foo title')
        self.assertEqual(si.as_result(), {'test_id': 'foo-id'})
        self.assertEqual(si.as_result(True),
                         {'test_id': 'foo-id', 'result': True})
        self.assertEqual(si.as_result(False),
                         {'test_id': 'foo-id', 'result': False})


class TestAttemptSuiteFactory(TestCase):

    def test_factory(self):
        fake_bootstrap = FakeAttemptClass('bootstrap')
        factory = AttemptSuiteFactory([],
                                      bootstrap_attempt=fake_bootstrap)
        attempt_suite = factory.factory(['1', '2'], 'log-1', 'foo-stream')
        self.assertEqual(factory, attempt_suite.attempt_list)
        self.assertEqual(['1', '2'], attempt_suite.upgrade_sequence)
        self.assertEqual('log-1', attempt_suite.log_dir)
        self.assertEqual('foo-stream', attempt_suite.agent_stream)

    def test_get_test_info(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap')
        fake_1 = FakeAttemptClass('fake-1')
        fake_2 = FakeAttemptClass('fake-2')
        factory = AttemptSuiteFactory([fake_1, fake_2],
                                      bootstrap_attempt=fake_bootstrap)
        self.assertEqual(OrderedDict([
            ('fake-bootstrap-id', {'title': 'fake-bootstrap'}),
            ('prepare-suite', {'title': 'Prepare suite tests',
                               'report_on': False}),
            ('fake-1-id', {'title': 'fake-1'}),
            ('fake-2-id', {'title': 'fake-2'}),
            ('destroy-env', {'title': 'destroy environment',
                             'report_on': True}),
            ('substrate-clean', {'title': 'check substrate clean',
                                 'report_on': True}),
            ]), factory.get_test_info())


class TestAttemptSuite(TestCase):

    def test_get_test_info(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap')
        fake_1 = FakeAttemptClass('fake-1')
        fake_2 = FakeAttemptClass('fake-2')
        factory = AttemptSuiteFactory([fake_1, fake_2],
                                      bootstrap_attempt=fake_bootstrap)
        attempt_suite = AttemptSuite(factory, None, None, None)
        self.assertEqual(OrderedDict([
            ('fake-bootstrap-id', {'title': 'fake-bootstrap'}),
            ('prepare-suite', {'title': 'Prepare suite tests',
                               'report_on': False}),
            ('fake-1-id', {'title': 'fake-1'}),
            ('fake-2-id', {'title': 'fake-2'}),
            ('destroy-env', {'title': 'destroy environment',
                             'report_on': True}),
            ('substrate-clean', {'title': 'check substrate clean',
                                 'report_on': True}),
            ]), attempt_suite.get_test_info())

    @contextmanager
    def iter_steps_cxt(self, attempt_suite):
        with patch('industrial_test.BootstrapManager') as mock_bm:
            with patch.object(attempt_suite,
                              '_iter_bs_manager_steps') as mock_ibms:
                with patch('industrial_test.make_log_dir',
                           return_value='qux-1'):
                    yield (mock_ibms, mock_bm)

    def test_iter_steps(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap', '1', '2')
        factory = AttemptSuiteFactory([], bootstrap_attempt=fake_bootstrap)
        attempt_suite = AttemptSuite(factory, None, 'asdf', None)
        with self.iter_steps_cxt(attempt_suite) as (mock_ibms, mock_bm):
            client = fake_juju_client()
            attempt_suite.iter_steps(client)
        mock_bm.assert_called_once_with(
            'name', client, client, agent_stream=None, agent_url=None,
            bootstrap_host=None, jes_enabled=True, keep_env=True,
            log_dir='qux-1', machines=[], permanent=True,
            region=None, series=None)

    def test_iter_steps_agent_stream(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap', '1', '2')
        factory = AttemptSuiteFactory([], bootstrap_attempt=fake_bootstrap)
        attempt_suite = AttemptSuite(factory, None, 'asdf', 'bar-stream')
        with self.iter_steps_cxt(attempt_suite) as (mock_ibms, mock_bm):
            client = fake_juju_client()
            iterator = attempt_suite.iter_steps(client)
        self.assertEqual(iterator, mock_ibms.return_value)
        mock_bm.assert_called_once_with(
            'name', client, client, agent_stream='bar-stream', agent_url=None,
            bootstrap_host=None, jes_enabled=True, keep_env=True,
            log_dir='qux-1', machines=[], permanent=True,
            region=None, series=None)

    def test__iter_bs_manager_steps(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap', '1', '2')
        fake_1 = FakeAttemptClass('fake-1', '1', '2')
        fake_2 = FakeAttemptClass('fake-2', '1', '2')
        factory = AttemptSuiteFactory([fake_1, fake_2],
                                      bootstrap_attempt=fake_bootstrap)
        attempt_suite = AttemptSuite(factory, None, None, None)
        client = fake_juju_client()
        bs_manager = FakeBootstrapManager(client)
        steps = list(attempt_suite._iter_bs_manager_steps(
            bs_manager, client, fake_bootstrap(), True))
        self.assertEqual([
            {'test_id': 'fake-bootstrap-id'},
            {'test_id': 'fake-bootstrap-id', 'result': '1'},
            {'test_id': 'prepare-suite'},
            {'test_id': 'prepare-suite', 'result': True},
            {'test_id': 'fake-1-id'},
            {'test_id': 'fake-1-id', 'result': '1'},
            {'test_id': 'fake-2-id'},
            {'test_id': 'fake-2-id', 'result': '1'},
            {'test_id': 'destroy-env'},
            {'test_id': 'destroy-env', 'result': True},
            {'test_id': 'substrate-clean'},
            {'test_id': 'substrate-clean', 'result': True},
            ], steps)

    def test__iter_bs_manager_steps_teardown_in_runtime(self):
        fake_bootstrap = FakeAttemptClass('fake-bootstrap', '1', '2')
        fake_1 = FakeAttemptClass('fake-1', Exception('fake exception'), '2')
        factory = AttemptSuiteFactory([fake_1],
                                      bootstrap_attempt=fake_bootstrap)
        attempt_suite = AttemptSuite(factory, None, None, None)
        client = fake_juju_client()
        bs_manager = FakeBootstrapManager(client, keep_env=True)
        with self.assertRaisesRegexp(Exception, 'fake exception'):
            list(attempt_suite._iter_bs_manager_steps(
                bs_manager, client, fake_bootstrap(), True))
        self.assertIs(True, bs_manager.torn_down)
