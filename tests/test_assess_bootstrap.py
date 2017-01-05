from argparse import Namespace
from contextlib import contextmanager

from mock import (
    Mock,
    patch,
    )

from assess_bootstrap import (
    assess_bootstrap,
    assess_metadata,
    assess_to,
    get_controller_address,
    get_controller_hostname,
    INVALID_URL,
    parse_args,
    prepare_metadata,
    prepare_temp_metadata,
    thin_booted_context,
    )
from deploy_stack import (
    BootstrapManager,
    )
from fakejuju import (
    fake_juju_client,
    )
from jujupy import (
    _temp_env as temp_env,
    Status,
    )
from tests import (
    FakeHomeTestCase,
    TestCase,
    )
from utility import (
    JujuAssertionError,
    temp_dir,
    )


class TestThinBootedContext(TestCase):

    def make_bs_manager_mock(self, jes_enabled=False):
        client = Mock()
        client.attach_mock(Mock(), 'bootstrap')
        client.attack_mock(Mock(return_value=jes_enabled), 'is_jes_enabled')
        client.attack_mock(Mock(), 'kill_controller')
        bs_manager = Mock()
        bs_manager.attach_mock(client, 'client')
        bs_manager.attack_mock(Mock(), 'tear_down')

        @contextmanager
        def top_mock():
            yield 'machines'

        @contextmanager
        def cxt_mock(machines):
            yield

        bs_manager.attach_mock(Mock(side_effect=top_mock), 'top_context')
        bs_manager.attach_mock(Mock(side_effect=cxt_mock),
                               'bootstrap_context')
        bs_manager.attach_mock(Mock(side_effect=cxt_mock), 'runtime_context')
        return bs_manager

    def test_thin_booted_context(self):
        bs_manager = self.make_bs_manager_mock()
        with thin_booted_context(bs_manager):
            pass
        bs_manager.top_context.assert_called_once_with()
        bs_manager.bootstrap_context.assert_called_once_with('machines')
        bs_manager.runtime_context.assert_called_once_with('machines')
        bs_manager.client.kill_controller.assert_called_once_with()
        bs_manager.client.bootstrap.assert_called_once_with()

    def test_thin_booted_context_kwargs(self):
        bs_manager = self.make_bs_manager_mock(True)
        with thin_booted_context(bs_manager, alpha='foo', beta='bar'):
            pass
        bs_manager.client.bootstrap.assert_called_once_with(
            alpha='foo', beta='bar')


class TestParseArgs(TestCase):

    def test_parse_args(self):
        with temp_dir() as log_dir:
            args = parse_args(['base', 'foo', 'bar', log_dir, 'baz'])
            self.assertEqual(
                Namespace(
                    agent_stream=None, agent_url=None, bootstrap_host=None,
                    deadline=None, debug=False, env='foo', juju_bin='bar',
                    keep_env=False, local_metadata_source=None, logs=log_dir,
                    machine=[], part='base', region=None, series=None,
                    temp_env_name='baz', to=None, upload_tools=False,
                    verbose=20),
                args)

    def test_parse_args_debug(self):
        args = parse_args(['base', 'foo', 'bar', '--debug'])
        self.assertIsTrue(args.debug)

    def test_parse_args_region(self):
        args = parse_args(['base', 'foo', 'bar', '--region', 'foo'])
        self.assertEqual(args.region, 'foo')

    def test_parse_args_temp_env_name(self):
        args = parse_args(['base', 'fee', 'fi', 'foe', 'fum'])
        self.assertEqual(args.temp_env_name, 'fum')

    def test_parse_args_local_metadata_source(self):
        args = parse_args(['base', 'foo', 'bar',
                           '--local-metadata-source', 'qux'])
        self.assertEqual(args.local_metadata_source, 'qux')

    def test_parse_args_to(self):
        args = parse_args(['to', 'foo', 'bar', '--to', 'qux'])
        self.assertEqual(args.to, 'qux')

    def test_parse_args_part(self):
        args = parse_args(['metadata'])
        self.assertEqual(args.part, 'metadata')


