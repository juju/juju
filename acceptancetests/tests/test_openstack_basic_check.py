from argparse import Namespace
import os


from tests import TestCase
from openstack_basic_check import (
    get_args,
    set_environ,
    )


__metaclass__ = type


class TestGetArgs(TestCase):

    def test_get_args_defaults(self):
        args = get_args(
            ['--user', 'admin', '--password', 'password', '--tenant', 'foo',
             '--region', 'bar', '--auth-url', 'http://example.com'])
        self.assertEqual('admin', args.user)
        self.assertEqual('password', args.password)
        self.assertEqual('foo', args.tenant)
        self.assertEqual('bar', args.region)
        self.assertEqual('http://example.com', args.auth_url)

    def test_get_args_raises_without_user(self):
        with self.assertRaisesRegexp(
                Exception, 'User must be provided'):
            get_args(
                ['--password', 'password', '--tenant', 'foo',
                 '--region', 'bar', '--auth-url', 'http://example.com'])

    def test_get_args_raises_without_password(self):
        with self.assertRaisesRegexp(
                Exception, 'Password must be provided'):
            get_args(
                ['--user', 'admin', '--tenant', 'foo',
                 '--region', 'bar', '--auth-url', 'http://example.com'])

    def test_get_args_raises_without_tenant(self):
        with self.assertRaisesRegexp(
                Exception, 'Tenant must be provided'):
            get_args(
                ['--user', 'admin', '--password', 'password',
                 '--region', 'bar', '--auth-url', 'http://example.com'])

    def test_get_args_raises_without_region(self):
        with self.assertRaisesRegexp(
                Exception, 'Region must be provided'):
            get_args(
                ['--user', 'admin', '--password', 'password', '--tenant',
                 'foo', '--auth-url', 'http://example.com'])

    def test_get_args_raises_without_auth_url(self):
        with self.assertRaisesRegexp(
                Exception, 'auth-url must be provided'):
            get_args(
                ['--user', 'admin', '--password', 'password', '--tenant',
                 'foo', '--region', 'bar'])


class TestSetEnviron(TestCase):

    def test_set_environ(self):
        set_environ(Namespace(user='admin', password='passwd',
                    tenant='bar', region='foo',
                    auth_url='http://a.com:5000/v2.0'))
        self.assertEqual(os.environ['OS_USERNAME'], 'admin')
        self.assertEqual(os.environ['OS_PASSWORD'], 'passwd')
        self.assertEqual(os.environ['OS_TENANT_NAME'], 'bar')
        self.assertEqual(os.environ['OS_REGION_NAME'], 'foo')
        self.assertEqual(os.environ['OS_AUTH_URL'],
                         'http://a.com:5000/v2.0')
