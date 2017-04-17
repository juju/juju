"""Tests for assess_destroy_model module."""

import logging
import StringIO
import yaml
import os
import io

from mock import (
    Mock,
    call,
    patch,
    mock_open
    )
from assess_add_credentials import (
    verify_add_credentials,
    get_credentials,
    verify_bootstrap,
    verify_credentials_match,
    add_aws,
    add_gce,
    add_maas,
    add_azure,
    add_joyent,
    add_rackspace,
    parse_args,
    main,
    )
from tests import (
    parse_error,
    TestCase,
    )
from utility import (
    JujuAssertionError,
    )


dummy_test_pass = u"""
  credentials:
    aws:
      aws:
        auth-type: access-key
        access-key: foo
        secret-key: verysecret-key"""

dummy_test_fail = u"""
  credentials:
    aws:
      aws:
        auth-type: access-key
        access-key: bar
        secret-key: verysecret-key"""

dummy_creds = u"""
    credentials:
      aws:
        credentials:
          auth-type: access-key
          access-key: foo
          secret-key: verysecret-key
      azure:
        credentials:
          auth-type: service-principal-secret
          application-id: foo-bar-baz
          application-password: somepass
          subscription-id: someid
      google:
        credentials:
          auth-type: oauth2
          client-email: foo@developer.gserviceaccount.com
          client-id: foo.apps.googleusercontent.com
          private-key: |
            -----BEGIN PRIVATE KEY-----
            somekeyfoo
            -----END PRIVATE KEY-----
          project-id: gothic-list-89514
      joyent:
        credentials:
          auth-type: userpass
          algorithm: rsa-sha256
          private-key: |
            -----BEGIN RSA PRIVATE KEY-----
            somekeybar
            -----END RSA PRIVATE KEY-----
          sdc-key-id: AA:AA
          sdc-user: dummyuser
      maas:
        credentials:
          auth-type: oauth1
          maas-oauth: EXAMPLE:DUMMY:OAUTH
      rackspace:
        credentials:
          auth-type: userpass
          password: somepass
          tenant-name: "123456789"
          username: userfoo
    """

dummy_loaded = yaml.load(dummy_creds)


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
        args = parse_args(argv)
        with patch("assess_add_credentials.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_add_credentials.assess_add_credentials",
                       autospec=True) as mock_assess:
                main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_assess.assert_called_once_with(args)


class TestAssess(TestCase):

    def test_verify_add_credentials(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        args = parse_args(argv)
        cred = dummy_loaded['credentials']['aws']
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                verify_add_credentials(args, 'aws', cred)
        self.assertEqual(
            [call('juju add-credential aws'),
             call().__enter__(),
             call().__enter__().expect('Enter credential name:'),
             call().__enter__().sendline('aws'),
             call().__enter__().expect('Enter access-key:'),
             call().__enter__().sendline('foo'),
             call().__enter__().expect('Enter secret-key:'),
             call().__enter__().sendline('verysecret-key'),
             call().__exit__(None, None, None)],
            mock_assess.mock_calls)

    def test_get_credentials(self):
        m = mock_open()
        with patch('assess_add_credentials.open', m, create=True) as o:
            o.return_value = io.StringIO(dummy_creds)
            with patch.dict(os.environ, {'HOME': '/'}):
                out = get_credentials('aws')
        self.assertEqual(out, dummy_loaded['credentials']['aws'])

    def test_verify_bootstrap(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        args = parse_args(argv)
        with patch.dict(os.environ, {'JUJU_DATA': '/', 'HOME': '/'}):
            with patch('shutil.copy', autospec=True, return_value=None):
                with patch(
                    "assess_destroy_model.BootstrapManager.booted_context",
                        autospec=True) as mock_bc:
                    with patch('deploy_stack.client_from_config',
                               return_value=client) as mock_cfc:
                        verify_bootstrap(args)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)

    def test_verify_credentials_match_pass(self):
        m = mock_open()
        with patch('assess_add_credentials.open', m, create=True) as o:
            o.return_value = io.StringIO(dummy_test_pass)
            with patch.dict(os.environ, {'JUJU_DATA': '/'}):
                verify_credentials_match(
                    'aws', dummy_loaded['credentials']['aws'])

    def test_verify_credentials_match_fail(self):
        m = mock_open()
        creds = dummy_loaded['credentials']['aws']
        with patch('assess_add_credentials.open', m, create=True) as o:
            o.return_value = io.StringIO(dummy_test_fail)
            with patch.dict(os.environ, {'JUJU_DATA': '/'}):
                pattern = 'Credential miss-match after manual add'
                with self.assertRaisesRegexp(JujuAssertionError, pattern):
                    verify_credentials_match('aws', creds)

    def test_add_aws(self):
        env = 'aws'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                add_aws(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('aws'),
             call.expect('Enter access-key:'),
             call.sendline('foo'),
             call.expect('Enter secret-key:'),
             call.sendline('verysecret-key')],
            mock_assess.mock_calls)

    def test_add_gce(self):
        env = 'google'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                add_gce(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('google'),
             call.expect('Select auth-type:'),
             call.sendline('oauth2'),
             call.expect('Enter client-id:'),
             call.sendline('foo.apps.googleusercontent.com'),
             call.expect('Enter client-email:'),
             call.sendline('foo@developer.gserviceaccount.com'),
             call.expect('Enter private-key:'),
             call.send('-----BEGIN PRIVATE KEY-----\n'
                       'somekeyfoo\n-----END PRIVATE KEY-----\n'),
             call.sendline(''),
             call.expect('Enter project-id:'),
             call.sendline('gothic-list-89514')],
            mock_assess.mock_calls)

    def test_add_maas(self):
        env = 'maas'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                add_maas(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('maas'),
             call.expect('Enter maas-oauth:'),
             call.sendline('EXAMPLE:DUMMY:OAUTH')],
            mock_assess.mock_calls)

    def test_add_azure(self):
        env = 'azure'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                add_azure(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('azure'),
             call.expect('Select auth-type:'),
             call.sendline('service-principal-secret'),
             call.expect('Enter application-id:'),
             call.sendline('foo-bar-baz'),
             call.expect('Enter subscription-id:'),
             call.sendline('someid'),
             call.expect('Enter application-password:'),
             call.sendline('somepass')],
            mock_assess.mock_calls)

    def test_add_joyent(self):
        env = 'joyent'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                with patch.dict(os.environ, {'HOME': '/'}):
                    add_joyent(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('joyent'),
             call.expect('Enter sdc-user:'),
             call.sendline('dummyuser'),
             call.expect('Enter sdc-key-id:'),
             call.sendline('AA:AA'),
             call.expect('Enter private-key-path:'),
             call.sendline('/cloud-city/joyent-key'),
             call.expect(',rsa-sha512]:'),
             call.sendline('rsa-sha256')],
            mock_assess.mock_calls)

    def test_add_rackspace(self):
        env = 'rackspace'
        creds = dummy_loaded['credentials'][env]
        with patch('pexpect.spawn', autospec=True) as mock_assess:
            with patch('assess_add_credentials.end_session',
                       return_value=None):
                add_rackspace(mock_assess, env, creds)
        self.assertEqual(
            [call.expect('Enter credential name:'),
             call.sendline('rackspace'),
             call.expect('Enter username:'),
             call.sendline('userfoo'),
             call.expect('Enter password:'),
             call.sendline('somepass'),
             call.expect('Enter tenant-name:'),
             call.sendline('123456789')],
            mock_assess.mock_calls)
