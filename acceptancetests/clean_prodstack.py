#!/usr/bin/env python

import re
import os
import sys
import logging
import subprocess

def get_current_controller_and_models():
    lines = subprocess.check_output(
        ['juju', 'models'], stderr=subprocess.STDOUT).split('\n')
    ctrl_name = [
        item.split(':')[-1].strip() for item in lines if 'Controller:' in item][0]
    for i in range(0, len(lines)):
        if ('Model' in lines[i] and
            'Cloud/Region' in lines[i] and
            i+1<len(lines)):
            model_lines = filter(None, lines[i+1:])
            break
    if not model_lines:
        logging.info('No model has been found, exit.')
        sys.exit(1)
    model_list = [elem.split(' ')[0].strip('*') for elem in model_lines]
    logging.info(
        'Models under the Controller {} are:\n{}'.format(
        ctrl_name,
        '\n'.join(model_list)))
    return (ctrl_name, model_list)


def get_reserved_machine_ipaddress():
    role = 'admin'
    ip_pool = []
    ctrl_name, model_list = get_current_controller_and_models()
    for item in model_list:
        model = '{}:{}/{}'.format(ctrl_name, role, item)
        lines = subprocess.check_output(
            ['juju', 'show-machine', '--model', model],
            stderr=subprocess.STDOUT)
        # match ipv4 address, no validation as it comes from Juju
        ip_list = re.findall(r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', lines)
        if ip_list:
            [ip_pool.append(elem) for elem in ip_list if elem not in ip_pool]
    if ip_pool:
        logging.info(
            'DO NOT touch servers associated to these ip address:\n{}'.format(
            '\n'.join(ip_pool)))
    else:
        logging.info(
            'No ip address has been found, free to delete all nova servers.')
    return ip_pool


def get_target_server_list():
    ip_pool = get_reserved_machine_ipaddress()
    egrep_out = []
    reserved_nova_server_dict = {}
    full_nova_server_dict = {}
    target_nova_server_dict = {}
    temp_file = '/tmp/burton-test-nova-list.txt'
    with open(temp_file, 'w') as f:
        subprocess.check_call(['nova', 'list'], stdout=f)
    if not os.path.isfile(temp_file):
        logging.info('Failed to get nova server list, exit.')
        sys.exit(1)
    with open(temp_file, 'r') as stream:
        # using readlines() as the outout from nova list is tiny
        for line in stream.readlines():
            if 'juju-' in line:
                full_nova_server_dict[
                    filter(None, line.split('|'))[0].strip()] = filter(
                    None, line.split('|'))[1].strip()
    if not ip_pool:
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
            logging.info(
                'Failed to delete {}, manual check required.'.format(
                target_nova_server_dict[key]))


def main():
    logging.basicConfig(level=logging.INFO)
    logging.getLogger('clean_prodstack').setLevel(logging.INFO)
    destroy_nova_server()


if __name__ == '__main__':
    main()