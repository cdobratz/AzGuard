package pricing

import "strings"

// Bundled returns list prices used when the retail price APIs are unreachable.
// Region is empty so the prices apply everywhere; a successful API response for
// a specific region overrides the matching key.
//
// The five sample SKUs covered by TestSampleSKUListPrices are:
//   - Standard HDD S10 (128 GB) in eastus: $5.89 / month
//   - Standard static public IPv4: $0.005 / hour
//   - EBS gp3: $0.08 / GB-month (us-east-1)
//   - AWS public IPv4: $0.005 / hour
//   - NAT Gateway: $0.045 / hour
func Bundled() *Book {
	hour := func(key string, usd float64) Item {
		return Item{Key: key, Unit: UnitHour, USD: usd, Source: SourceBundled}
	}
	month := func(key string, usd float64) Item {
		return Item{Key: key, Unit: UnitMonth, USD: usd, Source: SourceBundled}
	}
	gb := func(key string, usd float64) Item {
		return Item{Key: key, Unit: UnitGBMonth, USD: usd, Source: SourceBundled}
	}
	return NewBook([]Item{
		month("azure.disk.hdd.s4", 1.54),
		month("azure.disk.hdd.s6", 3.01),
		month("azure.disk.hdd.s10", 5.89),
		month("azure.disk.hdd.s15", 11.32),
		month("azure.disk.hdd.s20", 21.76),
		month("azure.disk.ssd.e10", 9.60),
		month("azure.disk.premium.p10", 19.71),
		hour(KeyAzurePublicIPStandard, 0.005),
		month(KeyAzurePublicIPBasic, 0),
		hour(KeyAzureNATGateway, 0.045),
		gb(KeyAzureSnapshotGB, 0.05),
		gb(KeyAzureLogAnalyticsGB, 2.30),
		month("azure.plan.f1", 0),
		month("azure.plan.d1", 0),
		hour("azure.plan.b1", 0.018),
		hour("azure.plan.b2", 0.036),
		hour("azure.plan.b3", 0.072),
		hour("azure.plan.s1", 0.10),
		hour("azure.vm.standard_b1s", 0.0104),
		hour("azure.vm.standard_b1ms", 0.0208),
		hour("azure.vm.standard_b2s", 0.0416),

		gb("aws.ebs.gp3", 0.08),
		gb("aws.ebs.gp2", 0.10),
		gb("aws.ebs.io1", 0.125),
		gb("aws.ebs.io2", 0.125),
		gb("aws.ebs.st1", 0.045),
		gb("aws.ebs.sc1", 0.015),
		gb(KeyAWSSnapshotGB, 0.05),
		hour(KeyAWSPublicIPv4, 0.005),
		hour(KeyAWSNATGateway, 0.045),
		hour(KeyAWSVPCEndpointHour, 0.01),
		hour(KeyAWSALBHour, 0.0225),
		hour(KeyAWSNLBHour, 0.0225),
		hour("aws.ec2.t3.micro", 0.0104),
		hour("aws.ec2.t3.small", 0.0208),
		hour("aws.ec2.t3.medium", 0.0416),
		hour("aws.ec2.t2.micro", 0.0116),
		hour("aws.ec2.m5.large", 0.096),
	})
}

// AzureDiskKey maps a managed-disk SKU name and size to a bundled or retail price key.
func AzureDiskKey(sku string, sizeGB float64) string {
	n := diskTierNumber(sizeGB)
	family := "hdd"
	s := strings.ToLower(sku)
	switch {
	case strings.Contains(s, "premium"):
		family = "premium"
	case strings.Contains(s, "standardssd"), strings.Contains(s, "ssd"):
		family = "ssd"
	}
	letter := "s"
	if family == "premium" {
		letter = "p"
	} else if family == "ssd" {
		letter = "e"
	}
	return "azure.disk." + family + "." + letter + n
}

func diskTierNumber(sizeGB float64) string {
	switch {
	case sizeGB <= 32:
		return "4"
	case sizeGB <= 64:
		return "6"
	case sizeGB <= 128:
		return "10"
	case sizeGB <= 256:
		return "15"
	case sizeGB <= 512:
		return "20"
	case sizeGB <= 1024:
		return "30"
	case sizeGB <= 2048:
		return "40"
	default:
		return "50"
	}
}

// AzureVMKey returns the price key for a VM size such as Standard_B1s.
func AzureVMKey(size string) string {
	s := strings.ToLower(strings.TrimSpace(size))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return ""
	}
	return "azure.vm." + s
}

// AzurePlanKey returns the price key for an App Service plan SKU such as B1.
func AzurePlanKey(sku string) string {
	s := strings.ToLower(strings.TrimSpace(sku))
	if s == "" {
		return ""
	}
	return "azure.plan." + s
}

// AWSEBSKey returns the price key for an EBS volume type such as gp3.
func AWSEBSKey(volumeType string) string {
	s := strings.ToLower(strings.TrimSpace(volumeType))
	if s == "" {
		return ""
	}
	return "aws.ebs." + s
}

// AWSEC2Key returns the price key for an EC2 instance type such as t3.micro.
func AWSEC2Key(instanceType string) string {
	s := strings.ToLower(strings.TrimSpace(instanceType))
	if s == "" {
		return ""
	}
	return "aws.ec2." + s
}
