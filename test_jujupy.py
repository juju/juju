__metaclass__ = type

from collections import defaultdict
from contextlib import contextmanager
from datetime import (
    datetime,
    timedelta,
)
import os
import shutil
import StringIO
import subprocess
import sys
import tempfile
from textwrap import dedent
from unittest import TestCase

from mock import (
    call,
    MagicMock,
    patch,
)
import yaml

from jujuconfig import (
    get_environments_path,
    get_jenv_path,
    get_juju_home,
    NoSuchEnvironment,
)
from jujupy import (
    CannotConnectEnv,
    Environment,
    EnvJujuClient,
    EnvJujuClient22,
    EnvJujuClient24,
    EnvJujuClient25,
    EnvJujuClient26,
    ErroredUnit,
    GroupReporter,
    get_cache_path,
    get_local_root,
    get_timeout_path,
    jes_home_path,
    JESByDefault,
    JESNotSupported,
    JujuClientDevel,
    JUJU_DEV_FEATURE_FLAGS,
    make_client,
    make_jes_home,
    parse_new_state_server_from_error,
    SimpleEnvironment,
    Status,
    temp_bootstrap_env,
    _temp_env as temp_env,
    uniquify_local,
)
from utility import (
    scoped_environ,
    temp_dir,
)


def assert_juju_call(test_case, mock_method, client, expected_args,
                     call_index=None, assign_stderr=False):
    if call_index is None:
        test_case.assertEqual(len(mock_method.mock_calls), 1)
        call_index = 0
    empty, args, kwargs = mock_method.mock_calls[call_index]
    test_case.assertEqual(args, (expected_args,))
    kwarg_keys = ['env']
    if assign_stderr:
        with tempfile.TemporaryFile() as example:
            # 'example' is a pragmatic way of checking file types in py2 and 3.
            kwarg_keys = ['stderr'] + kwarg_keys
            test_case.assertIsInstance(kwargs['stderr'], type(example))
    test_case.assertItemsEqual(kwargs.keys(), kwarg_keys)
    bin_dir = os.path.dirname(client.full_path)
    test_case.assertRegexpMatches(kwargs['env']['PATH'],
                                  r'^{}\:'.format(bin_dir))


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('bar', 'baz')
        self.assertEqual('bar is in state baz', str(e))


class ClientTest(TestCase):

    def setUp(self):
        super(ClientTest, self).setUp()
        patcher = patch('jujupy.pause')
        self.addCleanup(patcher.stop)
        self.pause_mock = patcher.start()


class CloudSigmaTest:

    def test__shell_environ_no_flags(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.25-foobar', 'path')
        env = client._shell_environ()
        self.assertEqual(env.get(JUJU_DEV_FEATURE_FLAGS, ''), '')

    def test__shell_environ_cloudsigma(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'cloudsigma'}),
            '1.25-foobar', 'path')
        env = client._shell_environ()
        self.assertTrue('cloudsigma' in env[JUJU_DEV_FEATURE_FLAGS].split(","))

    def test__shell_environ_juju_home(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.25-foobar', 'path',
            'asdf')
        env = client._shell_environ()
        self.assertEqual(env['JUJU_HOME'], 'asdf')


