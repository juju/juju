import subprocess
import os


def persistent_modprobe(module):
    """Load a kernel module and configure for auto-load on reboot."""
    if not os.path.exists('/etc/rc.modules'):
        open('/etc/rc.modules', 'a')
        os.chmod('/etc/rc.modules', 111)
    with open('/etc/rc.modules', 'r+') as modules:
        if module not in modules.read():
            modules.write('modprobe %s\n' % module)


def update_initramfs(version='all'):
    """Updates an initramfs image."""
    return subprocess.check_call(["dracut", "-f", version])
