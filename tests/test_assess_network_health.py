import json
import yaml
import StringIO
import logging
from textwrap import dedent

from mock import (
    call,
    patch,
    Mock,
    )
from tests import (
    TestCase,
    parse_error
    )
from jujupy import (
    fake_juju_client
)
from assess_network_health import (
    main,
    assess_network_health,
    setup_testing_environment,
    connect_to_existing_model,
    setup_dummy_deployment,
    setup_bundle_deployment,
    get_juju_status,
    ensure_juju_agnostic_visibility,
    neighbor_visibility,
    ensure_exposed,
    setup_expose_test,
    parse_expose_results,
    parse_final_results,
    parse_targets,
    parse_args,
    ConnectionError
    )

apps = {'foo':
        {'charm-name': 'foo',
         'exposed': True,
         'units': {
            'foo/0': {'machine': '0',
                      'public-address': '1.1.1.1',
                      'subordinates': {
                        'bar/0': {
                            'public-address': '1.1.1.1'}}},
            'foo/1': {'machine': '1',
                      'public-address': '1.1.1.2',
                      'subordinates': {
                        'bar/1': {
                            'public-address': '1.1.1.2'}}}}},
        'bar':
        {'charm-name': 'bar',
         'exposed': False}}


bundle_string = dedent("""\
services:
  foo:
    charm: local:trusty/foo
    num_units: 1
    expose: true
  bar:
    charm: local:trusty/bar
    num_units: 1
series: trusty
relations:
- - foo:baz
  - bar:baz
""")

bundle_yaml = yaml.safe_load(bundle_string)
dummy_charm = 'dummy'
series = 'trusty'

class TestAssessNetworkHealth(TestCase):

    def test_assess_network_health(self):
        pass

    def test_agonostic(self):
        pass

    def test_visibility(self):
        pass

    def test_exposure(self):
        pass

    def test_setup_testing_environment(self):
        pass

    def test_connect_to_existing_model(self):
        pass

    def test_dummy_deployment(self):
        client = fake_juju_client()
        client.bootstrap()
        client.deploy(dummy_charm)

    def test_bundle_deployment(self):
        client = fake_juju_client()
        client.bootstrap()
        client.deploy_bundle(bundle_string)

    def test_expose_test_deployment(self):
        pass

    def test_get_juju_status(self):
        client = fake_juju_client()

    def test_parse_expose_results(self):
        exposed = ['bar', 'baz']
        results = {}
        expected = {"fail": ("foo"), "pass": ("bar", "baz")}

    def test_parse_final_results_with_fail(self):
        agnostic = {"0": {"1.1.1.1": False}}
        visible = {"bar/0": {"foo": {"foo/0": False, "foo/1": True}}}
        exposed = {"fail": ("foo"), "pass": ("bar", "baz")}
        with self.assertRaises(ConnectionError) as context:
            parse_final_results(agnostic, visible, exposed)
        error_strings = ["Failed to ping machine 0 at address 1.1.1.1",
                         "NH-Unit bar/0 failed to contact unit(s): ['foo/0']",
                         "Application(s) foo failed expose test"]
        for line in error_strings:
            self.assertTrue(line in context.exception.message)

    def test_parse_final_results_without_fail(self):
        agnostic = {"0": {"1.1.1.1": True}}
        visible = {"bar/0": {"foo": {"foo/0": True, "foo/1": True}}}
        exposed = {"fail": (), "pass": ("foo", "bar", "baz")}
        parse_final_results(agnostic, visible, exposed)

    def test_ping_units(self):
        pass

    def test_to_json(self):
        pass

    def test_parse_targets(self):
        expected = {'foo': {'foo/0': '1.1.1.1', 'foo/1': '1.1.1.2'}}
        targets = parse_targets(apps)
        self.assertEqual(expected, targets)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_network_health.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_network_health.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_network_health.assess_network_health",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, bundle=None,
                                            series='trusty')


class TestParseArgs(TestCase):

    def test_common_args(self):
        # Test common args
        common = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"]
        args = parse_args(common)
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)
        # Test specific args
        nh_args = parse_args([common, "--model", "foo", "--bundle", "bar"])
        self.assertEqual("foo", nh_args.model)
        self.assertEqual("bar", nh_args.bundle)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn("Network Health", fake_stdout.getvalue())