class TestEnvJujuClient26(ClientTest, CloudSigmaTest):

    client_class = EnvJujuClient26

    def test_enable_jes_already_supported(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        with patch('subprocess.check_output', autospec=True,
                   return_value='system') as co_mock:
            with self.assertRaises(JESByDefault):
                client.enable_jes()
        self.assertFalse(client._use_jes)
        assert_juju_call(
            self, co_mock, client, ('juju', '--show-log', 'help', 'commands'),
            assign_stderr=True)

    def test_enable_jes_unsupported(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        with patch('subprocess.check_output', autospec=True,
                   return_value='') as co_mock:
            with self.assertRaises(JESNotSupported):
                client.enable_jes()
        self.assertFalse(client._use_jes)
        assert_juju_call(
            self, co_mock, client, ('juju', '--show-log', 'help', 'commands'),
            0, assign_stderr=True)
        assert_juju_call(
            self, co_mock, client, ('juju', '--show-log', 'help', 'commands'),
            1, assign_stderr=True)
        self.assertEqual(co_mock.call_count, 2)

    def test_enable_jes_requires_flag(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        with patch('subprocess.check_output', autospec=True,
                   side_effect=['', 'system']) as co_mock:
            client.enable_jes()
        self.assertTrue(client._use_jes)
        assert_juju_call(
            self, co_mock, client, ('juju', '--show-log', 'help', 'commands'),
            0, assign_stderr=True)
        assert_juju_call(
            self, co_mock, client, ('juju', '--show-log', 'help', 'commands'),
            1, assign_stderr=True)
        self.assertEqual(co_mock.call_count, 2)

    def test__shell_environ_jes(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        client._use_jes = True
        env = client._shell_environ()
        self.assertIn('jes', env[JUJU_DEV_FEATURE_FLAGS].split(","))

    def test__shell_environ_jes_cloudsigma(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'cloudsigma'}),
            '1.25-foobar', 'path')
        client._use_jes = True
        env = client._shell_environ()
        flags = env[JUJU_DEV_FEATURE_FLAGS].split(",")
        self.assertItemsEqual(['cloudsigma', 'jes'], flags)


class TestEnvJujuClient25(TestEnvJujuClient26):

    client_class = EnvJujuClient25


class TestEnvJujuClient22(ClientTest):

    client_class = EnvJujuClient22

    def test__shell_environ(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.22-foobar', 'path')
        env = client._shell_environ()
        self.assertEqual(env.get(JUJU_DEV_FEATURE_FLAGS), 'actions')

    def test__shell_environ_juju_home(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.22-foobar', 'path',
            'asdf')
        env = client._shell_environ()
        self.assertEqual(env['JUJU_HOME'], 'asdf')


class TestEnvJujuClient24(ClientTest, CloudSigmaTest):

    client_class = EnvJujuClient24

    def test_no_jes(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'cloudsigma'}),
            '1.25-foobar', 'path')
        with self.assertRaises(JESNotSupported):
            client.enable_jes()
        client._use_jes = True
        env = client._shell_environ()
        self.assertNotIn('jes', env[JUJU_DEV_FEATURE_FLAGS].split(","))


class TestEnvJujuClient(ClientTest):

    def test_get_version(self):
        value = ' 5.6 \n'
        with patch('subprocess.check_output', return_value=value) as vsn:
            version = EnvJujuClient.get_version()
        self.assertEqual('5.6', version)
        vsn.assert_called_with(('juju', '--version'))

    def test_get_version_path(self):
        with patch('subprocess.check_output', return_value=' 4.3') as vsn:
            EnvJujuClient.get_version('foo/bar/baz')
        vsn.assert_called_once_with(('foo/bar/baz', '--version'))

    def test_get_matching_agent_version(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        self.assertEqual('1.23.1', client.get_matching_agent_version())
        self.assertEqual('1.23', client.get_matching_agent_version(
                         no_build=True))
        client.version = '1.20-beta1-series-arch'
        self.assertEqual('1.20-beta1.1', client.get_matching_agent_version())

    def test_upgrade_juju_nonlocal(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'nonlocal'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234'))

    def test_upgrade_juju_local(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234', '--upload-tools',))

    def test_upgrade_juju_no_force_version(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju(force_version=False)
        juju_mock.assert_called_with(
            'upgrade-juju', ('--upload-tools',))

    def test_by_version(self):
        def juju_cmd_iterator():
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'
            yield '1.22.1'
            yield '1.24-alpha1'
            yield '1.24.7'
            yield '1.25.1'
            yield '1.26.1'
            yield '1.27.1'

        context = patch.object(
            EnvJujuClient, 'get_version',
            side_effect=juju_cmd_iterator().send)
        with context:
            self.assertIs(EnvJujuClient,
                          type(EnvJujuClient.by_version(None)))
            with self.assertRaisesRegexp(Exception, 'Unsupported juju: 1.16'):
                EnvJujuClient.by_version(None)
            with self.assertRaisesRegexp(Exception,
                                         'Unsupported juju: 1.16.1'):
                EnvJujuClient.by_version(None)
            client = EnvJujuClient.by_version(None)
            self.assertIs(EnvJujuClient, type(client))
            self.assertEqual('1.15', client.version)
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient22)
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient24)
            self.assertEqual(client.version, '1.24-alpha1')
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient24)
            self.assertEqual(client.version, '1.24.7')
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient25)
            self.assertEqual(client.version, '1.25.1')
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient26)
            self.assertEqual(client.version, '1.26.1')
            client = EnvJujuClient.by_version(None)
            self.assertIs(type(client), EnvJujuClient)
            self.assertEqual(client.version, '1.27.1')

    def test_by_version_path(self):
        with patch('subprocess.check_output', return_value=' 4.3') as vsn:
            client = EnvJujuClient.by_version(None, 'foo/bar/qux')
        vsn.assert_called_once_with(('foo/bar/qux', '--version'))
        self.assertNotEqual(client.full_path, 'foo/bar/qux')
        self.assertEqual(client.full_path, os.path.abspath('foo/bar/qux'))

    def test_full_args(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, 'my/juju/bin')
        full = client._full_args('bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', '--show-log', 'bar', '-e', 'foo', 'baz',
                          'qux'), full)
        full = client._full_args('bar', True, ('baz', 'qux'))
        self.assertEqual((
            'juju', '--show-log', 'bar', '-e', 'foo',
            'baz', 'qux'), full)
        client.env = None
        full = client._full_args('bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', '--show-log', 'bar', 'baz', 'qux'), full)

    def test_full_args_debug(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, 'my/juju/bin')
        client.debug = True
        full = client._full_args('bar', False, ('baz', 'qux'))
        self.assertEqual((
            'juju', '--debug', 'bar', '-e', 'foo', 'baz', 'qux'), full)

    def test_full_args_action(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, 'my/juju/bin')
        full = client._full_args('action bar', False, ('baz', 'qux'))
        self.assertEqual((
            'juju', '--show-log', 'action', 'bar', '-e', 'foo', 'baz', 'qux'),
            full)

    def test_bootstrap_hpcloud(self):
        env = SimpleEnvironment('hp')
        with patch.object(env, 'hpcloud', lambda: True):
            with patch.object(EnvJujuClient, 'juju') as mock:
                EnvJujuClient(env, None, None).bootstrap()
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_maas(self):
        env = SimpleEnvironment('maas')
        with patch.object(EnvJujuClient, 'juju') as mock:
            client = EnvJujuClient(env, None, None)
            with patch.object(client.env, 'maas', lambda: True):
                client.bootstrap()
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G arch=amd64'), False)

    def test_bootstrap_joyent(self):
        env = SimpleEnvironment('joyent')
        with patch.object(EnvJujuClient, 'juju', autospec=True) as mock:
            client = EnvJujuClient(env, None, None)
            with patch.object(client.env, 'joyent', lambda: True):
                client.bootstrap()
            mock.assert_called_once_with(
                client, 'bootstrap', ('--constraints', 'mem=2G cpu-cores=1'),
                False)

    def test_bootstrap_non_sudo(self):
        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'juju') as mock:
            client = EnvJujuClient(env, None, None)
            with patch.object(client.env, 'needs_sudo', lambda: False):
                client.bootstrap()
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_sudo(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client.env, 'needs_sudo', lambda: True):
            with patch.object(client, 'juju') as mock:
                client.bootstrap()
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), True)

    def test_bootstrap_upload_tools(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client.env, 'needs_sudo', lambda: True):
            with patch.object(client, 'juju') as mock:
                client.bootstrap(upload_tools=True)
            mock.assert_called_with(
                'bootstrap', ('--upload-tools', '--constraints', 'mem=2G'),
                True)

    def test_bootstrap_async(self):
        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'juju_async', autospec=True) as mock:
            client = EnvJujuClient(env, None, None)
            client.juju_home = 'foo'
            with client.bootstrap_async():
                mock.assert_called_once_with(
                    client, 'bootstrap', ('--constraints', 'mem=2G'))

    def test_bootstrap_async_upload_tools(self):
        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'juju_async', autospec=True) as mock:
            client = EnvJujuClient(env, None, None)
            with client.bootstrap_async(upload_tools=True):
                mock.assert_called_with(
                    client, 'bootstrap', ('--upload-tools', '--constraints',
                                          'mem=2G'))

    def test_destroy_environment_non_sudo(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client.env, 'needs_sudo', lambda: False):
            with patch.object(client, 'juju') as mock:
                client.destroy_environment()
            mock.assert_called_with(
                'destroy-environment', ('foo', '--force', '-y'),
                False, check=False, include_e=False, timeout=600.0)

    def test_destroy_environment_sudo(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client.env, 'needs_sudo', lambda: True):
            with patch.object(client, 'juju') as mock:
                client.destroy_environment()
            mock.assert_called_with(
                'destroy-environment', ('foo', '--force', '-y'),
                True, check=False, include_e=False, timeout=600.0)

    def test_destroy_environment_no_force(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_environment(force=False)
            mock.assert_called_with(
                'destroy-environment', ('foo', '-y'),
                False, check=False, include_e=False, timeout=600.0)

    def test_destroy_environment_delete_jenv(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client, 'juju'):
            with temp_env({}) as juju_home:
                client.juju_home = juju_home
                jenv_path = get_jenv_path(juju_home, 'foo')
                os.makedirs(os.path.dirname(jenv_path))
                open(jenv_path, 'w')
                self.assertTrue(os.path.exists(jenv_path))
                client.destroy_environment(delete_jenv=True)
                self.assertFalse(os.path.exists(jenv_path))

    def test_get_juju_output(self):
        env = SimpleEnvironment('foo')
        asdf = lambda x, stderr, env: 'asdf'
        client = EnvJujuClient(env, None, None)
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = client.get_juju_output('bar')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo'),),
                         mock.call_args[0])

    def test_get_juju_output_accepts_varargs(self):
        env = SimpleEnvironment('foo')
        asdf = lambda x, stderr, env: 'asdf'
        client = EnvJujuClient(env, None, None)
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = client.get_juju_output('bar', 'baz', '--qux')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo', 'baz',
                           '--qux'),), mock.call_args[0])

    def test_get_juju_output_stderr(self):
        def raise_without_stderr(args, stderr, env):
            stderr.write('Hello!')
            raise subprocess.CalledProcessError('a', 'b')
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.check_output', raise_without_stderr):
                client.get_juju_output('bar')
        self.assertEqual(exc.exception.stderr, 'Hello!')

    def test_get_juju_output_accepts_timeout(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch('subprocess.check_output') as sco_mock:
            client.get_juju_output('bar', timeout=5)
        self.assertEqual(
            sco_mock.call_args[0][0],
            (sys.executable, get_timeout_path(), '5.00', '--', 'juju',
             '--show-log', 'bar', '-e', 'foo'))

    def test__shell_environ_cloudsigma(self):
        client = EnvJujuClient(
            SimpleEnvironment('baz', {'type': 'cloudsigma'}),
            '1.24-foobar', 'path')
        env = client._shell_environ()
        self.assertEqual(env.get(JUJU_DEV_FEATURE_FLAGS, ''), '')

    def test_juju_output_supplies_path(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, '/foobar/bar')
        with patch('subprocess.check_output') as sco_mock:
            client.get_juju_output('cmd', 'baz')
        self.assertRegexpMatches(sco_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')

    def test_get_status(self):
        output_text = yield dedent("""\
                - a
                - b
                - c
                """)
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=output_text):
            result = client.get_status()
        self.assertEqual(Status, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_get_status_retries_on_error(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        client.attempt = 0

        def get_juju_output(command, *args):
            if client.attempt == 1:
                return '"hello"'
            client.attempt += 1
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output', get_juju_output):
            client.get_status()

    def test_get_status_raises_on_timeout_1(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)

        def get_juju_output(command):
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output',
                          side_effect=get_juju_output):
            with patch('jujupy.until_timeout', lambda x: iter([None, None])):
                with self.assertRaisesRegexp(
                        Exception, 'Timed out waiting for juju status'):
                    client.get_status()

    def test_get_status_raises_on_timeout_2(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch('jujupy.until_timeout', return_value=iter([1])) as mock_ut:
            with patch.object(client, 'get_juju_output',
                              side_effect=StopIteration):
                with self.assertRaises(StopIteration):
                    client.get_status(500)
        mock_ut.assert_called_with(500)

    @staticmethod
    def make_status_yaml(key, machine_value, unit_value):
        return dedent("""\
            machines:
              "0":
                {0}: {1}
            services:
              jenkins:
                units:
                  jenkins/0:
                    {0}: {2}
        """.format(key, machine_value, unit_value))

    def test_deploy_non_joyent(self):
        env = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb')
        mock_juju.assert_called_with('deploy', ('mondogb',))

    def test_deploy_joyent(self):
        env = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb')
        mock_juju.assert_called_with('deploy', ('mondogb',))

    def test_deploy_repository(self):
        env = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb', '/home/jrandom/repo')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--repository', '/home/jrandom/repo'))

    def test_deploy_to(self):
        env = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb', to='0')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--to', '0'))

    def test_status_until_always_runs_once(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        with patch.object(client, 'get_juju_output', return_value=status_txt):
            result = list(client.status_until(-1))
        self.assertEqual(
            [r.status for r in result], [Status.from_text(status_txt).status])

    def test_status_until_timeout(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        status_yaml = yaml.safe_load(status_txt)

        def until_timeout_stub(timeout, start=None):
            return iter([None, None])

        with patch.object(client, 'get_juju_output', return_value=status_txt):
            with patch('jujupy.until_timeout',
                       side_effect=until_timeout_stub) as ut_mock:
                result = list(client.status_until(30, 70))
        self.assertEqual(
            [r.status for r in result], [status_yaml] * 3)
        # until_timeout is called by status as well as status_until.
        self.assertEqual(ut_mock.mock_calls,
                         [call(60), call(30, start=70), call(60), call(60)])

    def test_add_ssh_machines(self):
        client = EnvJujuClient(SimpleEnvironment('foo'), None, '')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-bar'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-baz'), 2)
        self.assertEqual(cc_mock.call_count, 3)

    def test_wait_for_started(self):
        value = self.make_status_yaml('agent-state', 'started', 'started')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_started()

    def test_wait_for_started_timeout(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch('jujupy.until_timeout', lambda x, start=None: range(1)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for agents to start in local'):
                    with patch('logging.error'):
                        client.wait_for_started()

    def test_wait_for_started_start(self):
        value = self.make_status_yaml('agent-state', 'started', 'pending')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for agents to start in local'):
                    with patch('logging.error'):
                        client.wait_for_started(start=now - timedelta(1200))

    def test_wait_for_started_logs_status(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            with self.assertRaisesRegexp(
                    Exception,
                    'Timed out waiting for agents to start in local'):
                with patch('logging.error') as le_mock:
                    client.wait_for_started(0)
        le_mock.assert_called_once_with(value)

    def test_wait_for_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1/0:
                        agent-state: started
              ubuntu:
                units:
                  ubuntu/0:
                    subordinates:
                      sub2/0:
                        agent-state: started
                      sub3/0:
                        agent-state: started
        """)
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch('jujupy.GroupReporter.update') as update_mock:
                    with patch('jujupy.GroupReporter.finish') as finish_mock:
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))
        expected_states = defaultdict(list)
        expected_states['started'].append('sub1/0')
        update_mock.assert_called_once_with(expected_states)
        finish_mock.assert_called_once_with()

    def test_wait_for_multiple_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              ubuntu:
                units:
                  ubuntu/0:
                    subordinates:
                      sub/0:
                        agent-state: started
                  ubuntu/1:
                    subordinates:
                      sub/1:
                        agent-state: started
        """)
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch('jujupy.GroupReporter.update') as update_mock:
                    with patch('jujupy.GroupReporter.finish') as finish_mock:
                        client.wait_for_subordinate_units(
                            'ubuntu', 'sub', start=now - timedelta(1200))
        expected_states = defaultdict(list)
        expected_states['started'].append('sub/0')
        expected_states['started'].append('sub/1')
        update_mock.assert_called_once_with(expected_states)
        finish_mock.assert_called_once_with()

    def test_wait_for_subordinate_units_checks_slash_in_unit_name(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1:
                        agent-state: started
        """)
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for agents to start in local'):
                    with patch('logging.error'):
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_subordinate_units_no_subordinate(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    agent-state: started
        """)
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for agents to start in local'):
                    with patch('logging.error'):
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_ha(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
                '2': {'state-server-member-status': 'has-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_ha()

    def test_wait_for_ha_no_has_vote(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'no-vote'},
                '1': {'state-server-member-status': 'no-vote'},
                '2': {'state-server-member-status': 'no-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch('sys.stdout'):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for voting to be enabled.'):
                    client.wait_for_ha(0.01)

    def test_wait_for_ha_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for voting to be enabled.'):
                    client.wait_for_ha()

    def test_wait_for_deploy_started(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        })
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_deploy_started()

    def test_wait_for_deploy_started_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'services': {},
        })
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for services to start.'):
                    client.wait_for_deploy_started()

    def test_wait_for_version(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_version('1.17.2')

    def test_wait_for_version_timeout(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.1')
        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        Exception, 'Some versions did not update'):
                    client.wait_for_version('1.17.2')

    def test_wait_for_version_handles_connection_error(self):
        err = subprocess.CalledProcessError(2, 'foo')
        err.stderr = 'Unable to connect to environment'
        err = CannotConnectEnv(err)
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        output_real = 'test_jujupy.EnvJujuClient.get_juju_output'
        devnull = open(os.devnull, 'w')
        with patch('sys.stdout', devnull):
            with patch(output_real, get_juju_output_fake):
                client.wait_for_version('1.17.2')

    def test_wait_for_version_raises_non_connection_error(self):
        err = Exception('foo')
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        client = EnvJujuClient(SimpleEnvironment('local'), None, None)
        output_real = 'test_jujupy.EnvJujuClient.get_juju_output'
        devnull = open(os.devnull, 'w')
        with patch('sys.stdout', devnull):
            with patch(output_real, get_juju_output_fake):
                with self.assertRaisesRegexp(Exception, 'foo'):
                    client.wait_for_version('1.17.2')

    def test_get_env_option(self):
        env = SimpleEnvironment('foo', None)
        client = EnvJujuClient(env, None, None)
        with patch('subprocess.check_output') as mock:
            mock.return_value = 'https://example.org/juju/tools'
            result = client.get_env_option('tools-metadata-url')
        self.assertEqual(
            mock.call_args[0][0],
            ('juju', '--show-log', 'get-env', '-e', 'foo',
             'tools-metadata-url'))
        self.assertEqual('https://example.org/juju/tools', result)

    def test_set_env_option(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, None)
        with patch('subprocess.check_call') as mock:
            client.set_env_option(
                'tools-metadata-url', 'https://example.org/juju/tools')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.juju_home
        mock.assert_called_with(
            ('juju', '--show-log', 'set-env', '-e', 'foo',
             'tools-metadata-url=https://example.org/juju/tools'),
            env=environ)

    def test_set_testing_tools_metadata_url(self):
        env = SimpleEnvironment(None, {'type': 'foo'})
        client = EnvJujuClient(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_tools_metadata_url()
        mock_get.assert_called_with('tools-metadata-url')
        mock_set.assert_called_with(
            'tools-metadata-url',
            'https://example.org/juju/testing/tools')

    def test_set_testing_tools_metadata_url_noop(self):
        env = SimpleEnvironment(None, {'type': 'foo'})
        client = EnvJujuClient(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/testing/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_tools_metadata_url()
        mock_get.assert_called_with('tools-metadata-url')
        self.assertEqual(0, mock_set.call_count)

    def test_juju(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, None)
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.check_call') as mock:
                client.juju('foo', ('bar', 'baz'))
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.juju_home
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), env=environ)
        stdout_mock.flush.assert_called_with()

    def test_juju_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_call') as cc_mock:
            client.juju('foo', ('bar', 'baz'))
        self.assertRegexpMatches(cc_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')

    def test_juju_no_check(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, None)
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.juju_home
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.call') as mock:
                client.juju('foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), env=environ)
        stdout_mock.flush.assert_called_with()

    def test_juju_no_check_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.call') as call_mock:
            client.juju('foo', ('bar', 'baz'), check=False)
        self.assertRegexpMatches(call_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')

    def test_juju_timeout(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_call') as cc_mock:
            client.juju('foo', ('bar', 'baz'), timeout=58)
        self.assertEqual(cc_mock.call_args[0][0], (
            sys.executable, get_timeout_path(), '58.00', '--', 'juju',
            '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))

    def test_juju_juju_home(self):
        env = SimpleEnvironment('qux')
        with scoped_environ():
            os.environ['JUJU_HOME'] = 'foo'
            client = EnvJujuClient(env, None, '/foobar/baz')
            with patch('subprocess.check_call') as cc_mock:
                client.juju('foo', ('bar', 'baz'))
                self.assertEqual(cc_mock.mock_calls[0][2]['env']['JUJU_HOME'],
                                 'foo')
                client.juju_home = 'asdf'
                client.juju('foo', ('bar', 'baz'))
                self.assertEqual(cc_mock.mock_calls[1][2]['env']['JUJU_HOME'],
                                 'asdf')

    def test_juju_extra_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, None)
        extra_env = {'JUJU': '/juju', 'JUJU_HOME': client.juju_home}
        with patch('sys.stdout'):
            with patch('subprocess.check_call') as mock:
                client.juju('quickstart', ('bar', 'baz'), extra_env=extra_env)
        env = dict(os.environ)
        env.update(extra_env)
        mock.assert_called_with(
            ('juju', '--show-log', 'quickstart', '-e', 'qux', 'bar', 'baz'),
            env=env)
        self.assertEqual('/juju', mock.call_args[1]['env']['JUJU'])

    def test_juju_backup_with_tgz(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value='foojuju-backup-24.tgzz') as co_mock:
            with patch('sys.stdout'):
                backup_file = client.backup()
        self.assertEqual(backup_file, os.path.abspath('juju-backup-24.tgz'))
        assert_juju_call(self, co_mock, client, ['juju', 'backup'])
        self.assertEqual(co_mock.mock_calls[0][2]['env']['JUJU_ENV'], 'qux')

    def test_juju_backup_with_tar_gz(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value='foojuju-backup-123-456.tar.gzbar'):
            with patch('sys.stdout'):
                backup_file = client.backup()
        self.assertEqual(
            backup_file, os.path.abspath('juju-backup-123-456.tar.gz'))

    def test_juju_backup_no_file(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_output', return_value=''):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                with patch('sys.stdout'):
                    client.backup()

    def test_juju_backup_wrong_file(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value='mumu-backup-24.tgz'):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                with patch('sys.stdout'):
                    client.backup()

    def test_juju_async(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with client.juju_async('foo', ('bar', 'baz')) as proc:
                assert_juju_call(self, popen_class_mock, client, (
                    'juju', '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))
                self.assertIs(proc, popen_class_mock.return_value)
                self.assertEqual(proc.wait.call_count, 0)
                proc.wait.return_value = 0
        proc.wait.assert_called_once_with()

    def test_juju_async_failure(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with self.assertRaises(subprocess.CalledProcessError) as err_cxt:
                with client.juju_async('foo', ('bar', 'baz')):
                    proc_mock = popen_class_mock.return_value
                    proc_mock.wait.return_value = 23
        self.assertEqual(err_cxt.exception.returncode, 23)
        self.assertEqual(err_cxt.exception.cmd, (
            'juju', '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))

    def test_is_jes_enabled(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value=' system') as co_mock:
            self.assertFalse(client.is_jes_enabled())
        assert_juju_call(self, co_mock, client, (
            'juju', '--show-log', 'help', 'commands'), assign_stderr=True)
        with patch('subprocess.check_output', autospec=True,
                   return_value='system') as co_mock:
            self.assertTrue(client.is_jes_enabled())

    def test_get_juju_timings(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient(env, None, 'my/juju/bin')
        client.juju_timings = {("juju", "op1"): [1], ("juju", "op2"): [2]}
        flattened_timings = client.get_juju_timings()
        expected = {"juju op1": [1], "juju op2": [2]}
        self.assertEqual(flattened_timings, expected)


class TestUniquifyLocal(TestCase):

    def test_uniquify_local_empty(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        uniquify_local(env)
        self.assertEqual(env.config, {
            'type': 'local',
            'api-port': 17071,
            'state-port': 37018,
            'storage-port': 8041,
            'syslog-port': 6515,
        })

    def test_uniquify_local_preset(self):
        env = SimpleEnvironment('foo', {
            'type': 'local',
            'api-port': 17071,
            'state-port': 37018,
            'storage-port': 8041,
            'syslog-port': 6515,
        })
        uniquify_local(env)
        self.assertEqual(env.config, {
            'type': 'local',
            'api-port': 17072,
            'state-port': 37019,
            'storage-port': 8042,
            'syslog-port': 6516,
        })

    def test_uniquify_nonlocal(self):
        env = SimpleEnvironment('foo', {
            'type': 'nonlocal',
            'api-port': 17071,
            'state-port': 37018,
            'storage-port': 8041,
            'syslog-port': 6515,
        })
        uniquify_local(env)
        self.assertEqual(env.config, {
            'type': 'nonlocal',
            'api-port': 17071,
            'state-port': 37018,
            'storage-port': 8041,
            'syslog-port': 6515,
        })


@contextmanager
def bootstrap_context(client=None):
    # Avoid unnecessary syscalls.
    with patch('jujupy.check_free_disk_space'):
        with scoped_environ():
            with temp_dir() as fake_home:
                os.environ['JUJU_HOME'] = fake_home
                yield fake_home


class TestJesHomePath(TestCase):

    def test_jes_home_path(self):
        path = jes_home_path('/home/jrandom/foo', 'bar')
        self.assertEqual(path, '/home/jrandom/foo/jes-homes/bar')


class TestGetCachePath(TestCase):

    def test_get_cache_path(self):
        path = get_cache_path('/home/jrandom/foo')
        self.assertEqual(path, '/home/jrandom/foo/environments/cache.yaml')


class TestMakeJESHome(TestCase):

    def test_make_jes_home(self):
        with temp_dir() as juju_home:
            with make_jes_home(juju_home, 'bar', {'baz': 'qux'}) as jes_home:
                pass
            with open(get_environments_path(jes_home)) as env_file:
                env = yaml.safe_load(env_file)
        self.assertEqual(env, {'baz': 'qux'})
        self.assertEqual(jes_home, jes_home_path(juju_home, 'bar'))

    def test_clean_existing(self):
        with temp_dir() as juju_home:
            with make_jes_home(juju_home, 'bar', {'baz': 'qux'}) as jes_home:
                foo_path = os.path.join(jes_home, 'foo')
                with open(foo_path, 'w') as foo:
                    foo.write('foo')
                self.assertTrue(os.path.isfile(foo_path))
            with make_jes_home(juju_home, 'bar', {'baz': 'qux'}) as jes_home:
                self.assertFalse(os.path.exists(foo_path))


def stub_bootstrap(client):
    jenv_path = get_jenv_path(client.juju_home, 'qux')
    os.mkdir(os.path.dirname(jenv_path))
    with open(jenv_path, 'w') as f:
        f.write('Bogus jenv')


class TestTempBootstrapEnv(TestCase):

    def test_no_config_mangling_side_effect(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            with temp_bootstrap_env(fake_home, client):
                stub_bootstrap(client)
        self.assertEqual(env.config, {'type': 'local'})

    def test_temp_bootstrap_env_environment(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        with bootstrap_context() as fake_home:
            client = EnvJujuClient.by_version(env)
            agent_version = client.get_matching_agent_version()
            with temp_bootstrap_env(fake_home, client):
                temp_home = os.environ['JUJU_HOME']
                self.assertNotEqual(temp_home, fake_home)
                symlink_path = get_jenv_path(fake_home, 'qux')
                symlink_target = os.path.realpath(symlink_path)
                expected_target = os.path.realpath(
                    get_jenv_path(temp_home, 'qux'))
                self.assertEqual(symlink_target, expected_target)
                config = yaml.safe_load(
                    open(get_environments_path(temp_home)))
                self.assertEqual(config, {'environments': {'qux': {
                    'type': 'local',
                    'root-dir': get_local_root(fake_home, client.env),
                    'agent-version': agent_version,
                    'test-mode': True,
                    'name': 'qux',
                }}})
                stub_bootstrap(client)

    def test_temp_bootstrap_env_provides_dir(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with temp_dir() as fake_home:
            juju_home = os.path.join(fake_home, 'asdf')

            def side_effect(*args, **kwargs):
                os.mkdir(juju_home)
                return juju_home

            with patch('utility.mkdtemp', side_effect=side_effect):
                with temp_bootstrap_env(fake_home, client) as temp_home:
                    pass
        self.assertEqual(temp_home, juju_home)

    def test_temp_bootstrap_env_no_set_home(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with temp_dir() as fake_home:
            with scoped_environ():
                os.environ['JUJU_HOME'] = 'foo'
                with temp_bootstrap_env(fake_home, client, set_home=False):
                    self.assertEqual(os.environ['JUJU_HOME'], 'foo')

    def test_output(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            with temp_bootstrap_env(fake_home, client):
                stub_bootstrap(client)
            jenv_path = get_jenv_path(fake_home, 'qux')
            self.assertFalse(os.path.islink(jenv_path))
            self.assertEqual(open(jenv_path).read(), 'Bogus jenv')

    def test_rename_on_exception(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            with self.assertRaisesRegexp(Exception, 'test-rename'):
                with temp_bootstrap_env(fake_home, client):
                    stub_bootstrap(client)
                    raise Exception('test-rename')
            jenv_path = get_jenv_path(os.environ['JUJU_HOME'], 'qux')
            self.assertFalse(os.path.islink(jenv_path))
            self.assertEqual(open(jenv_path).read(), 'Bogus jenv')

    def test_exception_no_jenv(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            with self.assertRaisesRegexp(Exception, 'test-rename'):
                with temp_bootstrap_env(fake_home, client):
                    jenv_path = get_jenv_path(os.environ['JUJU_HOME'], 'qux')
                    os.mkdir(os.path.dirname(jenv_path))
                    raise Exception('test-rename')
            jenv_path = get_jenv_path(os.environ['JUJU_HOME'], 'qux')
            self.assertFalse(os.path.lexists(jenv_path))

    def test_check_space_local_lxc(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        with bootstrap_context() as fake_home:
            client = EnvJujuClient.by_version(env)
            with patch('jujupy.check_free_disk_space') as mock_cfds:
                with temp_bootstrap_env(fake_home, client):
                    stub_bootstrap(client)
        self.assertEqual(mock_cfds.mock_calls, [
            call(os.path.join(fake_home, 'qux'), 8000000, 'MongoDB files'),
            call('/var/lib/lxc', 2000000, 'LXC containers'),
        ])

    def test_check_space_local_kvm(self):
        env = SimpleEnvironment('qux', {'type': 'local', 'container': 'kvm'})
        with bootstrap_context() as fake_home:
            client = EnvJujuClient.by_version(env)
            with patch('jujupy.check_free_disk_space') as mock_cfds:
                with temp_bootstrap_env(fake_home, client):
                    stub_bootstrap(client)
        self.assertEqual(mock_cfds.mock_calls, [
            call(os.path.join(fake_home, 'qux'), 8000000, 'MongoDB files'),
            call('/var/lib/uvtool/libvirt/images', 2000000, 'KVM disk files'),
        ])

    def test_error_on_jenv(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            jenv_path = get_jenv_path(fake_home, 'qux')
            os.mkdir(os.path.dirname(jenv_path))
            with open(jenv_path, 'w') as f:
                f.write('In the way')
            with self.assertRaisesRegexp(Exception, '.* already exists!'):
                with temp_bootstrap_env(fake_home, client):
                    stub_bootstrap(client)

    def test_not_permanent(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            client.juju_home = fake_home
            with temp_bootstrap_env(fake_home, client,
                                    permanent=False) as tb_home:
                stub_bootstrap(client)
            self.assertFalse(os.path.exists(tb_home))
            self.assertTrue(os.path.exists(get_jenv_path(fake_home,
                            client.env.environment)))
            self.assertFalse(os.path.exists(get_jenv_path(tb_home,
                             client.env.environment)))
        self.assertFalse(os.path.exists(tb_home))
        self.assertEqual(client.juju_home, fake_home)
        self.assertNotEqual(tb_home,
                            jes_home_path(fake_home, client.env.environment))

    def test_permanent(self):
        env = SimpleEnvironment('qux', {'type': 'local'})
        client = EnvJujuClient.by_version(env)
        with bootstrap_context(client) as fake_home:
            client.juju_home = fake_home
            with temp_bootstrap_env(fake_home, client,
                                    permanent=True) as tb_home:
                stub_bootstrap(client)
            self.assertTrue(os.path.exists(tb_home))
            self.assertFalse(os.path.exists(get_jenv_path(fake_home,
                             client.env.environment)))
            self.assertTrue(os.path.exists(get_jenv_path(tb_home,
                            client.env.environment)))
        self.assertFalse(os.path.exists(tb_home))
        self.assertEqual(client.juju_home, tb_home)


class TestJujuClientDevel(TestCase):

    def test_get_version(self):
        value = ' 5.6 \n'
        with patch('subprocess.check_output', return_value=value) as vsn:
            version = JujuClientDevel.get_version()
        self.assertEqual('5.6', version)
        vsn.assert_called_with(('juju', '--version'))

    def test_by_version(self):
        def juju_cmd_iterator():
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'

        context = patch.object(
            JujuClientDevel, 'get_version',
            side_effect=juju_cmd_iterator().next)
        with context:
            self.assertIs(JujuClientDevel,
                          type(JujuClientDevel.by_version()))
            with self.assertRaisesRegexp(Exception, 'Unsupported juju: 1.16'):
                JujuClientDevel.by_version()
            with self.assertRaisesRegexp(Exception,
                                         'Unsupported juju: 1.16.1'):
                JujuClientDevel.by_version()
            client = JujuClientDevel.by_version()
            self.assertIs(JujuClientDevel, type(client))
            self.assertEqual('1.15', client.version)

    def test_bootstrap_hpcloud(self):
        env = SimpleEnvironment('hp')
        with patch.object(env, 'hpcloud', lambda: True):
            with patch.object(EnvJujuClient, 'juju') as mock:
                JujuClientDevel(None, None).bootstrap(env)
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_non_sudo(self):
        env = SimpleEnvironment('foo')
        with patch.object(SimpleEnvironment, 'needs_sudo', return_value=False):
            with patch.object(EnvJujuClient, 'juju') as mock:
                JujuClientDevel(None, None).bootstrap(env)
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_sudo(self):
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, None)
        with patch.object(SimpleEnvironment, 'needs_sudo', return_value=True):
            with patch.object(EnvJujuClient, 'juju') as mock:
                client.bootstrap(env)
            mock.assert_called_with(
                'bootstrap', ('--constraints', 'mem=2G'), True)

    def test_destroy_environment_non_sudo(self):
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, None)
        with patch.object(SimpleEnvironment, 'needs_sudo', return_value=False):
            with patch.object(EnvJujuClient, 'juju') as mock:
                client.destroy_environment(env)
            mock.assert_called_with(
                'destroy-environment', ('foo', '--force', '-y'),
                False, check=False, include_e=False, timeout=600)

    def test_destroy_environment_sudo(self):
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, None)
        with patch.object(SimpleEnvironment, 'needs_sudo', return_value=True):
            with patch.object(EnvJujuClient, 'juju') as mock:
                client.destroy_environment(env)
            mock.assert_called_with(
                'destroy-environment', ('foo', '--force', '-y'),
                True, check=False, include_e=False, timeout=600.0)

    def test_get_juju_output(self):
        env = SimpleEnvironment('foo')
        asdf = lambda x, stderr, env: 'asdf'
        client = JujuClientDevel(None, None)
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = client.get_juju_output(env, 'bar')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo'),),
                         mock.call_args[0])

    def test_get_juju_output_accepts_varargs(self):
        env = SimpleEnvironment('foo')
        asdf = lambda x, stderr, env: 'asdf'
        client = JujuClientDevel(None, None)
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = client.get_juju_output(env, 'bar', 'baz', '--qux')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo', 'baz',
                           '--qux'),), mock.call_args[0])

    def test_get_juju_output_stderr(self):
        def raise_without_stderr(args, stderr, env):
            stderr.write('Hello!')
            raise subprocess.CalledProcessError('a', 'b')
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, None)
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.check_output', raise_without_stderr):
                client.get_juju_output(env, 'bar')
        self.assertEqual(exc.exception.stderr, 'Hello!')

    def test_get_juju_output_accepts_timeout(self):
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, None)
        with patch('subprocess.check_output') as sco_mock:
            client.get_juju_output(env, 'bar', timeout=5)
        self.assertEqual(
            sco_mock.call_args[0][0],
            (sys.executable, get_timeout_path(), '5.00', '--', 'juju',
             '--show-log', 'bar', '-e', 'foo'))

    def test_juju_output_supplies_path(self):
        env = SimpleEnvironment('foo')
        client = JujuClientDevel(None, '/foobar/bar')
        with patch('subprocess.check_output') as sco_mock:
            client.get_juju_output(env, 'baz')
        self.assertRegexpMatches(sco_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')

    def test_get_status(self):
        output_text = yield dedent("""\
                - a
                - b
                - c
                """)
        client = JujuClientDevel(None, None)
        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'get_juju_output',
                          return_value=output_text):
            result = client.get_status(env)
        self.assertEqual(Status, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_get_status_retries_on_error(self):
        client = JujuClientDevel(None, None)
        client.attempt = 0

        def get_juju_output(command, *args):
            if client.attempt == 1:
                return '"hello"'
            client.attempt += 1
            raise subprocess.CalledProcessError(1, command)

        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'get_juju_output', get_juju_output):
            client.get_status(env)

    def test_get_status_raises_on_timeout_1(self):
        client = JujuClientDevel(None, None)

        def get_juju_output(command):
            raise subprocess.CalledProcessError(1, command)

        env = SimpleEnvironment('foo')
        with patch.object(EnvJujuClient, 'get_juju_output',
                          side_effect=get_juju_output):
            with patch('jujupy.until_timeout', lambda x: iter([None, None])):
                with self.assertRaisesRegexp(
                        Exception, 'Timed out waiting for juju status'):
                    client.get_status(env)

    def test_get_status_raises_on_timeout_2(self):
        client = JujuClientDevel(None, None)
        env = SimpleEnvironment('foo')
        with patch('jujupy.until_timeout', return_value=iter([1])) as mock_ut:
            with patch.object(EnvJujuClient, 'get_juju_output',
                              side_effect=StopIteration):
                with self.assertRaises(StopIteration):
                    client.get_status(env, 500)
        mock_ut.assert_called_with(500)

    def test_get_env_option(self):
        client = JujuClientDevel(None, None)
        env = SimpleEnvironment('foo')
        with patch('subprocess.check_output') as mock:
            mock.return_value = 'https://example.org/juju/tools'
            result = client.get_env_option(env, 'tools-metadata-url')
        self.assertEqual(
            mock.call_args[0][0],
            ('juju', '--show-log', 'get-env', '-e', 'foo',
             'tools-metadata-url'))
        self.assertEqual('https://example.org/juju/tools', result)

    def test_set_env_option(self):
        client = JujuClientDevel(None, None)
        env = SimpleEnvironment('foo')
        with patch('subprocess.check_call') as mock:
            client.set_env_option(
                env, 'tools-metadata-url', 'https://example.org/juju/tools')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = get_juju_home()
        mock.assert_called_with(
            ('juju', '--show-log', 'set-env', '-e', 'foo',
             'tools-metadata-url=https://example.org/juju/tools'),
            env=environ)

    def test_juju(self):
        env = SimpleEnvironment('qux')
        client = JujuClientDevel(None, None)
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.check_call') as mock:
                client.juju(env, 'foo', ('bar', 'baz'))
        environ = dict(os.environ)
        environ['JUJU_HOME'] = get_juju_home()
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), env=environ)
        stdout_mock.flush.assert_called_with()

    def test_juju_env(self):
        env = SimpleEnvironment('qux')
        client = JujuClientDevel(None, '/foobar/baz')
        with patch('subprocess.check_call') as cc_mock:
            client.juju(env, 'foo', ('bar', 'baz'))
        self.assertRegexpMatches(cc_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')

    def test_juju_no_check(self):
        env = SimpleEnvironment('qux')
        client = JujuClientDevel(None, None)
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.call') as mock:
                client.juju(env, 'foo', ('bar', 'baz'), check=False)
        environ = dict(os.environ)
        environ['JUJU_HOME'] = get_juju_home()
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), env=environ)
        stdout_mock.flush.assert_called_with()

    def test_juju_no_check_env(self):
        env = SimpleEnvironment('qux')
        client = JujuClientDevel(None, '/foobar/baz')
        with patch('subprocess.call') as call_mock:
            client.juju(env, 'foo', ('bar', 'baz'), check=False)
        self.assertRegexpMatches(call_mock.call_args[1]['env']['PATH'],
                                 r'/foobar\:')


class TestStatus(TestCase):

    def test_iter_machines_no_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {'1/lxc/0': {'baz': 'qux'}}}
            },
            'services': {}}, '')
        self.assertEqual(list(status.iter_machines()),
                         [('1', status.status['machines']['1'])])

    def test_iter_machines_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {'1/lxc/0': {'baz': 'qux'}}}
            },
            'services': {}}, '')
        self.assertEqual(list(status.iter_machines(containers=True)), [
            ('1', status.status['machines']['1']),
            ('1/lxc/0', {'baz': 'qux'}),
        ])

    def test_agent_items_empty(self):
        status = Status({'machines': {}, 'services': {}}, '')
        self.assertItemsEqual([], status.agent_items())

    def test_agent_items(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar'}
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'subordinates': {
                                'sub': {'baz': 'qux'}
                            }
                        }
                    }
                }
            }
        }, '')
        expected = [
            ('1', {'foo': 'bar'}),
            ('jenkins/1', {'subordinates': {'sub': {'baz': 'qux'}}}),
            ('sub', {'baz': 'qux'})]
        self.assertItemsEqual(expected, status.agent_items())

    def test_agent_items_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {
                    '2': {'qux': 'baz'},
                }}
            },
            'services': {}
        }, '')
        expected = [
            ('1', {'foo': 'bar', 'containers': {'2': {'qux': 'baz'}}}),
            ('2', {'qux': 'baz'})
        ]
        self.assertItemsEqual(expected, status.agent_items())

    def test_get_service_count_zero(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
        }, '')
        self.assertEqual(0, status.get_service_count())

    def test_get_service_count(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'dummy-sink/0': {'agent-state': 'started'},
                    }
                },
                'juju-reports': {
                    'units': {
                        'juju-reports/0': {'agent-state': 'pending'},
                    }
                }
            }
        }, '')
        self.assertEqual(3, status.get_service_count())

    def test_get_service_unit_count_zero(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
        }, '')
        self.assertEqual(0, status.get_service_unit_count('jenkins'))

    def test_get_service_unit_count(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'bad'},
                        'jenkins/3': {'agent-state': 'bad'},
                    }
                }
            }
        }, '')
        self.assertEqual(3, status.get_service_unit_count('jenkins'))

    def test_get_unit(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {'agent-state': 'started'},
                    }
                },
            }
        }, '')
        self.assertEqual(
            status.get_unit('jenkins/1'), {'agent-state': 'bad'})
        self.assertEqual(
            status.get_unit('jenkins/2'), {'agent-state': 'started'})
        with self.assertRaisesRegexp(KeyError, 'jenkins/3'):
            status.get_unit('jenkins/3')

    def test_service_subordinate_units(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'services': {
                'ubuntu': {},
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'subordinates': {
                                'chaos-monkey/0': {'agent-state': 'started'},
                            }
                        }
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {
                            'subordinates': {
                                'chaos-monkey/1': {'agent-state': 'started'}
                            }
                        },
                        'jenkins/3': {
                            'subordinates': {
                                'chaos-monkey/2': {'agent-state': 'started'}
                            }
                        }
                    }
                }
            }
        }, '')
        self.assertItemsEqual(
            status.service_subordinate_units('ubuntu'),
            [])
        self.assertItemsEqual(
            status.service_subordinate_units('jenkins'),
            [('chaos-monkey/0', {'agent-state': 'started'},)])
        self.assertItemsEqual(
            status.service_subordinate_units('dummy-sink'), [
                ('chaos-monkey/1', {'agent-state': 'started'}),
                ('chaos-monkey/2', {'agent-state': 'started'})]
            )

    def test_get_service_config(self):
        def output(*args, **kwargs):
            return yaml.safe_dump({
                'charm': 'foo',
                'service': 'foo',
                'settings': {
                    'dir': {
                        'default': 'true',
                        'description': 'bla bla',
                        'type': 'string',
                        'value': '/tmp/charm-dir',
                    }
                }
            })
        expected = yaml.safe_load(output())
        client = EnvJujuClient(SimpleEnvironment('bar', {}), None, '/foo')
        with patch.object(client, 'get_juju_output', side_effect=output):
            results = client.get_service_config('foo')
        self.assertEqual(expected, results)

    def test_get_service_config_timesout(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        with patch('jujupy.until_timeout', return_value=range(0)):
            with self.assertRaisesRegexp(
                    Exception, 'Timed out waiting for juju get'):
                client.get_service_config('foo')

    def test_get_open_ports(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {'open-ports': ['42/tcp']},
                    }
                },
            }
        }, '')
        self.assertEqual(status.get_open_ports('jenkins/1'), [])
        self.assertEqual(status.get_open_ports('jenkins/2'), ['42/tcp'])

    def test_agent_states(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        }, '')
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_check_agents_started_not_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        }, '')
        self.assertEqual(status.agent_states(),
                         status.check_agents_started('env1'))

    def test_check_agents_started_all_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'agent-state': 'started',
                            'subordinates': {
                                'sub1': {
                                    'agent-state': 'started'
                                }
                            }
                        },
                        'jenkins/2': {'agent-state': 'started'},
                    }
                }
            }
        }, '')
        self.assertIs(None, status.check_agents_started('env1'))

    def test_check_agents_started_agent_error(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'any-error'},
            },
            'services': {}
        }, '')
        with self.assertRaisesRegexp(ErroredUnit,
                                     '1 is in state any-error'):
            status.check_agents_started('env1')

    def do_check_agents_started_failure(self, failure):
        status = Status({
            'machines': {'0': {
                'agent-state-info': failure}},
            'services': {},
        }, '')
        with self.assertRaises(ErroredUnit) as e_cxt:
            status.check_agents_started()
        e = e_cxt.exception
        self.assertEqual(
            str(e), '0 is in state {}'.format(failure))
        self.assertEqual(e.unit_name, '0')
        self.assertEqual(e.state, failure)

    def test_check_agents_cannot_set_up_groups(self):
        self.do_check_agents_started_failure('cannot set up groups foobar')

    def test_check_agents_error(self):
        self.do_check_agents_started_failure('error executing "lxc-start"')

    def test_check_agents_cannot_run_instances(self):
        self.do_check_agents_started_failure('cannot run instances')

    def test_check_agents_cannot_run_instance(self):
        self.do_check_agents_started_failure('cannot run instance')

    def test_check_agents_started_agent_info_error(self):
        # Sometimes the error is indicated in a special 'agent-state-info'
        # field.
        status = Status({
            'machines': {
                '1': {'agent-state-info': 'any-error'},
            },
            'services': {}
        }, '')
        with self.assertRaisesRegexp(ErroredUnit,
                                     '1 is in state any-error'):
            status.check_agents_started('env1')

    def test_get_agent_versions(self):
        status = Status({
            'machines': {
                '1': {'agent-version': '1.6.2'},
                '2': {'agent-version': '1.6.1'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/0': {
                            'agent-version': '1.6.1'},
                        'jenkins/1': {},
                    },
                }
            }
        }, '')
        self.assertEqual({
            '1.6.2': {'1'},
            '1.6.1': {'jenkins/0', '2'},
            'unknown': {'jenkins/1'},
        }, status.get_agent_versions())

    def test_iter_new_machines(self):
        old_status = Status({
            'machines': {
                'bar': 'bar_info',
            }
        }, '')
        new_status = Status({
            'machines': {
                'foo': 'foo_info',
                'bar': 'bar_info',
            }
        }, '')
        self.assertItemsEqual(new_status.iter_new_machines(old_status),
                              [('foo', 'foo_info')])

    def test_get_instance_id(self):
        status = Status({
            'machines': {
                '0': {'instance-id': 'foo-bar'},
                '1': {},
            }
        }, '')
        self.assertEqual(status.get_instance_id('0'), 'foo-bar')
        with self.assertRaises(KeyError):
            status.get_instance_id('1')
        with self.assertRaises(KeyError):
            status.get_instance_id('2')

    def test_from_text(self):
        text = TestEnvJujuClient.make_status_yaml(
            'agent-state', 'pending', 'horsefeathers')
        status = Status.from_text(text)
        self.assertEqual(status.status_text, text)
        self.assertEqual(status.status, {
            'machines': {'0': {'agent-state': 'pending'}},
            'services': {'jenkins': {'units': {'jenkins/0': {
                'agent-state': 'horsefeathers'}}}}
        })


