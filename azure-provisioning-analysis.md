# Azure Provider: Resource Provisioning Analysis

This document details all resources that Juju provisions when bootstrapping a controller or creating machines in Microsoft Azure.

**Provider Location**: `internal/provider/azure/`  
**Analysis Date**: 2026-06-04  
**Juju Branch**: 3.6

---

## Table of Contents

1. [Bootstrap Operations](#bootstrap-operations)
2. [Instance/Machine Creation](#instancemachine-creation)
3. [Credentials and Authentication](#credentials-and-authentication)
4. [IP Addresses and Networking](#ip-addresses-and-networking)
5. [Security Groups and Firewall Rules](#security-groups-and-firewall-rules)
6. [Storage (Disks and Volumes)](#storage-disks-and-volumes)
7. [Other Azure Resources](#other-azure-resources)
8. [Bootstrap vs. Regular Machine Comparison](#bootstrap-vs-regular-machine-comparison)

---

## Bootstrap Operations

### Bootstrap Flow

The bootstrap process is coordinated through `environ.go` `Bootstrap()` method:

1. **Resource Group Creation** - Creates a controller-specific resource group (unless using an existing one)
2. **Common Resource Deployment Folding** - Unlike non-controller models, bootstrap folds common resources into the bootstrap machine's deployment
3. **Bootstrap Machine Deployment** - Creates the controller machine with all network resources in a single deployment

### Bootstrap-Specific Resources

#### Resource Group
- Tagged with model UUID and controller UUID
- Named based on environment name
- Location-specific

#### Network Resources (when creating new)
- **Virtual Network**: 192.168.0.0/16 address space
  - Name: "juju-internal-network"
  - Location: Inherits from environment location
  
- **Controller Subnet**: 192.168.16.0/20
  - Name: "juju-controller-subnet"
  - Purpose: Controller machines and API access
  
- **Internal Subnet**: 192.168.0.0/20
  - Name: "juju-internal-subnet"
  - Purpose: Non-controller machines

- **Network Security Group**: "juju-internal-nsg"
  - Shared by all machines in the resource group
  - Bootstrap-specific rules for SSH and Juju API access
  - Applied to all subnets

#### Compute Resources
- **Availability Set**: "juju-controller"
  - Shared by all controller machines
  - Platform Fault Domain Count: 2 or 3 (location-dependent)
  - SKU: "Aligned" (for managed disks)

- **VM Extension** (CentOS only):
  - Extension: Microsoft.OSTCExtensions/CustomScriptForLinux
  - Executes CustomData via base64 decoding

#### Bootstrap Credentials & Identity

If using instance roles (managed identity):
- Creates user-assigned managed identity via `CreateAutoInstanceRole`
- Identity naming follows Azure resource conventions
- Role assignment at subscription level with custom role permissions
- Permissions defined in `internal/azureauth/JujuActions`

---

## Instance/Machine Creation

### StartInstance Flow

Orchestrated through `StartInstance` and `startInstance` methods:

1. **Instance Spec Identification**: Finds suitable instance type and image
2. **Virtual Machine Creation**: Calls `createVirtualMachine`
3. **Availability Set Handling**: Attempts with availability set; retries without on conflict

### Per-Machine Resources

#### Network Resources

**Public IP Address** (if `allocate-public-ip` constraint not explicitly false):
- Name: `{vmName}-public-ip`
- Allocation Method: Static (default) or Dynamic (for Basic SKU load balancers)
- IPv4 version only
- SKU: Determined by `loadBalancerSkuName` configuration

**Network Interface Cards (NICs)**:
- One per subnet (multiple NICs per machine supported)
- Primary NIC: `{vmName}-primary`
- Additional NICs: `{vmName}-interface-{i}`
- IP Configuration: Dynamic private IP allocation via DHCP
- Primary NIC attached to public IP if configured

#### Compute Resources

**Virtual Machine**:
- Type: Specified via instance type constraints
- Storage Profile: Managed disk OS disk (see Storage section)
- OS Profile: Linux/Windows configuration with cloud-init/cloud-config
- Network Profile: All NICs attached
- Availability Set: Referenced if not bootstrapping or on retry
- Managed Identity: Attached for controller machines using instance roles

**Availability Set** (only if `createAvailabilitySet=true`, first machine per type):
- Name: Derived from machine name and controller status
- Platform Fault Domain Count: Location-dependent (2 or 3)
- SKU: "Aligned" for managed disks

**CustomScript VM Extension** (CentOS only):
- Extension: Microsoft.OSTCExtensions/CustomScriptForLinux
- Executes CustomData via base64 decoding and bash execution
- Not used for Ubuntu (uses cloud-init via cloud-config)

#### Subnet Selection Logic

- **Bootstrap**: Uses controller subnet (192.168.16.0/20)
- **Controller (non-bootstrap)**: Uses controller subnet
- **Non-controller**: Uses internal subnet (192.168.0.0/20)
- **Placement override**: Uses specified placement subnet if provided

---

## Credentials and Authentication

### Authentication Types Supported

#### 1. Service Principal Secret (`clientCredentialsAuthType`)

**Credential Attributes**:
- `application-id`: Azure AD application ID
- `subscription-id`: Azure subscription ID
- `application-password`: OAuth secret
- `application-object-id`: Optional, for advanced scenarios
- `managed-subscription-id`: Optional

**Implementation**: Uses `azidentity.NewClientSecretCredential()`

#### 2. Device Code Flow (Interactive)

- Interactive OAuth flow for initial credential generation
- Generates temporary service principal with password
- Converts to `clientCredentialsAuthType` for storage

#### 3. Managed Identity Authentication

**Credential Attributes**:
- `managed-identity-path`: Format is `{resourceGroup}/{identityName}` or `{subscription}/{resourceGroup}/{identityName}`
- `subscription-id`: Azure subscription ID

**Implementation**:
- Within machines: `azidentity.NewManagedIdentityCredential()`
- Scoped by ResourceID

### Bootstrap Credential Finalization

`FinaliseBootstrapCredential` handles:
- Conversion of instance-role constraints to managed identity credentials
- No-op for non-instance-role scenarios
- Preserves credential for subsequent controller machines

### Credential Verification

- `PrepareForBootstrap` verifies credentials can access Azure if `ShouldVerifyCredentials()` is true
- Uses a test API call to validate Azure access

---

## IP Addresses and Networking

### Private IP Addresses

- **Allocation Method**: Dynamic (DHCP) for all machines
- **Subnet Ranges**:
  - Internal subnet: 192.168.0.0/20 (non-controller machines)
  - Controller subnet: 192.168.16.0/20 (controller machines)
- **NIC Configuration**: Each NIC gets one or more IP configurations
- **Primary NIC**: First NIC always marked as primary

### Public IP Addresses

- **Default**: Static allocation method (preserves across reboots)
- **Basic SKU Configuration**: Dynamic allocation if using Basic tier load balancer
- **Naming Convention**: `{vmName}-public-ip`
- **Protocol**: IPv4 only
- **SKU**: Determined by `loadBalancerSkuName` configuration
- **Assignment**: Associated with primary NIC if enabled

### Virtual Network

- **Network Name**: "juju-internal-network" (unless custom vnet specified)
- **Address Space**: 192.168.0.0/16 (divided into subnets)
- **Scope**: Per-model (each resource group gets its own virtual network)
- **Location**: Inherits from environment location

### Subnet Architecture

#### Internal Subnet
- **Name**: "juju-internal-subnet"
- **Prefix**: 192.168.0.0/20
- **Purpose**: Non-controller machines
- **NSG**: Internal Security Group attached

#### Controller Subnet
- **Name**: "juju-controller-subnet"
- **Prefix**: 192.168.16.0/20
- **Purpose**: Controller machines only
- **NSG**: Internal Security Group attached
- **Created**: Only during bootstrap (when API ports needed)

### Network Interface Details

Per NIC configuration:
- **DeviceIndex**: Sequential index (0 for primary)
- **MAC Address**: Normalized from Azure format
- **Configuration Methods**: DHCP or Static
- **ProviderId**: Azure resource ID
- **Shadow Addresses**: Public IP addresses associated with NIC

---

## Security Groups and Firewall Rules

### Security Group Design

**Single NSG per Resource Group**:
- Name: "juju-internal-nsg"
- Shared by all machines in the resource group
- Applied to all subnets in the virtual network

### Default Security Rules

Rules are created in `networkSecurityRules`:

#### 1. SSH Inbound (Priority: 100)
- **Direction**: Inbound
- **Protocol**: TCP
- **Port**: 22
- **Source**: `*` (any)
- **Destination**: `*` (all machines)
- **Access**: Allow

#### 2. Juju API Inbound (Priority: 101+)
- **Direction**: Inbound
- **Protocol**: TCP
- **Port**: Specified by controller config (default 17070)
- **Source**: `*` (any)
- **Destination**: Controller subnet only (192.168.16.0/20)
- **Access**: Allow
- **Multiple Rules**: One per API port (priority incremented: 101, 102, etc.)

#### 3. Autocert HTTP Challenge (Priority: 102+ if enabled)
- **Port**: 80
- **Condition**: Only if `AutocertDNSName()` is configured
- **Destination**: Controller subnet only

### Rule Priority Allocation

- **Juju-managed rules**: 100-199 (internal rules)
- **User-defined rules**: 200-3999 (via firewaller)
- **Maximum priority**: 4096

### Security Group Application

- Attached at subnet level (not instance level)
- Both controller and internal subnets use the same NSG
- Rules are namespace-agnostic (all machines see same rules)

---

## Storage (Disks and Volumes)

### OS Disk (Root Disk)

Created via `createVirtualMachine` and `newStorageProfile`:

#### Disk Type
- **Type**: Managed disk (not unmanaged storage accounts)
- **Storage Account Type**:
  - Default: StandardSSD_LRS (Standard SSD Locally Redundant)
  - Options: Standard_LRS, Premium_LRS, StandardSSD_LRS
  - Configurable via RootDisk constraints

#### Disk Size
- **Minimum**: 30 GiB (constant `minRootDiskSize`)
- **User-specified**: Via `RootDisk` constraint if > 30 GiB
- **Maximum**: Capped by instance type maximum if applicable

#### Encryption
- **Optional**: Via `diskEncryptionInfo()`
- **Implementation**: Can reference existing Disk Encryption Set or create new via Key Vault
- **Resource**: Azure DiskEncryptionSet

### Additional Storage (Block Storage/Volumes)

Managed through `storage.go`:

#### Provider Configuration
- **Provider Type**: "azure" (block storage only)
- **Default Pool**: "azure-premium" pool with Premium_LRS

#### Supported Storage Types
- Premium_LRS (premium managed disks)
- Standard_LRS (standard managed disks)
- StandardSSD_LRS (standard SSD managed disks)

#### Volume Creation
- **Method**: `CreateVolumes()` creates managed disks via Compute API
- **Tagging**: Tagged with Juju machine/storage tags
- **Size Conversion**: From MiB to GiB
- **Disk Type**: Per storage config

#### Volume Attachment
- Managed through Juju machine agent (not provider)
- Requires manual attachment script or agent provisioning

### Storage Constraints

- **Maximum Volume Size**: 1023 GiB per disk
- **Disk Encryption**: Optional via Key Vault integration
- **Releasable**: False (storage tied to resource group)
- **Scope**: Environment-level (can't move between resource groups)

### Disk Encryption Setup

If encryption enabled via RootDisk attributes:

#### DES (Disk Encryption Set)
- **Name**: Derived from vault name prefix
- **Location**: Model's resource group
- **References**: Azure Key Vault
- **Identity**: System-assigned for key access

#### Key Vault (created if needed)
- Stores encryption keys
- Access policy for DES identity
- Azure-managed key rotation

---

## Other Azure Resources

### Availability Sets

Created via `createVirtualMachine`:

#### Controller Availability Set
- **Name**: "juju-controller"
- **Scope**: Shared by all controller machines
- **Creation**: On first controller machine
- **Reuse**: All subsequent controllers added to same set

#### Non-Controller Availability Sets
- **Name**: Derived from machine name
- **Creation**: One per non-controller machine initially
- **Reuse**: Subsequent machines of same type reuse set
- **Conflict Handling**: Retry without availability set if conflict

#### Properties
- **Platform Fault Domain Count**: Location-dependent (2 or 3)
- **Managed**: True (uses managed disks)
- **SKU**: "Aligned" (for managed disks)

### Resource Tags

Applied to all resources at creation via `tags.ResourceTags`:

#### Juju-specific Tags
- `juju-model`: Model UUID
- `juju-controller`: Controller UUID
- `juju-machine-name`: VM name (on each resource)

#### User-defined Tags
- Custom tags from environment config

### Virtual Machine Identities

For controllers using managed identity:

- **Identity Type**: UserAssigned
- **Identity Reference**: Full Azure resource ID
- **Used by**: Controller agent for Azure API calls
- **Scope**: Subscription-level (via role assignments)

### Common Deployments

From `createCommonResourceDeployment`:

#### For Non-Controller Models (created immediately)
- Virtual Network
- Network Security Group
- Subnets (internal + controller)

#### For Controller Models
- Folded into bootstrap machine deployment
- Allows non-bootstrap machines to start without waiting

### Deployment Patterns

- **Resource-level**: ARM Template deployments via Incremental mode
- **Subscription-level**: Needed for role definitions and assignments
- **Polling**: Non-common deployments are polled to completion
- **Async**: Common deployments use fire-and-forget pattern

### Resource Cleanup

- **By Tag**: Failed machines cleaned up by `juju-machine-name` tag
- **Entire Resource Group**: `Destroy()` deletes entire model's resource group
- **Deployment Cancellation**: Failed bootstrap cancels in-progress deployments

---

## Bootstrap vs. Regular Machine Comparison

| Aspect | Bootstrap | Regular Machine |
|--------|-----------|-----------------|
| **Network Creation** | Creates if not existing | Uses pre-created (from common deployment) |
| **Common Resources** | Folded into bootstrap deployment | Created separately upfront |
| **Availability Set** | Uses "juju-controller" | Uses machine-specific set |
| **Deployment Wait** | Polls for completion | Does not wait |
| **Managed Identity** | May create (if instance-role) | Inherited from bootstrap |
| **Security Rules** | May create API-specific rules | Uses existing rules |
| **Subnet Selection** | Controller subnet (192.168.16.0/20) | Internal subnet (192.168.0.0/20) |
| **Cleanup on Failure** | Cancels deployment + destroys RG | Stops instance by tag |
| **Public IP** | Created with static allocation | Created based on constraints |
| **VM Extension** | May create (CentOS) | May create (CentOS) |
| **Resource Group** | Creates new or uses existing | Uses bootstrap-created RG |

---

## Summary

Juju's Azure provider implements a comprehensive resource provisioning strategy that:

1. **Separates bootstrap and regular machine workflows** to optimize resource creation timing
2. **Uses managed identities** for secure credential management when enabled
3. **Implements subnet-based isolation** between controller and worker machines
4. **Leverages availability sets** for high availability within Azure fault domains
5. **Supports flexible storage options** including disk encryption via Key Vault
6. **Applies consistent tagging** for lifecycle management and resource tracking
7. **Uses ARM template deployments** for atomic resource creation
8. **Provides network security** through centralized NSG with rule priority management

This design ensures robust, secure, and maintainable Azure infrastructure for Juju deployments.
