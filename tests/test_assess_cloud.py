from argparse import Namespace
from contextlib import contextmanager
import logging

from mock import (
    call,
    patch,
    )
from assess_cloud import (
    assess_cloud_combined,
    assess_cloud_kill_controller,
    assess_cloud_provisioning,
    client_from_args,
    main,
    parse_args,
    )
from deploy_stack import BootstrapManager
from jujupy import (
    EnvJujuClient,
    FakeBackend,
    fake_juju_client,
    Juju2Backend,
    )
from tests import (
    FakeHomeTestCase,
    observable_temp_file,
    TestCase,
    )
from utility import (
    temp_dir,
    temp_yaml_file,
    )


def backend_call(client, cmd, args, model=None, check=True, timeout=None,
                 extra_env=None):
    """Return the mock.call for this command."""
    return call(cmd, args, client.used_feature_flags,
                client.env.juju_home, model, check, timeout, extra_env)


@contextmanager
def mocked_bs_manager(juju_home):
    client = fake_juju_client()
    client.env.juju_home = juju_home
    bs_manager = BootstrapManager(
        'foo', client, client, bootstrap_host=None, machines=[],
        series=None, agent_url=None, agent_stream=None, region=None,
        log_dir=juju_home, keep_env=False, permanent=True,
        jes_enabled=True, logged_exception_exit=False)
    backend = client._backend
    with patch.object(backend, 'juju', wraps=backend.juju):
        with observable_temp_file() as temp_file:
            yield bs_manager, temp_file


def strip_calls(calls):
    """Strip out irrelevant / non-action calls."""
    new_calls = []
    for num, juju_call in enumerate(calls):
        cls, args, kwargs = juju_call
        # Ignore initial teardown
        if num == 0 and args[0] == 'kill-controller':
            continue
        if args[0] in('list-controllers', 'list-models', 'show-status'):
            continue
        new_calls.append(juju_call)
    return new_calls


class TestAssessCloudCombined(FakeHomeTestCase):

    def test_assess_cloud_combined(self):
        with self.check_assess_cloud_combined(self) as bs_manager:
            assess_cloud_combined(bs_manager)

    @staticmethod
    @contextmanager
    def check_assess_cloud_combined(test_case):
        with mocked_bs_manager(test_case.juju_home) as (bs_manager,
                                                        config_file):
            yield bs_manager
            client = bs_manager.client
            juju_wrapper = client._backend.juju
        test_case.assertEqual([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(client, 'deploy', 'ubuntu', 'foo:foo'),
            backend_call(client, 'remove-unit', 'ubuntu/0', 'foo:foo'),
            backend_call(
                client, 'destroy-controller',
                ('foo', '-y', '--destroy-all-models'), timeout=600),
            ], strip_calls(juju_wrapper.mock_calls))


class TestAssessCloudKillController(FakeHomeTestCase):

    def test_assess_cloud_kill_controller(self):
        with self.check_assess_cloud_kill_controller(self) as bs_manager:
            assess_cloud_kill_controller(bs_manager)

    @staticmethod
    @contextmanager
    def check_assess_cloud_kill_controller(test_case):
        with mocked_bs_manager(test_case.juju_home) as (bs_manager,
                                                        config_file):
            yield bs_manager
            client = bs_manager.client
            juju_wrapper = client._backend.juju
        test_case.assertEqual([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(
                client, 'kill-controller', ('foo', '-y'), timeout=600,
                check=True),
            ], strip_calls(juju_wrapper.mock_calls))


class TestAssessCloudProvisioning(FakeHomeTestCase):

    def test_assess_cloud_provisioning(self):
        with self.check_assess_cloud_provisioning(self) as bs_manager:
            assess_cloud_provisioning(bs_manager)

    @staticmethod
    @contextmanager
    def check_assess_cloud_provisioning(test_case):
        with mocked_bs_manager(test_case.juju_home) as (bs_manager,
                                                        config_file):
            client = bs_manager.client
            yield bs_manager
            juju_wrapper = client._backend.juju
        test_case.assertEqual([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(client, 'add-machine', ('--series', 'win2012r2'),
                         'foo:foo'),
            backend_call(client, 'add-machine', ('--series', 'trusty'),
                         'foo:foo'),
            backend_call(client, 'remove-machine', ('0',), 'foo:foo'),
            backend_call(client, 'remove-machine', ('1',), 'foo:foo'),
            backend_call(
                client, 'destroy-controller',
                ('foo', '-y', '--destroy-all-models'), timeout=600),
            ], strip_calls(juju_wrapper.mock_calls))