def fast_timeout(count):
    if False:
        yield


@contextmanager
def temp_config():
    home = tempfile.mkdtemp()
    try:
        environments_path = os.path.join(home, 'environments.yaml')
        old_home = os.environ.get('JUJU_HOME')
        os.environ['JUJU_HOME'] = home
        try:
            with open(environments_path, 'w') as environments:
                yaml.dump({'environments': {
                    'foo': {'type': 'local'}
                }}, environments)
            yield
        finally:
            if old_home is None:
                del os.environ['JUJU_HOME']
            else:
                os.environ['JUJU_HOME'] = old_home
    finally:
        shutil.rmtree(home)


class TestSimpleEnvironment(TestCase):

    def test_local_from_config(self):
        env = SimpleEnvironment('local', {'type': 'openstack'})
        self.assertFalse(env.local, 'Does not respect config type.')
        env = SimpleEnvironment('local', {'type': 'local'})
        self.assertTrue(env.local, 'Does not respect config type.')

    def test_kvm_from_config(self):
        env = SimpleEnvironment('local', {'type': 'local'})
        self.assertFalse(env.kvm, 'Does not respect config type.')
        env = SimpleEnvironment('local',
                                {'type': 'local', 'container': 'kvm'})
        self.assertTrue(env.kvm, 'Does not respect config type.')

    def test_hpcloud_from_config(self):
        env = SimpleEnvironment('cloud', {'auth-url': 'before.keystone.after'})
        self.assertFalse(env.hpcloud, 'Does not respect config type.')
        env = SimpleEnvironment('hp', {'auth-url': 'before.hpcloudsvc.after/'})
        self.assertTrue(env.hpcloud, 'Does not respect config type.')

    def test_from_config(self):
        with temp_config():
            env = SimpleEnvironment.from_config('foo')
            self.assertIs(SimpleEnvironment, type(env))
            self.assertEqual({'type': 'local'}, env.config)

    def test_from_bogus_config(self):
        with temp_config():
            with self.assertRaises(NoSuchEnvironment):
                SimpleEnvironment.from_config('bar')


