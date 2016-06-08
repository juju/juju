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


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['foo', 'bar'])
        self.assertEqual(args, Namespace(
            juju='foo', env='bar', debug=False, region=None,
            temp_env_name=None))

    def test_parse_args_debug(self):
        args = parse_args(['foo', 'bar', '--debug'])
        self.assertEqual(args.debug, True)

    def test_parse_args_region(self):
        args = parse_args(['foo', 'bar', '--region', 'foo'])
        self.assertEqual(args.region, 'foo')

    def test_parse_args_temp_env_name(self):
        args = parse_args(['foo', 'bar', 'foo'])
        self.assertEqual(args.temp_env_name, 'foo')


class TestAssessBootstrap(FakeHomeTestCase):

    @contextmanager
    def assess_boostrap_cxt(self):
        call_cxt = patch('subprocess.call')
        cc_cxt = patch('subprocess.check_call')
        gv_cxt = patch('jujupy.EnvJujuClient.get_version',
                       side_effect=lambda cls: '1.25.5')
        gjo_cxt = patch('jujupy.EnvJujuClient.get_juju_output', autospec=True,
                        return_value='')
        imc_cxt = patch('jujupy.EnvJujuClient.iter_model_clients',
                        autospec=True, return_value=[])
        env_cxt = temp_env({'environments': {'bar': {'type': 'foo'}}})
        with call_cxt, cc_cxt, gv_cxt, gjo_cxt, env_cxt, imc_cxt:
            yield

    def test_assess_bootstrap_defaults(self):
        def check(myself):
            self.assertEqual(myself.env.config,
                             {'name': 'bar', 'type': 'foo'})
        with self.assess_boostrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                with patch('deploy_stack.get_machine_dns_name'):
                    with patch('deploy_stack.dump_env_logs_known_hosts'):
                        assess_bootstrap('/foo', 'bar', False, None, None)
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"(?m)^INFO Environment successfully bootstrapped.$")

    def test_assess_bootstrap_region_temp_env(self):
        def check(myself):
            self.assertEqual(
                myself.env.config, {
                    'name': 'qux', 'type': 'foo', 'region': 'baz'})
            self.assertEqual(myself.env.environment, 'qux')
        with self.assess_boostrap_cxt():
            with patch('jujupy.EnvJujuClient.bootstrap', side_effect=check,
                       autospec=True):
                with patch('deploy_stack.get_machine_dns_name'):
                    with patch('deploy_stack.dump_env_logs_known_hosts'):
                        assess_bootstrap('/foo', 'bar', False, 'baz', 'qux')
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"(?m)^INFO Environment successfully bootstrapped.$")
