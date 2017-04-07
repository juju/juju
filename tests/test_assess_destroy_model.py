"""Tests for assess_destroy_model module."""

import logging
import StringIO
from textwrap import dedent

from mock import (
    Mock,
    call,
    patch,
    )

from assess_destroy_model import (
    add_model,
    destroy_model,
    switch_model,
    parse_args,
    main,
    )
from jujupy import (
    fake_juju_client,
    )
from tests import (
    parse_error,
    TestCase,
    )
from utility import (
    JujuAssertionError
    )

list_model_initial = dedent(
    """{
    "models": [
        {"name": "controller",
         "controller-name": "foo-tmp-env"},
        {"name": "foo-tmp-env",
         "controller-name": "foo-tmp-env"}],
    "current-model": "foo-tmp-env"
    }""")

list_model_destroy = dedent(
    """{
    "models": [
        {"name": "controller",
         "controller-name": "foo-tmp-env"},
        {"name": "foo-tmp-env",
         "controller-name": "foo-tmp-env"}]
    }""")

list_model_add = dedent(
    """{
    "models": [
        {"name": "controller",
         "controller-name": "foo-tmp-env"},
        {"name": "test-tmp-env",
         "controller-name": "foo-tmp-env"},
        {"name": "foo-tmp-env",
         "controller-name": "foo-tmp-env"}],
    "current-model": "test-tmp-env"
    }""")

list_model_switch = dedent(
    """{
    "models": [
        {"name": "controller",
         "controller-name": "foo-tmp-env"},
        {"name": "foo-tmp-env",
         "controller-name": "foo-tmp-env"}],
    "current-model": "bar-tmp-env"
    }""")


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
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_destroy_model.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_destroy_model.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_destroy_model.assess_destroy_model",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_add_model(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_add
        fake_client.add_model.return_value = fake_client
        add_model(fake_client)
        self.assertEqual(
            [call.bootstrap(),
             call.add_model('test-tmp-env'),
             call.get_juju_output('list-models', '--format', 'json',
                                  include_e=False)],
            fake_client.mock_calls)

    def test_add_model_fails(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_initial
        fake_client.add_model.return_value = fake_client
        pattern = 'Expected test-tmp-env got foo-tmp-env'
        with self.assertRaisesRegexp(JujuAssertionError, pattern):
            add_model(fake_client)

    def test_destroy_model(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_destroy
        destroy_model(fake_client, fake_client)
        self.assertEqual(
            [call.bootstrap(),
             call.destroy_model(),
             call.get_juju_output('list-models', '--format', 'json',
                                  include_e=False)],
            fake_client.mock_calls)

    def test_destroy_model_fails(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_initial
        pattern = 'Juju failed to unset model after it was destroyed'
        with self.assertRaisesRegexp(JujuAssertionError, pattern):
            destroy_model(fake_client, fake_client)

    def test_switch_model(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_initial
        switch_model(fake_client, 'foo-tmp-env', 'foo-tmp-env')
        self.assertEqual(
            [call.bootstrap(),
             call.switch(controller='foo-tmp-env', model='foo-tmp-env'),
             call.get_juju_output('list-models', '--format', 'json',
                                  include_e=False)],
            fake_client.mock_calls)

    def test_switch_model_fails(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_juju_output.return_value = list_model_switch
        pattern = 'Expected test-tmp-env got bar-tmp-env'
        with self.assertRaisesRegexp(JujuAssertionError, pattern):
            switch_model(fake_client, 'foo-tmp-env', 'test-tmp-env')
