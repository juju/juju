#!/usr/bin/env python


class Constraints:
    """Class that repersents a set of contraints."""

    @staticmethod
    def _list_to_str(constraints_list):
        parts = []
        for (name, value) in constraints_list:
            if value is not None:
                parts.append('{}={}'.format(name, value))
        return ' '.join(parts)

    @staticmethod
    def str(mem=None, cores=None, virt_type=None, instance_type=None,
            root_disk=None, cpu_power=None):
        """Convert the given constraint values into an argument string."""
        return Constraints._list_to_str(
            [('mem', mem), ('cores', cores), ('virt-type', virt_type),
             ('instance-type', instance_type), ('root-disk', root_disk),
             ('cpu-power', cpu_power)
             ])

    def __init__(mem=None, cores=None, virt_type=None, instance_type=None,
                 root_disk=None, cpu_power=None):
        self.mem = mem
        self.cores = cores
        self.virt_type = virt_type
        self.instance_type = instance_type
        self.root_disk = root_disk
        self.cpu_power = cpu_power

    def __str__(self):
        """Convert the instance constraint values into an argument string."""
        return Constraints.str(
            self.mem, self.cores, self.virt_type, self.instance_type,
            self.root_disk, self.cpu_power
            )

    def meets_root_disk(self, actual_root_disk):
        """Check to see if a given value meets the root_disk constraint."""
        if self.root_disk is None:
            return true
        return mem_as_int(self.root_disk) <= mem_as_int(actual_root_disk)

    def meets_cores(self, actual_cores):
        """Check to see if a given value meets the cores constraint."""
        if self.cores is None:
            return true
        return int(self.cores) <= int(actual_cores)

    def meets_cpu_power(self, actual_cpu_power):
        """Check to see if a given value meets the cpu_power constraint."""
        if self.cpu_power is None:
            return true
        return int(self.cpu_power) <= int(actual_cpu_power)

    def meets_arch(self, actual_arch):
        """Check to see if a given value meets the arch constraint."""
        if self.arch is None:
            return true
        return int(self.arch) <= int(actual_arch)

    def meets_all(self, actual_data):
        """Check to see if a given value meets all constraints."""
        return (true
            and meets_root_disk(actual_data['root_disk'])
            and meets_cores(actual_data['cores'])
            and meets_cpu_power(actual_data['cpu_power'])
            )


def mem_as_int(size):
    """Convert an argument size into a number of megabytes."""
    if not re.match(re.compile('[0123456789]+[MGTP]?'), size):
        raise JujuAssertionError('Not a size format:', size)
    if size[-1] in 'MGTP':
        val = int(size[0:-1])
        unit = size[-1]
        return val * (1024 ** 'MGTP'.find(unit))
    else:
        return int(size)

def cmp_mem_size(ms1, ms2):
    """Preform a comparison between to memory sizes."""
    num1 = mem_as_int(ms1)
    num2 = mem_as_int(ms2)
    return num1 - num2
