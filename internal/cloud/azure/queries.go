package azure

// Resource Graph queries used by `azguard resources`. Each query is read-only.
// The matching rule also filters the rows, so a recorded fixture can include
// resources that should not become findings.
const (
	QueryUnattachedDisks = `
Resources
| where type =~ 'microsoft.compute/disks'
| extend diskState = tostring(properties.diskState)
| extend sizeGB = todouble(properties.diskSizeGB)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, diskState, sizeGB, skuName, createdBy`

	QueryUnassociatedPublicIPs = `
Resources
| where type =~ 'microsoft.network/publicipaddresses'
| where isnull(properties.ipConfiguration)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, skuName, createdBy`

	QueryUnattachedNICs = `
Resources
| where type =~ 'microsoft.network/networkinterfaces'
| where isnull(properties.virtualMachine)
| extend hasPrivateEndpoint = isnotnull(properties.privateEndpoint)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, hasPrivateEndpoint, createdBy`

	QueryStoppedAllocatedVMs = `
Resources
| where type =~ 'microsoft.compute/virtualmachines'
| extend powerState = tostring(properties.extended.instanceView.powerState.code)
| extend vmSize = tostring(properties.hardwareProfile.vmSize)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, powerState, vmSize, createdBy`

	QueryEmptyAppServicePlans = `
Resources
| where type =~ 'microsoft.web/serverfarms'
| extend numberOfSites = toint(properties.numberOfSites)
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, skuName, numberOfSites, createdBy`

	QuerySnapshots = `
Resources
| where type =~ 'microsoft.compute/snapshots'
| extend sizeGB = todouble(properties.diskSizeGB)
| extend skuName = tostring(sku.name)
| extend timeCreated = tostring(properties.timeCreated)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, skuName, sizeGB, timeCreated, createdBy`

	QueryNATGateways = `
Resources
| where type =~ 'microsoft.network/natgateways'
| extend subnetCount = iif(isnull(properties.subnets), 0, array_length(properties.subnets))
| extend skuName = tostring(sku.name)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, skuName, subnetCount, createdBy`

	QueryWorkspaces = `
Resources
| where type =~ 'microsoft.operationalinsights/workspaces'
| extend skuName = tostring(properties.sku.name)
| extend retentionDays = toint(properties.retentionInDays)
| extend createdBy = coalesce(tostring(systemData.createdBy), '')
| project id, name, location, skuName, retentionDays, createdBy`

	QueryWorkspaceSolutions = `
Resources
| where type =~ 'microsoft.operationsmanagement/solutions'
| project workspaceId = tostring(properties.workspaceResourceId)`
)
