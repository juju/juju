// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import "github.com/juju/juju/core/facades"

// SupportedFacadeVersions returns the list of facades that the api supports.
func SupportedFacadeVersions() facades.FacadeVersions {
	return facadeVersions
}

// facadeVersions lists the best version of facades that we want to support. This
// will be used to pick out a default version for communication, given the list
// of known versions that the API server tells us it is capable of supporting.
// This map should be updated whenever the API server exposes a new version (so
// that the client will use it whenever it is available). Removal of an API
// server version can be done when any prior releases are no longer supported.
// Normally, this can be done at major release, although additional thought around
// FullStatus (client facade) and Migration (controller facade) is needed.
// New facades should start at 1.
// We no longer support facade versions at 0.
var facadeVersions = facades.FacadeVersions{
	"Action":                       {7},
	"Agent":                        {3},
	"AgentLifeFlag":                {1},
	"AgentTools":                   {1},
	"Annotations":                  {2},
	"Application":                  {19, 20},
	"ApplicationOffers":            {5, 6},
	"Backups":                      {3},
	"Block":                        {2},
	"Bundle":                       {8},
	"CAASAgent":                    {2},
	"CAASAdmission":                {1},
	"CAASApplication":              {1},
	"CAASApplicationProvisioner":   {1},
	"CAASModelConfigManager":       {1},
	"CAASModelOperator":            {1},
	"CAASOperatorUpgrader":         {1},
	"Charms":                       {7},
	"Cleaner":                      {2},
	"Client":                       {8},
	"Cloud":                        {7},
	"Controller":                   {12, 13},
	"CredentialManager":            {1},
	"CredentialValidator":          {2, 3},
	"CrossController":              {1},
	"CrossModelRelations":          {3},
	"CrossModelSecrets":            {1, 2},
	"Deployer":                     {1},
	"DiskManager":                  {2},
	"EntityWatcher":                {2},
	"ExternalControllerUpdater":    {1},
	"FilesystemAttachmentsWatcher": {2},
	"Firewaller":                   {7},
	"HighAvailability":             {2, 3},
	"HostKeyReporter":              {1},
	"ImageMetadata":                {3},
	"ImageMetadataManager":         {1},
	"InstanceMutater":              {3},
	"InstancePoller":               {4},
	"KeyManager":                   {1},
	"KeyUpdater":                   {1},
	"LeadershipService":            {2},
	"Logger":                       {1},
	"MachineActions":               {1},
	"MachineManager":               {11},
	"MachineUndertaker":            {1},
	"Machiner":                     {5, 6},
	"MigrationFlag":                {1},
	"MigrationMaster":              {4, 5},
	"MigrationMinion":              {1},
	"MigrationStatusWatcher":       {1},
	"MigrationTarget":              {4, 5},
	"ModelConfig":                  {3, 4},
	"ModelManager":                 {9, 10, 11},
	"ModelSummaryWatcher":          {1},
	"ModelUpgrader":                {1},
	"NotifyWatcher":                {1},
	"OfferStatusWatcher":           {1},
	"Pinger":                       {1},
	"Provisioner":                  {11},
	"ProxyUpdater":                 {2},
	"Reboot":                       {2},
	"RelationStatusWatcher":        {1},
	"RelationUnitsWatcher":         {1},
	"RemoteRelations":              {2},
	"RemoteRelationWatcher":        {1},
	"Resources":                    {3},
	"ResourcesHookContext":         {1},
	"RetryStrategy":                {1},
	"SecretsTriggerWatcher":        {1},
	"SecretBackends":               {1},
	"SecretBackendsManager":        {1},
	"SecretBackendsRotateWatcher":  {1},
	"SecretsRevisionWatcher":       {1},
	"Secrets":                      {1, 2},
	"SecretsManager":               {3},
	"SecretsDrain":                 {1},
	"UserSecretsDrain":             {1},
	"UserSecretsManager":           {1},
	"Spaces":                       {6},
	"SSHClient":                    {4, 5},
	"Storage":                      {6},
	"StorageProvisioner":           {4},
	"StringsWatcher":               {1},
	"Subnets":                      {5},
	"Undertaker":                   {1},
	"UnitAssigner":                 {1},
	"Uniter":                       {19, 20, 21},
	"Upgrader":                     {1},
	"UserManager":                  {3},
	"VolumeAttachmentsWatcher":     {2},
	"VolumeAttachmentPlansWatcher": {1},

	// Technically we don't require this facade in the client, as it is only
	// used by the agent. Yet the migration checks will use this to verify
	// that the controller is capable of handling the migration.
	"PayloadsHookContext": {1, 2},
}
