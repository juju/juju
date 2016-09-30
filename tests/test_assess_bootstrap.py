from argparse import Namespace
from contextlib import contextmanager

from mock import patch

from assess_bootstrap import (
    assess_bootstrap,
    assess_metadata,
    INVALID_URL,
    parse_args,
    prepare_metadata,
    prepare_temp_metadata,
    )
from deploy_stack import (
    BootstrapManager,
    )
from fakejuju import (
    fake_juju_client,
    )
from jujupy import (
    _temp_env as temp_env,
    )
from tests import (
    FakeHomeTestCase,
    TestCase,
    )
from utility import (
    temp_dir,
    )


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
                    temp_env_name='baz', upload_tools=False, verbose=20),
                args)

    def test_parse_args_debug(self):
        args = parse_args(['base', 'foo', 'bar', '--debug'])
        self.assertEqual(args.debug, True)

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

    def test_parse_args_part(self):
        args = parse_args(['metadata'])
        self.assertEqual(args.part, 'metadata')


class TestPrepareMetadata(TestCase):

    def test_prepare_metadata(self):
        client = fake_juju_client()
        with patch.object(client, 'sync_tools') as sync_mock:
            with temp_dir() as metadata_dir:
                prepare_metadata(client, metadata_dir)
        sync_mock.assert_called_once_with('--local-dir', metadata_dir)

    def test_prepare_metadata_source(self):
        client = fake_juju_client()
        with patch.object(client, 'sync_tools') as sync_mock:
            with temp_dir() as metadata_dir:
                with temp_dir() as source_dir:
                    prepare_metadata(client, metadata_dir, source_dir)
        sync_mock.assert_called_once_with('--local-dir', metadata_dir,
                                          '--source', source_dir)

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


class TestAssessBootstrap(FakeHomeTestCase):

    @contextmanager
    def sub_assess_mocks(self):
        """Mock all of the sub assess functions."""
        base_patch = patch('assess_bootstrap.assess_base_bootstrap',
                           autospec=True)
        metadata_patch = patch('assess_bootstrap.assess_metadata',
                               autospec=True)
        with base_patch as base_mock, metadata_patch as metadata_mock:
            yield (base_mock, metadata_mock)

    def test_assess_bootstrap_part_base(self):
        args = parse_args(['base', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as (base_mock, metadata_mock):
                assess_bootstrap(args)
        self.assertEqual(1, base_mock.call_count)
        self.assertEqual(0, metadata_mock.call_count)

    def test_assess_bootstrap_part_metadata(self):
        args = parse_args(['metadata', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as (base_mock, metadata_mock):
                assess_bootstrap(args)
        self.assertEqual(0, base_mock.call_count)
        self.assertEqual(1, metadata_mock.call_count)


class TestAssessBaseBootstrap(FakeHomeTestCase):

    def test_assess_base_bootstrap_defaults(self):
        def check(myself):
            self.assertEqual(myself.env.config,
                             {'name': 'bar', 'type': 'foo'})
        with assess_bootstrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                with patch('deploy_stack.get_machine_dns_name'):
                    with patch('deploy_stack.dump_env_logs_known_hosts'):
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
        with assess_bootstrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                with patch('deploy_stack.get_machine_dns_name'):
                    with patch('deploy_stack.dump_env_logs_known_hosts'):
                        args = parse_args(['base', 'bar', '/foo'])
                        args.region = 'baz'
                        args.temp_env_name = 'qux'
                        assess_bootstrap(args)
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"(?m)^INFO Environment successfully bootstrapped.$")


class TestAssessMetadata(FakeHomeTestCase):

    @contextmanager
    def extend_bootstrap_cxt(self, juju_version=None):
        with assess_bootstrap_cxt(juju_version):
            with patch('deploy_stack.get_machine_dns_name'):
                with patch('deploy_stack.dump_env_logs_known_hosts'):
                    yield

    target_dict = {'name': 'qux', 'type': 'foo',
                   'agent-metadata-url': INVALID_URL}

    def get_url(self, bs_manager):
        """Wrap up the agent-metadata-url as model-config data."""
        url = bs_manager.client.env.config['agent-metadata-url']
        return {'agent-metadata-url': {'value': url}}

    def test_assess_metadata(self):
        def check(myself, metadata_source=None):
            self.assertEqual(self.target_dict, myself.env.config)
            self.assertIsNotNone(metadata_source)
        with self.extend_bootstrap_cxt('2.0-rc1'):
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
            self.assertEqual(self.target_dict, myself.env.config)
            self.assertEqual('agents', metadata_source)
        with self.extend_bootstrap_cxt('2.0-rc1'):
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                args = parse_args(['metadata', 'bar', '/foo'])
                args.temp_env_name = 'qux'
                bs_manager = BootstrapManager.from_args(args)
                with patch.object(
                        bs_manager.client, 'get_model_config',
                        side_effect=lambda: self.get_url(bs_manager)):
                    assess_metadata(bs_manager, 'agents')
