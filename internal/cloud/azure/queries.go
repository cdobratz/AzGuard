package azure

// Resource Graph queries used by `azguard resources`. Each query is read-only.
// The matching rule also filters the rows, so a recorded fixture can include
// resources that should not become findings.
// tags and createdAt are projected so `azguard cleanup` can honor azguard:keep
// and --older-than without a second inventory.
const (
	QueryUnattachedDisks = `
Resources
| where type =~ 'microsoft.compute/disks'
| extend diskState = tostring(properties.diskState)
| extend sizeGB = todouble(properties.diskSizeGB)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, diskState, sizeGB, skuName, createdBy, createdAt, tags`

	QueryUnassociatedPublicIPs = `
Resources
| where type =~ 'microsoft.network/publicipaddresses'
| where isnull(properties.ipConfiguration)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, skuName, createdBy, createdAt, tags`

	QueryUnattachedNICs = `
Resources
| where type =~ 'microsoft.network/networkinterfaces'
| where isnull(properties.virtualMachine)
| extend hasPrivateEndpoint = isnotnull(properties.privateEndpoint)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, hasPrivateEndpoint, createdBy, createdAt, tags`

	QueryStoppedAllocatedVMs = `
Resources
| where type =~ 'microsoft.compute/virtualmachines'
| extend powerState = tostring(properties.extended.instanceView.powerState.code)
| extend vmSize = tostring(properties.hardwareProfile.vmSize)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, powerState, vmSize, createdBy, createdAt, tags`

	QueryEmptyAppServicePlans = `
Resources
| where type =~ 'microsoft.web/serverfarms'
| extend numberOfSites = toint(properties.numberOfSites)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, skuName, numberOfSites, createdBy, createdAt, tags`

	QuerySnapshots = `
Resources
| where type =~ 'microsoft.compute/snapshots'
| extend sizeGB = todouble(properties.diskSizeGB)
| extend skuName = tostring(sku.name)
| extend timeCreated = tostring(properties.timeCreated)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(properties.timeCreated), tostring(systemData.createdAt), '')
| project id, name, location, skuName, sizeGB, timeCreated, createdBy, createdAt, tags`

	QueryNATGateways = `
Resources
| where type =~ 'microsoft.network/natgateways'
| extend subnetCount = iif(isnull(properties.subnets), 0, array_length(properties.subnets))
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, skuName, subnetCount, createdBy, createdAt, tags`

	QueryWorkspaces = `
Resources
| where type =~ 'microsoft.operationalinsights/workspaces'
| extend skuName = tostring(properties.sku.name)
| extend retentionDays = toint(properties.retentionInDays)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| extend createdAt = coalesce(tostring(systemData.createdAt), '')
| project id, name, location, skuName, retentionDays, createdBy, createdAt, tags`

	QueryWorkspaceSolutions = `
Resources
| where type =~ 'microsoft.operationsmanagement/solutions'
| project workspaceId = tostring(properties.workspaceResourceId)`
)
