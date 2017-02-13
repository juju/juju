import subprocess


def persistent_modprobe(module):
    """Load a kernel module and configure for auto-load on reboot."""
    with open('/etc/modules', 'r+') as modules:
        if module not in modules.read():
            modules.write(module + "\n")


def update_initramfs(version='all'):
    """Updates an initramfs image."""
    return subprocess.check_call(["update-initramfs", "-k", version, "-u"])
