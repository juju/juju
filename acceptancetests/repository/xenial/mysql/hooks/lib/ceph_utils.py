#
# Copyright 2012 Canonical Ltd.
#
# This file is sourced from lp:openstack-charm-helpers
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

import commands
import subprocess
import os
import shutil
import time
import lib.utils as utils

KEYRING = '/etc/ceph/ceph.client.%s.keyring'
KEYFILE = '/etc/ceph/ceph.client.%s.key'

CEPH_CONF = """[global]
 auth supported = %(auth)s
 keyring = %(keyring)s
 mon host = %(mon_hosts)s
"""


def execute(cmd):
    subprocess.check_call(cmd)


def execute_shell(cmd):
    subprocess.check_call(cmd, shell=True)


def install():
    ceph_dir = "/etc/ceph"
    if not os.path.isdir(ceph_dir):
        os.mkdir(ceph_dir)
    utils.install('ceph-common')


def rbd_exists(service, pool, rbd_img):
    (rc, out) = commands.getstatusoutput('rbd list --id %s --pool %s' %\
                                         (service, pool))
    return rbd_img in out


def create_rbd_image(service, pool, image, sizemb):
    cmd = [
        'rbd',
        'create',
        image,
        '--size',
        str(sizemb),
        '--id',
        service,
        '--pool',
        pool
        ]
    execute(cmd)


def pool_exists(service, name):
    (rc, out) = commands.getstatusoutput("rados --id %s lspools" % service)
    return name in out


def create_pool(service, name):
    cmd = [
        'rados',
        '--id',
        service,
        'mkpool',
        name
        ]
    execute(cmd)


def keyfile_path(service):
    return KEYFILE % service


def keyring_path(service):
    return KEYRING % service


def create_keyring(service, key):
    keyring = keyring_path(service)
    if os.path.exists(keyring):
        utils.juju_log('INFO', 'ceph: Keyring exists at %s.' % keyring)
    cmd = [
        'ceph-authtool',
        keyring,
        '--create-keyring',
        '--name=client.%s' % service,
        '--add-key=%s' % key
        ]
    execute(cmd)
    utils.juju_log('INFO', 'ceph: Created new ring at %s.' % keyring)


def create_key_file(service, key):
    # create a file containing the key
    keyfile = keyfile_path(service)
    if os.path.exists(keyfile):
        utils.juju_log('INFO', 'ceph: Keyfile exists at %s.' % keyfile)
    fd = open(keyfile, 'w')
    fd.write(key)
    fd.close()
    utils.juju_log('INFO', 'ceph: Created new keyfile at %s.' % keyfile)


def get_ceph_nodes():
    hosts = []
    for r_id in utils.relation_ids('ceph'):
        for unit in utils.relation_list(r_id):
            hosts.append(utils.relation_get('private-address',
                                            unit=unit, rid=r_id))
    return hosts


def configure(service, key, auth):
    create_keyring(service, key)
    create_key_file(service, key)
    hosts = get_ceph_nodes()
    mon_hosts = ",".join(map(str, hosts))
    keyring = keyring_path(service)
    with open('/etc/ceph/ceph.conf', 'w') as ceph_conf:
        ceph_conf.write(CEPH_CONF % locals())
    modprobe_kernel_module('rbd')


def image_mapped(image_name):
    (rc, out) = commands.getstatusoutput('rbd showmapped')
    return image_name in out


def map_block_storage(service, pool, image):
    cmd = [
        'rbd',
        'map',
        '%s/%s' % (pool, image),
        '--user',
        service,
        '--secret',
        keyfile_path(service),
        ]
    execute(cmd)


def filesystem_mounted(fs):
    return subprocess.call(['grep', '-wqs', fs, '/proc/mounts']) == 0


def make_filesystem(blk_device, fstype='ext4'):
    count = 0
    e_noent = os.errno.ENOENT
    while not os.path.exists(blk_device):
        if count >= 10:
            utils.juju_log('ERROR',
                'ceph: gave up waiting on block device %s' % blk_device)
            raise IOError(e_noent, os.strerror(e_noent), blk_device)
        utils.juju_log('INFO',
            'ceph: waiting for block device %s to appear' % blk_device)
        count += 1
        time.sleep(1)
    else:
        utils.juju_log('INFO',
            'ceph: Formatting block device %s as filesystem %s.' %
            (blk_device, fstype))
        execute(['mkfs', '-t', fstype, blk_device])


def place_data_on_ceph(service, blk_device, data_src_dst, fstype='ext4'):
    # mount block device into /mnt
    cmd = ['mount', '-t', fstype, blk_device, '/mnt']
    execute(cmd)

    # copy data to /mnt
    try:
        copy_files(data_src_dst, '/mnt')
    except:
        pass

    # umount block device
    cmd = ['umount', '/mnt']
    execute(cmd)

    _dir = os.stat(data_src_dst)
    uid = _dir.st_uid
    gid = _dir.st_gid

    # re-mount where the data should originally be
    cmd = ['mount', '-t', fstype, blk_device, data_src_dst]
    execute(cmd)

    # ensure original ownership of new mount.
    cmd = ['chown', '-R', '%s:%s' % (uid, gid), data_src_dst]
    execute(cmd)


# TODO: re-use
def modprobe_kernel_module(module):
    utils.juju_log('INFO', 'Loading kernel module')
    cmd = ['modprobe', module]
    execute(cmd)
    cmd = 'echo %s >> /etc/modules' % module
    execute_shell(cmd)


def copy_files(src, dst, symlinks=False, ignore=None):
    for item in os.listdir(src):
        s = os.path.join(src, item)
        d = os.path.join(dst, item)
        if os.path.isdir(s):
            shutil.copytree(s, d, symlinks, ignore)
        else:
            shutil.copy2(s, d)


def ensure_ceph_storage(service, pool, rbd_img, sizemb, mount_point,
                        blk_device, fstype, system_services=[]):
    """
    To be called from the current cluster leader.
    Ensures given pool and RBD image exists, is mapped to a block device,
    and the device is formatted and mounted at the given mount_point.

    If formatting a device for the first time, data existing at mount_point
    will be migrated to the RBD device before being remounted.

    All services listed in system_services will be stopped prior to data
    migration and restarted when complete.
    """
    # Ensure pool, RBD image, RBD mappings are in place.
    if not pool_exists(service, pool):
        utils.juju_log('INFO', 'ceph: Creating new pool %s.' % pool)
        create_pool(service, pool)

    if not rbd_exists(service, pool, rbd_img):
        utils.juju_log('INFO', 'ceph: Creating RBD image (%s).' % rbd_img)
        create_rbd_image(service, pool, rbd_img, sizemb)

    if not image_mapped(rbd_img):
        utils.juju_log('INFO', 'ceph: Mapping RBD Image as a Block Device.')
        map_block_storage(service, pool, rbd_img)

    # make file system
    # TODO: What happens if for whatever reason this is run again and
    # the data is already in the rbd device and/or is mounted??
    # When it is mounted already, it will fail to make the fs
    # XXX: This is really sketchy!  Need to at least add an fstab entry
    #      otherwise this hook will blow away existing data if its executed
    #      after a reboot.
    if not filesystem_mounted(mount_point):
        make_filesystem(blk_device, fstype)

        for svc in system_services:
            if utils.running(svc):
                utils.juju_log('INFO',
                               'Stopping services %s prior to migrating '\
                               'data' % svc)
                utils.stop(svc)

        place_data_on_ceph(service, blk_device, mount_point, fstype)

        for svc in system_services:
            utils.start(svc)
