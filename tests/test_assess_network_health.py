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


class TestAssessNetworkHealth(TestCase):

    @contextmanager
    def prepare_deploy_mock(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        """Mock a client and the deploy function."""
        fake_client = fake_juju_client()
        env = fake_client.env
        fake_client = Mock(wraps=fake_client)
        # force the real env, because attribute access on a wrapped one is
        # weird.
        fake_client.env = env
        fake_client.bootstrap()
        with patch('jujupy.ModelClient.deploy',
                   autospec=True) as deploy_mock:
            yield fake_client, deploy_mock

    def test_assess_network_health_deploy_without_bundle(self):
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_network_health(fake_client, bundle=None)
            deploy_mock.assert_called_once_with(fake_client,
                ('--show-log', 'deploy', '-m', 'nettemp:nettemp',
                 './repository/charms/ubuntu', '-n', '2'))

    def test_assess_network_health_deploy_with_bundle(self):
        fake_bundle = 'bundle.yaml'
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_network_health(fake_client, bundle=fake_bundle)
            deploy_mock.assert_called_once_with(fake_client,
                ('--show-log', 'deploy', '-m', 'nettemp:nettemp',
                 'bundle.yaml'))

    def test_nodes_return_unreachable(self):
        pass

    def test_raises_when_visibility_contains_failure(self):
        pass

    def test_passes_when_returns_match_exposed(self):
        pass

    def test_raises_when_returns_show_unexposed(self):
        pass

    def test_parse_results_matches_expectation(self):
        vis_input = {'foo/0': {'bar': {'bar/0': True, 'bar/1': True},
                               'baz': {'baz/0': True, 'baz/1': False}},
                     'foo/1': {'bar': {'bar/0': True, 'bar/1': True},
                               'baz': {'baz/0': False, 'baz/1': False}}}
        exp_input = {'fail': ('bar', 'baz'), 'pass': ('foo',)}
        exposed = ['bar', 'baz']
        expected_message_lines = [
            "NH-Unit foo/1 failed to contact unit(s): ['baz/0', 'baz/1']",
            "NH-Unit foo/0 failed to contact unit(s): ['baz/0', 'baz/1']",
            "Service(s) ('bar', 'baz') failed expose test"
            ]
        for line in expected_message_lines:
            assert line in output


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
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn("network health", fake_stdout.getvalue())
