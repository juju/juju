import json
import yaml

from mock import (
    call,
    patch,
    Mock,
    )
from tests import (
    TestCase
    )
from jujupy import (
    fake_juju_client
)
from assess_network_health import (
    main,
    parse_args,
    assess_network_health,
    neighbor_visibility,
    ensure_exposed,
    parse_results,
    ping_units,
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
        client.deploy_bundle()

    def test_expose_test_deployment(self):
        client = fake_juju_client()
        new_client = setup_expose_test(client)

    def test_get_juju_status(self):
        client = fake_juju_client()
        expected = client.get_status().status
        test = get_juju_status(client)
        self.assertEqual(expected, test)

    def test_parse_expose_results(self):
        pass

    def test_parse_final_results_with_fail(self):
        agnostic = {"0": {"1.1.1.1": True}, "1": {"1.1.1.2": True}}
        visible = {"bar/0": {"foo": {"foo/0": False, "foo/1": True}},
                   "bar/1": {"foo": {"bar/0": True, "bar/1": True}}}
        exposed = {"fail": ("foo"), "pass": ("bar", "baz")}
        expected = [""]
        out = parse_final_results(agnostic, visible, exposed)
        self.assertRaises()

    def test_parse_final_results_without_fail(self):
        agnostic = {"0": {"1.1.1.1": True}, "1": {"1.1.1.2": True}}
        visible = {"bar/0": {"foo": {"foo/0": True, "foo/1": True}},
                   "bar/1": {"foo": {"bar/0": True, "bar/1": True}}}
        exposed = {"fail": (), "pass": ("foo", "bar", "baz")}

        out = parse_final_results(agnostic, visible, exposed)
        self.assertEqual(out, expected)

    def test_ping_units(self):
        pass

    def test_to_json(self):
        pass

    def test_parse_targets(self):
        expected = {}
        targets = parse_targets(apps)
        self.assertEqual(expected, targets)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_min_version.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_min_version.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("deploy_stack.client_from_config",
                           return_value=client) as mock_c:
                    with patch("assess_min_version.assess_min_version",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


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
        self.assertIn("network health", fake_stdout.getvalue())