class TestClientFromArgs(FakeHomeTestCase):

    def test_client_from_args(self):
        with temp_yaml_file({}) as clouds_file:
            args = Namespace(
                juju_bin='/usr/bin/juju', clouds_file=clouds_file,
                cloud='mycloud', region=None, debug=False, deadline=None,
                config=None)
            with patch.object(EnvJujuClient.config_class,
                              'from_cloud_region') as fcr_mock:
                with patch.object(EnvJujuClient, 'get_version',
                                  return_value='2.0.x'):
                    client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), EnvJujuClient)
        self.assertIs(type(client._backend), Juju2Backend)
        self.assertEqual(client.version, '2.0.x')
        self.assertIs(client.env, fcr_mock.return_value)

    def test_client_from_args_fake(self):
        with temp_yaml_file({}) as clouds_file:
            args = Namespace(
                juju_bin='FAKE', clouds_file=clouds_file, cloud='mycloud',
                region=None, debug=False, deadline=None, config=None)
            with patch.object(EnvJujuClient.config_class,
                              'from_cloud_region') as fcr_mock:
                client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), EnvJujuClient)
        self.assertIs(type(client._backend), FakeBackend)
        self.assertEqual(client.version, '2.0.0')
        self.assertIs(client.env, fcr_mock.return_value)

    def test_config(self):
        with temp_yaml_file({}) as clouds_file:
            with temp_yaml_file({'foo': 'bar'}) as config_file:
                args = Namespace(
                    juju_bin='/usr/bin/juju', clouds_file=clouds_file,
                    cloud='mycloud', region=None, debug=False, deadline=None,
                    config=config_file)
                with patch.object(EnvJujuClient.config_class,
                                  'from_cloud_region') as fcr_mock:
                    with patch.object(EnvJujuClient, 'get_version',
                                      return_value='2.0.x'):
                        client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {'foo': 'bar'}, {},
                                         self.juju_home)


class TestParseArgs(TestCase):

    def test_parse_args_combined(self):
        with temp_dir() as log_dir:
            args = parse_args(['combined', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='combined', config=None,
            ))

    def test_parse_args_combined_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['combined', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)

    def test_parse_args_kill_controller(self):
        with temp_dir() as log_dir:
            args = parse_args(['kill-controller', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='kill-controller', config=None,
            ))

    def test_parse_args_kill_controller_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['kill-controller', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)

    def test_parse_args_provisioning(self):
        with temp_dir() as log_dir:
            args = parse_args(['provisioning', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='provisioning', config=None,
            ))

    def test_parse_args_provisioning_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['provisioning', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)


class TestMain(FakeHomeTestCase):

    @contextmanager
    def main_cxt(self, check_cxt):
        with temp_yaml_file({
                'clouds': {'cloud': {'type': 'foo'}}
                }) as clouds_file:
            with check_cxt as bs_manager:
                with patch.object(BootstrapManager, 'from_client',
                                  return_value=bs_manager):
                    yield clouds_file

    def test_main_provisioning(self):
        tacp = TestAssessCloudProvisioning
        check_cxt = tacp.check_assess_cloud_provisioning(self)
        with self.main_cxt(check_cxt) as clouds_file:
            main(['provisioning', clouds_file, 'cloud', 'FAKE'])

    def test_main_kill_controller(self):
        tackc = TestAssessCloudKillController
        check_cxt = tackc.check_assess_cloud_kill_controller(self)
        with self.main_cxt(check_cxt) as clouds_file:
            main(['kill-controller', clouds_file, 'cloud', 'FAKE'])

    def test_main_combined(self):
        tacc = TestAssessCloudCombined
        check_cxt = tacc.check_assess_cloud_combined(self)
        with self.main_cxt(check_cxt) as clouds_file:
            main(['combined', clouds_file, 'cloud', 'FAKE'])
