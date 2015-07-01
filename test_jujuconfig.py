from unittest import TestCase

from jujuconfig import (
    describe_substrate,
    get_jenv_path,
)


class TestDescribeSubstrate(TestCase):

    def test_describe_substrate_kvm(self):
        self.assertEqual('KVM (local)', describe_substrate(
            {'type': 'local', 'container': 'kvm'}))
        self.assertEqual('loca', describe_substrate(
            {'type': 'loca', 'container': 'kvm'}))

    def test_describe_substrate_lxc(self):
        self.assertEqual('LXC (local)', describe_substrate(
            {'type': 'local', 'container': 'lxc'}))
        self.assertEqual('LXC (local)', describe_substrate(
            {'type': 'local'}))

    def test_describe_substrate_hp(self):
        self.assertEqual('HPCloud', describe_substrate(
            {'type': 'openstack', 'auth-url': 'hpcloudsvc.com:35357/v2.0/'}))

    def test_describe_substrate_openstack(self):
        self.assertEqual('Openstack', describe_substrate(
            {'type': 'openstack', 'auth-url': 'pcloudsvc.com:35357/v2.0/'}))

    def test_describe_substrate_canonistack(self):
        self.assertEqual('Canonistack', describe_substrate(
            {
                'type': 'openstack',
                'auth-url':
                'https://keystone.canonistack.canonical.com:443/v2.0/'}))

    def test_describe_substrate_aws(self):
        self.assertEqual('AWS', describe_substrate({'type': 'ec2'}))

    def test_describe_substrate_joyent(self):
        self.assertEqual('Joyent', describe_substrate({'type': 'joyent'}))

    def test_describe_substrate_azure(self):
        self.assertEqual('Azure', describe_substrate({'type': 'azure'}))

    def test_describe_substrate_maas(self):
        self.assertEqual('MAAS', describe_substrate({'type': 'maas'}))


class TestConfig(TestCase):

    def test_get_jenv_path(self):
        self.assertEqual('home/environments/envname.jenv', get_jenv_path('home', 'envname'))
