// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/juju/core/backups"
)

// RecoveryApplication identifies one archived workload application for
// recovery substrate checks.
type RecoveryApplication struct {
	// Name is the application's name; on Kubernetes it is also the name
	// of its StatefulSet in the model namespace.
	Name string

	// UUID is the application's logical identity, preserved by recovery.
	UUID string

	// Units lists the archived unit names of the application.
	Units []string

	// PersistentVolumeClaims lists Kubernetes persistent volume or claim
	// names recorded by the archived storage filesystems.
	PersistentVolumeClaims []string
}

// RecoveryModel identifies one archived model for recovery substrate
// checks.
type RecoveryModel struct {
	// Name is the model's name; on Kubernetes it is also the model's
	// namespace name.
	Name string

	// UUID is the model's logical identity, preserved by recovery.
	UUID string

	// Applications lists the archived workload applications whose
	// substrate the check inventories. It excludes the controller
	// application: the controller namespace is disposable bootstrap
	// output, not surviving substrate.
	Applications []RecoveryApplication
}

// RecoverySubstrateParams describes the surviving substrate a provider
// must verify before a recovery bootstrap provisions anything.
type RecoverySubstrateParams struct {
	// WorkloadOnly skips controller fencing on a workload model's cluster.
	// The hosting cluster must be checked separately before provisioning.
	WorkloadOnly bool

	// ControllerUUID is the archived source controller's identity.
	ControllerUUID string

	// ControllerName is the archived source controller's name; on
	// Kubernetes the controller namespace derives from it.
	ControllerName string

	// ControllerModelUUID identifies the controller model; its namespace
	// is the controller namespace, checked separately.
	ControllerModelUUID string

	// Models is the archived model inventory, including the controller
	// model.
	Models []RecoveryModel
}

// RecoverySubstrateReport lists the archived workload substrate the
// surviving target no longer holds. Missing substrate never aborts
// recovery: it is reported so the operator knows what the first
// reconcile will re-create (workload objects) or find empty (volumes)
// before the recovered controller starts.
type RecoverySubstrateReport struct {
	// HostCloudRegion is cluster metadata read during recovery preparation.
	// It supplies Kubernetes bootstrap configuration absent from database dumps.
	HostCloudRegion string
	// MissingWorkloads names archived applications whose Kubernetes
	// workload objects (the StatefulSet named after the application)
	// are gone from the surviving namespace.
	MissingWorkloads []string

	// MissingPersistentVolumeClaims names archived volume claims that no
	// longer exist in the surviving namespace. A re-created claim comes
	// back empty: this is data loss surfaced, not healed.
	MissingPersistentVolumeClaims []string
}

// Empty reports whether the target holds every piece of archived
// substrate the check looked for.
func (r *RecoverySubstrateReport) Empty() bool {
	return r == nil || (len(r.MissingWorkloads) == 0 && len(r.MissingPersistentVolumeClaims) == 0)
}

// RecoverySubstrateChecker is implemented by providers that can verify a
// recovery target's surviving substrate before provisioning. The check is
// read-only; any failure aborts bootstrap before anything is created.
// Providers that can inventory workload substrate report what is missing
// in the returned report instead of failing: missing substrate is
// degraded recovery, not a refusal.
type RecoverySubstrateChecker interface {
	CheckRecoverySubstrate(ctx context.Context, params RecoverySubstrateParams) (*RecoverySubstrateReport, error)
}

// RecoveryControllerPreparer verifies surviving substrate and prepares the
// provider for a replacement controller without rejecting its workload models.
// It must perform the same checks as RecoverySubstrateChecker and create no
// provider resources. It is called before writing local controller records.
type RecoveryControllerPreparer interface {
	PrepareForRecovery(ctx BootstrapContext, params RecoverySubstrateParams) (*RecoverySubstrateReport, error)
}

// ControllerBackupProvisioning reads the running controller's provisioning
// facts without modifying provider resources. A backup fails when a required
// image or storage identity cannot be established.
type ControllerBackupProvisioning interface {
	ControllerBackupProvisioning(ctx context.Context, controllerID string) (*backups.KubernetesProvisioning, error)
}

// RecoveryMachineParams preserves the logical source identity while giving
// the replacement's provider resources a distinct physical name.
type RecoveryMachineParams struct {
	BootstrapParams
	MachineID string
}

// RecoveryMachineProvisioner provisions only replacement resources. The
// preparer is read-only. Provisioning returns a scoped cleanup even on error;
// cleanup must be idempotent and must never destroy the source model.
// The finaliser remains usable after ProvisionRecoveryMachine returns.
type RecoveryMachineProvisioner interface {
	RecoveryControllerPreparer
	ProvisionRecoveryMachine(BootstrapContext, RecoveryMachineParams) (*BootstrapResult, func(context.Context) error, error)
}