class TestPrepareMetadata(TestCase):

    def test_prepare_metadata(self):
        client = fake_juju_client()
        with patch.object(client, 'sync_tools') as sync_mock:
            with temp_dir() as metadata_dir:
                prepare_metadata(client, metadata_dir)
        sync_mock.assert_called_once_with(metadata_dir, None)

    def test_prepare_metadata_with_stream(self):
        client = fake_juju_client()
        with patch.object(client, 'sync_tools') as sync_mock:
            with temp_dir() as metadata_dir:
                prepare_metadata(client, metadata_dir, "testing")
        sync_mock.assert_called_once_with(metadata_dir, "testing")

    def test_prepare_temp_metadata(self):
        client = fake_juju_client()
        with patch('assess_bootstrap.prepare_metadata',
                   autospec=True) as prepare_mock:
            with prepare_temp_metadata(client) as metadata_dir:
                pass
        prepare_mock.assert_called_once_with(client, metadata_dir, None)

    def test_prepare_temp_metadata_source(self):
        client = fake_juju_client()
        with patch('assess_bootstrap.prepare_metadata',
                   autospec=True) as prepare_mock:
            with temp_dir() as source_dir:
                with prepare_temp_metadata(
                        client, source_dir) as metadata_dir:
                    pass
        self.assertEqual(source_dir, metadata_dir)
        self.assertEqual(0, prepare_mock.call_count)


@contextmanager
def assess_bootstrap_cxt(juju_version=None):
    """Mock helper functions used in the bootstrap process.

    Use the bar environment."""
    if juju_version is None:
        juju_version = '1.25.5'
    call_cxt = patch('subprocess.call')
    cc_cxt = patch('subprocess.check_call')
    gv_cxt = patch('jujupy.EnvJujuClient.get_version',
                   side_effect=lambda cls: juju_version)
    gjo_cxt = patch('jujupy.EnvJujuClient.get_juju_output', autospec=True,
                    return_value='')
    imc_cxt = patch('jujupy.EnvJujuClient.iter_model_clients',
                    autospec=True, return_value=[])
    env_cxt = temp_env({'environments': {'bar': {'type': 'foo'}}})
    with call_cxt, cc_cxt, gv_cxt, gjo_cxt, env_cxt, imc_cxt:
        yield


@contextmanager
def extended_bootstrap_cxt(juju_version=None):
    """Extention to assess_bootstrap_cxt if you are using runtime_context."""
    with assess_bootstrap_cxt(juju_version):
        gmdn_cxt = patch('deploy_stack.get_machine_dns_name')
        delkh_cxt = patch('deploy_stack.dump_env_logs_known_hosts')
        with gmdn_cxt, delkh_cxt:
            yield