class TestEnvironment(TestCase):

    @staticmethod
    def make_status_yaml(key, machine_value, unit_value):
        return dedent("""\
            machines:
              "0":
                {0}: {1}
            services:
              jenkins:
                units:
                  jenkins/0:
                    {0}: {2}
        """.format(key, machine_value, unit_value))

    def test_wait_for_started(self):
        value = self.make_status_yaml('agent-state', 'started', 'started')
        env = Environment('local', JujuClientDevel(None, None))
        with patch.object(EnvJujuClient, 'get_juju_output',
                          return_value=value):
            env.wait_for_started()

    def test_wait_for_started_timeout(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        env = Environment('local', JujuClientDevel(None, None))
        with patch('jujupy.until_timeout', lambda x, start=None: range(1)):
            with patch.object(EnvJujuClient, 'get_juju_output',
                              return_value=value):
                with self.assertRaisesRegexp(
                        Exception,
                        'Timed out waiting for agents to start in local'):
                    with patch('logging.error'):
                        env.wait_for_started()

    def test_wait_for_version(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        env = Environment('local', JujuClientDevel(None, None))
        with patch.object(
                EnvJujuClient, 'get_juju_output', return_value=value):
            env.wait_for_version('1.17.2')

    def test_wait_for_version_timeout(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.1')
        env = Environment('local', JujuClientDevel(None, None))
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with patch.object(EnvJujuClient, 'get_juju_output',
                              return_value=value):
                with self.assertRaisesRegexp(
                        Exception, 'Some versions did not update'):
                    env.wait_for_version('1.17.2')

    def test_wait_for_version_handles_connection_error(self):
        err = subprocess.CalledProcessError(2, 'foo')
        err.stderr = 'Unable to connect to environment'
        err = CannotConnectEnv(err)
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        env = Environment('local', JujuClientDevel(None, None))
        output_real = 'test_jujupy.EnvJujuClient.get_juju_output'
        devnull = open(os.devnull, 'w')
        with patch('sys.stdout', devnull):
            with patch(output_real, get_juju_output_fake):
                env.wait_for_version('1.17.2')

    def test_wait_for_version_raises_non_connection_error(self):
        err = Exception('foo')
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        env = Environment('local', JujuClientDevel(None, None))
        output_real = 'test_jujupy.EnvJujuClient.get_juju_output'
        devnull = open(os.devnull, 'w')
        with patch('sys.stdout', devnull):
            with patch(output_real, get_juju_output_fake):
                with self.assertRaisesRegexp(Exception, 'foo'):
                    env.wait_for_version('1.17.2')

    def test_from_config(self):
        with temp_config():
            env = Environment.from_config('foo')
            self.assertIs(Environment, type(env))
            self.assertEqual({'type': 'local'}, env.config)

    def test_upgrade_juju_nonlocal(self):
        client = JujuClientDevel('1.234-76', None)
        env = Environment('foo', client, {'type': 'nonlocal'})
        env_client = client.get_env_client(env)
        with patch.object(client, 'get_env_client', return_value=env_client):
            with patch.object(env_client, 'juju') as juju_mock:
                env.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234'))

    def test_get_matching_agent_version(self):
        client = JujuClientDevel('1.23-series-arch', None)
        env = Environment('foo', client, {'type': 'local'})
        self.assertEqual('1.23.1', env.get_matching_agent_version())
        self.assertEqual('1.23', env.get_matching_agent_version(
                         no_build=True))
        env.client.version = '1.20-beta1-series-arch'
        self.assertEqual('1.20-beta1.1', env.get_matching_agent_version())

    def test_upgrade_juju_local(self):
        client = JujuClientDevel(None, None)
        env = Environment('foo', client, {'type': 'local'})
        env.client.version = '1.234-76'
        env_client = client.get_env_client(env)
        with patch.object(client, 'get_env_client', return_value=env_client):
            with patch.object(env_client, 'juju') as juju_mock:
                env.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234', '--upload-tools',))

    def test_deploy_non_joyent(self):
        env = Environment('foo', MagicMock(), {'type': 'local'})
        env.client.version = '1.234-76'
        env.deploy('mondogb')
        env.client.juju.assert_called_with(env, 'deploy', ('mondogb',))

    def test_deploy_joyent(self):
        env = Environment('foo', MagicMock(), {'type': 'joyent'})
        env.client.version = '1.234-76'
        env.deploy('mondogb')
        env.client.juju.assert_called_with(
            env, 'deploy', ('mondogb',))

    def test_deployer(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'deployer', ('--debug', '--deploy-delay', '10', '--config',
                         'bundle:~juju-qa/some-bundle'), True
        )

    def test_deployer_with_bundle_name(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle', 'name')
        mock.assert_called_with(
            'deployer', ('--debug', '--deploy-delay', '10', '--config',
                         'bundle:~juju-qa/some-bundle', 'name'), True
        )

    def test_quickstart_maas(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'maas'}),
                               '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart',
            ('--constraints', 'mem=2G arch=amd64', '--no-browser',
             'bundle:~juju-qa/some-bundle'), False, extra_env={'JUJU': '/juju'}
        )

    def test_quickstart_local(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart',
            ('--constraints', 'mem=2G', '--no-browser',
             'bundle:~juju-qa/some-bundle'), True, extra_env={'JUJU': '/juju'}
        )

    def test_quickstart_nonlocal(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'nonlocal'}),
                               '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart',
            ('--constraints', 'mem=2G', '--no-browser',
             'bundle:~juju-qa/some-bundle'), False, extra_env={'JUJU': '/juju'}
        )

    def test_set_testing_tools_metadata_url(self):
        client = JujuClientDevel(None, None)
        env = Environment('foo', client)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                env.set_testing_tools_metadata_url()
        mock_get.assert_called_with(env, 'tools-metadata-url')
        mock_set.assert_called_with(
            env, 'tools-metadata-url',
            'https://example.org/juju/testing/tools')

    def test_set_testing_tools_metadata_url_noop(self):
        client = JujuClientDevel(None, None)
        env = Environment('foo', client)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/testing/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                env.set_testing_tools_metadata_url()
        mock_get.assert_called_with(env, 'tools-metadata-url')
        self.assertEqual(0, mock_set.call_count)

    def test_action_do(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'get_juju_output') as mock:
            mock.return_value = \
                "Action queued with id: 5a92ec93-d4be-4399-82dc-7431dbfd08f9"
            id = client.action_do("foo/0", "myaction", "param=5")
            self.assertEqual(id, "5a92ec93-d4be-4399-82dc-7431dbfd08f9")
        mock.assert_called_once_with(
            'action do', 'foo/0', 'myaction', "param=5"
        )

    def test_action_do_error(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'get_juju_output') as mock:
            mock.return_value = "some bad text"
            with self.assertRaisesRegexp(Exception,
                                         "Action id not found in output"):
                client.action_do("foo/0", "myaction", "param=5")

    def test_action_fetch(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            mock.return_value = ret
            out = client.action_fetch("123")
            self.assertEqual(out, ret)
        mock.assert_called_once_with(
            'action fetch', '123', "--wait", "1m"
        )

    def test_action_fetch_timeout(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        ret = "status: pending\nfoo: bar"
        with patch.object(EnvJujuClient,
                          'get_juju_output', return_value=ret):
            with self.assertRaisesRegexp(Exception,
                                         "timed out waiting for action"):
                client.action_fetch("123")

    def test_action_do_fetch(self):
        client = EnvJujuClient(SimpleEnvironment(None, {'type': 'local'}),
                               '1.23-series-arch', None)
        with patch.object(EnvJujuClient, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            # setting side_effect to an iterable will return the next value
            # from the list each time the function is called.
            mock.side_effect = [
                "Action queued with id: 5a92ec93-d4be-4399-82dc-7431dbfd08f9",
                ret]
            out = client.action_do_fetch("foo/0", "myaction", "param=5")
            self.assertEqual(out, ret)


class TestGroupReporter(TestCase):

    def test_single(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.update({"done": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1\n")

    def test_single_ticks(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 .")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 ..")
        reporter.update({"done": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 ..\n")

    def test_multiple_values(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1", "2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2")
        reporter.update({"working": ["1"], "done": ["2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2\nworking: 1")
        reporter.update({"done": ["1", "2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2\nworking: 1\n")

    def test_multiple_groups(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1", "2"], "starting": ["3"]})
        first = "starting: 3 | working: 1, 2"
        self.assertEqual(sio.getvalue(), first)
        reporter.update({"working": ["1", "3"], "done": ["2"]})
        second = "working: 1, 3"
        self.assertEqual(sio.getvalue(), "\n".join([first, second]))
        reporter.update({"done": ["1", "2", "3"]})
        self.assertEqual(sio.getvalue(), "\n".join([first, second, ""]))

    def test_finish(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.finish()
        self.assertEqual(sio.getvalue(), "working: 1\n")

    def test_finish_unchanged(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.finish()
        self.assertEqual(sio.getvalue(), "")

    def test_wrap_to_width(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        for _ in range(150):
            reporter.update({"working": ["1"]})
        reporter.finish()
        self.assertEqual(sio.getvalue(), """\
working: 1 ....................................................................
...............................................................................
..
""")

    def test_wrap_to_width_exact(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 12
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(20):
            reporter.update({"working": ["1"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1",
            "working: 1 .\n...",
            "working: 1 .\n.......",
            "working: 1 .\n...........",
            "working: 1 .\n............\n...",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")

    def test_wrap_to_width_overflow(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 8
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(16):
            reporter.update({"working": ["1"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1",
            "working: 1\n....",
            "working: 1\n........",
            "working: 1\n........\n....",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")

    def test_wrap_to_width_multiple_groups(self):
        sio = StringIO.StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 16
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(6):
            reporter.update({"working": ["1", "2"]})
            changes.append(sio.getvalue())
        for _ in range(10):
            reporter.update({"working": ["1"], "done": ["2"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1, 2",
            "working: 1, 2 ..\n..",
            "working: 1, 2 ..\n...\n"
            "working: 1 ..",
            "working: 1, 2 ..\n...\n"
            "working: 1 .....\n.",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")


class TestMakeClient(TestCase):

    @contextmanager
    def make_client_cxt(self):
        td = temp_dir()
        te = temp_env({'environments': {'foo': {
            'orig-name': 'foo', 'name': 'foo'}}})
        with td as juju_path, te, patch('subprocess.Popen',
                                        side_effect=ValueError):
            with patch('subprocess.check_output') as co_mock:
                co_mock.return_value = '1.18'
                yield juju_path

    def test_make_client(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, False, 'foo', 'bar')
        self.assertEqual(client.full_path, os.path.join(juju_path, 'juju'))
        self.assertEqual(client.debug, False)
        self.assertEqual(client.env.config['orig-name'], 'foo')
        self.assertEqual(client.env.config['name'], 'bar')
        self.assertEqual(client.env.environment, 'bar')

    def test_make_client_debug(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, True, 'foo', 'bar')
        self.assertEqual(client.debug, True)

    def test_make_client_no_temp_env_name(self):
        with self.make_client_cxt() as juju_path:
            client = make_client(juju_path, False, 'foo', None)
        self.assertEqual(client.full_path, os.path.join(juju_path, 'juju'))
        self.assertEqual(client.env.config['orig-name'], 'foo')
        self.assertEqual(client.env.config['name'], 'foo')
        self.assertEqual(client.env.environment, 'foo')


class AssessParseStateServerFromErrorTestCase(TestCase):

    def test_parse_new_state_server_from_error(self):
        output = dedent("""
            Waiting for address
            Attempting to connect to 10.0.0.202:22
            Attempting to connect to 1.2.3.4:22
            The fingerprint for the ECDSA key sent by the remote host is
            """)
        error = subprocess.CalledProcessError(1, ['foo'], output)
        address = parse_new_state_server_from_error(error)
        self.assertEqual('1.2.3.4', address)

    def test_parse_new_state_server_from_error_output_None(self):
        error = subprocess.CalledProcessError(1, ['foo'], None)
        address = parse_new_state_server_from_error(error)
        self.assertIs(None, address)

    def test_parse_new_state_server_from_error_no_output(self):
        address = parse_new_state_server_from_error(Exception())
        self.assertIs(None, address)
