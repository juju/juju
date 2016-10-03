from argparse import Namespace
from contextlib import contextmanager

from mock import patch

from assess_bootstrap import (
    assess_bootstrap,
    parse_args,
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
        with base_patch as base_mock:
            yield base_mock

    def test_assess_bootstrap_part_base(self):
        args = parse_args(['base', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as base_mock:
                assess_bootstrap(args)
        self.assertEqual(1, base_mock.call_count)

    def test_assess_bootstrap_part_metadata(self):
        args = parse_args(['metadata', 'bar'])
        with assess_bootstrap_cxt():
            with self.sub_assess_mocks() as base_mock:
                assess_bootstrap(args)
        self.assertEqual(0, base_mock.call_count)


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
