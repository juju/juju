// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Constraints = `
Constraints constrain the possible instances that may be started by juju
commands. They are usually passed as a flag to commands that provision a new
machine (such as bootstrap, deploy, and add-machine).

Each constraint defines a minimum acceptable value for a characteristic of a
machine.  Juju will provision the least expensive machine that fulfills all the
constraints specified.  Note that these values are the minimum, and the actual
machine used may exceed these specifications if one that exactly matches does
not exist.

If a constraint is defined that cannot be fulfilled by any machine in the
model, no machine will be provisioned, and an error will be printed in the
machine's entry in juju status.

Constraint defaults can be set on an model or on specific services by
using the set-constraints command (see juju help set-constraints).  Constraints
set on the model or on a service can be viewed by using the get-
constraints command.  In addition, you can specify constraints when executing a
command by using the --constraints flag (for commands that support it).

Constraints specified on the model and service will be combined to
determine the full list of constraints on the machine(s) to be provisioned by
the command.  Service-specific constraints will override model-specific
constraints, which override the juju default constraints.

Constraints are specified as key value pairs separated by an equals sign, with
multiple constraints delimited by a space.

Constraint Types:

arch
   Arch defines the CPU architecture that the machine must have.  Currently
   recognized architectures:
      amd64 (default)
      i386
      arm

cpu-cores
   Cpu-cores is a whole number that defines the number of effective cores the
   machine must have available.

mem
   Mem is a float with an optional suffix that defines the minimum amount of RAM
   that the machine must have.  The value is rounded up to the next whole
   megabyte.  The default units are megabytes, but you can use a size suffix to
   use other units:

      M megabytes (default)
      G gigabytes (1024 megabytes)
      T terabytes (1024 gigabytes)
      P petabytes (1024 terabytes)

root-disk
   Root-Disk is a float that defines the amount of space in megabytes that must
   be available in the machine's root partition.  For providers that have
   configurable root disk sizes (such as EC2) an instance with the specified
   amount of disk space in the root partition may be requested.  Root disk size
   defaults to megabytes and may be specified in the same manner as the mem
   constraint.

container
   Container defines that the machine must be a container of the specified type.
   A container of that type may be created by juju to fulfill the request.
   Currently supported containers:
      none - (default) no container
      lxc - an lxc container
      kvm - a kvm container

cpu-power
   Cpu-power is a whole number that defines the speed of the machine's CPU,
   where 100 CpuPower is considered to be equivalent to 1 Amazon ECU (or,
   roughly, a single 2007-era Xeon).  Cpu-power is currently only supported by
   the Amazon EC2 model.

tags
   Tags defines the list of tags that the machine must have applied to it.
   Multiple tags must be delimited by a comma. Both positive and negative
   tags constraints are supported, the latter have a "^" prefix. Tags are
   currently only supported by the MaaS model.

spaces
   Spaces constraint allows specifying a list of Juju network space names a unit
   or machine needs access to. Both positive and negative (prefixed with "^")
   spaces can be in the list, separated by commas.

   Example: spaces=storage,db,^logging,^public (meaning, select machines connected
   to the storage and db spaces, but NOT to logging or public spaces).

   EC2 is the only provider supporting for spaces constraints. Support for other
   provides is planned for future releases.

instance-type
   Instance-type is the provider-specific name of a type of machine to deploy,
   for example m1.small on EC2 or A4 on Azure.  Specifying this constraint may
   conflict with other constraints depending on the provider (since the instance
   type my determine things like memory size etc.)

Example:

   juju add-machine --constraints "arch=amd64 mem=8G tags=foo,^bar"

See Also:
   juju help set-constraints
   juju help get-constraints
   juju help deploy
   juju help add-unit
   juju help add-machine
   juju help bootstrap
`
