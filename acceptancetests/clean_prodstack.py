#!/usr/bin/env python

import re
import os
import sys
import json
import logging
import subprocess


def get_current_controller_and_models():
    """Get Models name from current Controller, as well as Controller name.
       Returns:
       ctrl_name: a string for current Controller name
       model_list: a list for name of Models under the current Controller"""

    ctrl_output = json.loads(
        subprocess.check_output(['juju', 'controllers', '--format', 'json']))
    ctrl_name = ctrl_output['current-controller']
    model_output = json.loads(
        subprocess.check_output(['juju', 'models', '--format', 'json']))
    model_list = [m['name'] for m in model_output['models']]
    if not model_list:
        logging.error('No model has been found, exit.')
        sys.exit(1)
    logging.info(
        'Models under the Controller {} are:\n{}'.format(
            ctrl_name,
            '\n'.join(model_list)))
    return (ctrl_name, model_list)


def get_reserved_machine_ipaddress():
    """Machines (nova servers) associated to Models from current Controller
       will be reserved, as they are either in use or orchestrating services.
       Returns:
       ip_pool: a list stores ip address of nova servers from Models in
                current Controller."""

    ip_pool = []
    ctrl_name, model_list = get_current_controller_and_models()
    for item in model_list:
        model = '{}:{}'.format(ctrl_name, item)
        lines = subprocess.check_output(
            ['juju', 'show-machine', '--model', model],
            stderr=subprocess.STDOUT)
        # match ipv4 address, no validation as it comes from Juju
        ip_list = re.findall(r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', lines)
        if ip_list:
            [ip_pool.append(elem) for elem in ip_list if elem not in ip_pool]
    if ip_pool:
        # ip_pool stores ip address associated to machines from active
        # Models and Controllers, therefore need to be reserved
        logging.info(
            'DO NOT touch servers associated to these ip address:\n{}'.format(
                '\n'.join(ip_pool)))
    else:
        logging.info(
            'No ip address has been found, free to delete all nova servers.')
    return ip_pool


def get_target_server_list():
    """From nova list output, remove reserved machines based on their ip adress,
       the rest is a lifeover server list which need to be reclaimed.
       Returns:
       target_nova_server_dict: a dictionary stores server_id:server_name which
                                being sent to destroy_nova_server()"""

    ip_pool = get_reserved_machine_ipaddress()
    egrep_out = []
    reserved_nova_server_dict = {}
    full_nova_server_dict = {}
    target_nova_server_dict = {}
    temp_file = '/tmp/burton-test-nova-list.txt'
    with open(temp_file, 'w') as f:
        subprocess.check_call(['nova', 'list'], stdout=f)
    if not os.path.isfile(temp_file):
        logging.error('Failed to get nova server list, exit.')
        sys.exit(1)
    with open(temp_file, 'r') as stream:
        # using readlines() as the outout from nova list is tiny
        # using juju- as a search prefix as all instances created
        # by Juju are in align with this format.
        for line in stream.readlines():
            if 'juju-' in line:
                full_nova_server_dict[
                    filter(None, line.split('|'))[0].strip()] = filter(
                    None, line.split('|'))[1].strip()
    if not ip_pool:
        # if there is no reserved server associated to
        # ip address from ip_pool, delete them all
        target_nova_server_dict = full_nova_server_dict
        return target_nova_server_dict
    else:
        ip_pool_filter = '({})[[:space:]]'.format('|'.join(ip_pool))
        try:
            egrep_out = subprocess.check_output(
                ['egrep', ip_pool_filter, temp_file],
                stderr=subprocess.STDOUT)
        except subprocess.CalledProcessError:
            # file existence has been checked, egrep expression has also
            # been locally tested, it's highly likely a no match here.
            logging.info('No match, delete all.')
            target_nova_server_dict = full_nova_server_dict
            return target_nova_server_dict

    reserved_nova_server = [
        filter(None, elem.split('|')) for elem in egrep_out.split('\n')]
    reserved_nova_server = filter(None, reserved_nova_server)
    for item in reserved_nova_server:
        reserved_nova_server_dict[item[0].strip()] = item[1].strip()
    # make a copy to avoid change full_nova_server_dict directly
    target_nova_server_dict = full_nova_server_dict
    for key in reserved_nova_server_dict:
        if key in target_nova_server_dict:
            reserved_msg = target_nova_server_dict.pop(key, None)
            logging.info('{} has been reserved.'.format(reserved_msg))
    if not target_nova_server_dict:
        logging.info('No server need to be reclaimed, exit')
        sys.exit(0)
    return target_nova_server_dict


def destroy_nova_server():
    """Using nova delete to remove leftover servers gathered from
       get_target_server_list()
       Returns: None"""

    target_nova_server_dict = get_target_server_list()
    # delete nova server by its name
    for key in target_nova_server_dict:
        try:
            subprocess.check_output(
                ['nova', 'delete', target_nova_server_dict[key]],
                stderr=subprocess.STDOUT)
            logging.info(
                'nova server {} has been deleted.'.format(
                    target_nova_server_dict[key]))
        except subprocess.CalledProcessError:
            logging.warning(
                'Failed to delete {}, manual check required.'.format(
                    target_nova_server_dict[key]))


def main():
    logging.basicConfig(level=logging.INFO)
    logging.getLogger('clean_prodstack').setLevel(logging.INFO)
    destroy_nova_server()


if __name__ == '__main__':
    main()
