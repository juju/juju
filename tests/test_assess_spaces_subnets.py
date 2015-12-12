import assess_spaces_subnets as jss
from test_assess_container_networking import JujuMockTestCase


class TestSubnetsSpaces(JujuMockTestCase):
    def test_ipv4_to_int(self):
        self.assertEqual(
            jss.ipv4_to_int('1.2.3.4'),
            0x01020304)

        self.assertEqual(
            jss.ipv4_to_int('255.255.255.255'),
            0xFFFFFFFF)

    def test_ipv4_in_cidr(self):
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.1/32'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.0/24'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.0.0/16'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.0.0.0/8'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '0.0.0.0/0'))

        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.1/32'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.0/24'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.0.0/16'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.0.0.0/8'))

    network_config = {
        'apps': ['10.0.0.0/16', '10.1.0.0/16'],
        'backend': ['10.2.0.0/16', '10.3.0.0/16'],
        'default': ['10.4.0.0/16', '10.5.0.0/16'],
        'dmz': ['10.6.0.0/16', '10.7.0.0/16'],
    }
    charms_to_space = {
        'haproxy': {'space': 'dmz'},
        'mediawiki': {'space': 'apps'},
        'memcached': {'space': 'apps'},
        'mysql': {'space': 'backend'},
        'mysql-slave': {
            'space': 'backend',
            'charm': 'mysql',
        },
    }

    def test_assess_spaces_subnets(self):
        # The following table is derived from the above settings

        # Charm ---------------- space --- address in subnet
        # haproxy              - dmz     - 10.6.0.2
        # mediawiki, memcached - apps    - 10.0.0.2
        # mysql, mysql-slace   - backend - 10.2.0.2

        # We translate the above table into these responses to "ip -o addr",
        # which are assigned to machines that we have found by running this
        # test. The order is fixed because we iterate over keys in dictionaries
        # in a sorted order.
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '4')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '5')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '6')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '7')

        jss._assess_spaces_subnets(
            self.client, self.network_config, self.charms_to_space)

    def test_assess_spaces_subnets_fail(self):
        # The output in this test is set to be the same as in
        # test_assess_spaces_subnets with machines 1 and 2 swapped.
        # This results in mediawiki/0 appearing in the dmz instead of apps
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '4')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '5')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '6')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '7')

        self.assertRaisesRegexp(
            ValueError, 'Found mediawiki/0 in dmz, expected apps',
            jss._assess_spaces_subnets,
            self.client, self.network_config, self.charms_to_space)

    def test_assess_spaces_subnets_fail_to_find_all_spaces(self):
        # Should get an error if we can't find the space for each unit
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.assertRaisesRegexp(
            ValueError, 'Could not find spaces for all units',
            jss._assess_spaces_subnets,
            self.client, self.network_config, self.charms_to_space)