class TestAssessBootstrap(FakeHomeTestCase):

    @contextmanager
    def sub_assess_mocks(self):
        """Mock all of the sub assess functions."""
        base_patch = patch('assess_bootstrap.assess_base_bootstrap',
                           autospec=True)
        metadata_patch = patch('assess_bootstrap.assess_metadata',
                               autospec=True)
        to_patch = patch('assess_bootstrap.assess_to', autospec=True)
        with base_patch as base_mock, metadata_patch as metadata_mock:
            with to_patch as to_mock:
                yield (base_mock, metadata_mock, to_mock)

    def test_assess_bootstrap_part_base(self):
        args = parse_args(['base', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as (base_mock, metadata_mock,
                                             to_mock):
                assess_bootstrap(args)
        self.assertEqual(1, base_mock.call_count)
        self.assertEqual(0, metadata_mock.call_count)
        self.assertEqual(0, to_mock.call_count)

    def test_assess_bootstrap_part_metadata(self):
        args = parse_args(['metadata', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as (base_mock, metadata_mock,
                                             to_mock):
                assess_bootstrap(args)
        self.assertEqual(0, base_mock.call_count)
        self.assertEqual(1, metadata_mock.call_count)
        self.assertEqual(0, to_mock.call_count)

    def test_assess_bootstrap_part_to(self):
        args = parse_args(['to', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as (base_mock, metadata_mock,
                                             to_mock):
                assess_bootstrap(args)
        self.assertEqual(0, base_mock.call_count)
        self.assertEqual(0, metadata_mock.call_count)
        self.assertEqual(1, to_mock.call_count)


class TestAssessBaseBootstrap(FakeHomeTestCase):

    def test_assess_base_bootstrap_defaults(self):
        def check(myself):
            self.assertEqual(myself.env.config,
                             {'name': 'bar', 'type': 'foo'})
        with extended_bootstrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                assess_bootstrap(parse_args(['base', 'bar', '/foo']))
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"(?m)^INFO Environment successfully bootstrapped.$")

    def test_assess_base_bootstrap_region_temp_env(self):
        def check(myself):
            self.assertEqual(
                myself.env.config, {
                    'name': 'qux', 'type': 'foo', 'region': 'baz'})
            self.assertEqual(myself.env.environment, 'qux')
        with extended_bootstrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                args = parse_args(['base', 'bar', '/foo'])
                args.region = 'baz'
                args.temp_env_name = 'qux'
                assess_bootstrap(args)
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"(?m)^INFO Environment successfully bootstrapped.$")


class TestAssessMetadata(FakeHomeTestCase):

    target_dict = {'name': 'qux', 'type': 'foo',
                   'agent-metadata-url': INVALID_URL}

    def get_url(self, bs_manager):
        """Wrap up the agent-metadata-url as model-config data."""
        url = bs_manager.client.env.get_option('agent-metadata-url')
        return {'agent-metadata-url': {'value': url}}

    def test_assess_metadata(self):
        def check(myself, metadata_source=None):
            self.assertEqual(self.target_dict, myself.env._config)
            self.assertIsNotNone(metadata_source)
        with extended_bootstrap_cxt('2.0.0'):
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                args = parse_args(['metadata', 'bar', '/foo'])
                args.temp_env_name = 'qux'
                bs_manager = BootstrapManager.from_args(args)
                with patch.object(
                        bs_manager.client, 'get_model_config',
                        side_effect=lambda: self.get_url(bs_manager)):
                    assess_metadata(bs_manager, None)

    def test_assess_metadata_local_source(self):
        def check(myself, metadata_source=None):
            self.assertEqual(self.target_dict, myself.env._config)
            self.assertEqual('agents', metadata_source)
        with extended_bootstrap_cxt('2.0.0'):
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                args = parse_args(['metadata', 'bar', '/foo'])
                args.temp_env_name = 'qux'
                bs_manager = BootstrapManager.from_args(args)
                with patch.object(
                        bs_manager.client, 'get_model_config',
                        side_effect=lambda: self.get_url(bs_manager)):
                    assess_metadata(bs_manager, 'agents')

    def test_assess_metadata_valid_url(self):
        with extended_bootstrap_cxt('2.0.0'):
            with patch('jujupy.EnvJujuClient.bootstrap', autospec=True):
                args = parse_args(['metadata', 'bar', '/foo'])
                args.temp_env_name = 'qux'
                bs_manager = BootstrapManager.from_args(args)
                with patch.object(
                        bs_manager.client, 'get_model_config',
                        return_value={'agent-metadata-url':
                                      {'value': 'example.com/valid'}}):
                    with self.assertRaises(JujuAssertionError):
                        assess_metadata(bs_manager, None)


class TestAssessTo(FakeHomeTestCase):

    def test_get_controller_address(self):
        status = Status({'machines': {"0": {'dns-name': '255.1.1.0'}}}, '')
        client = fake_juju_client()
        with patch('jujupy.EnvJujuClient.status_until', return_value=[status],
                   autospec=True):
            self.assertEqual('255.1.1.0', get_controller_address(client))

    def test_get_controller_hostname(self):
        controller_client = Mock(wraps=fake_juju_client())
        client = Mock(wraps=fake_juju_client())
        with patch.object(client, 'get_controller_client',
                          return_value=controller_client):
            with patch.object(controller_client, 'run',
                              return_value=' maas-node-x\n') as run_mock:
                self.assertEqual('maas-node-x',
                                 get_controller_hostname(client))
        run_mock.assert_called_once_with(['hostname'], machines=['0'],
                                         use_json=False)

    def test_assess_to(self):
        DEST = 'test-host'

        def check(myself, to):
            self.assertEqual({'name': 'qux', 'type': 'foo'},
                             myself.env._config)
            self.assertEqual(DEST, to)
        with extended_bootstrap_cxt('2.0.0'):
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                with patch('assess_bootstrap.get_controller_hostname',
                           return_value=DEST, autospec=True):
                    args = parse_args(['to', 'bar', '/foo',
                                       '--to', DEST])
                    args.temp_env_name = 'qux'
                    bs_manager = BootstrapManager.from_args(args)
                    assess_to(bs_manager, args.to)

    def test_assess_to_requires_to(self):
        with self.assertRaises(ValueError):
            assess_to('bs_manager', None)

    def test_assess_to_fails(self):
        with extended_bootstrap_cxt('2.0.0'):
            with patch('jujupy.EnvJujuClient.bootstrap', autospec=True):
                with patch('assess_bootstrap.get_controller_address',
                           return_value='255.1.1.0', autospec=True):
                    args = parse_args(['to', 'bar', '/foo',
                                       '--to', '255.1.13.0'])
                    args.temp_env_name = 'qux'
                    bs_manager = BootstrapManager.from_args(args)
                    with self.assertRaises(JujuAssertionError):
                        assess_to(bs_manager, args.to)
