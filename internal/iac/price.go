package iac

import (
	"strings"

	"github.com/cdobratz/azguard/internal/pricing"
)

func price(kind, address string, attrs map[string]any, book *pricing.Book) Line {
	line := Line{Address: address, Kind: kind}
	key, qty := priceKey(kind, attrs)
	if key == "" {
		return line
	}
	usd, _, ok := book.Monthly(key, "", qty)
	if !ok {
		return line
	}
	line.Priced = true
	line.MonthlyUSD = usd
	return line
}

func priceKey(kind string, attrs map[string]any) (string, float64) {
	switch kind {
	case "aws.nat", "azure.nat":
		if kind == "aws.nat" {
			return pricing.KeyAWSNATGateway, 1
		}
		return pricing.KeyAzureNATGateway, 1
	case "aws.eip":
		return pricing.KeyAWSPublicIPv4, 1
	case "azure.pip":
		sku := strings.ToLower(strAttr(attrs, "sku", "sku_name", "skuName"))
		if strings.Contains(sku, "basic") {
			return pricing.KeyAzurePublicIPBasic, 1
		}
		return pricing.KeyAzurePublicIPStandard, 1
	case "aws.instance":
		it := strAttr(attrs, "instance_type", "instanceType", "InstanceType")
		if it == "" {
			it = "t3.micro"
		}
		return pricing.AWSEC2Key(it), 1
	case "azure.vm":
		size := strAttr(attrs, "size", "vm_size", "vmSize")
		if size == "" {
			size = "Standard_B1s"
		}
		return pricing.AzureVMKey(size), 1
	case "aws.ebs":
		vt := strAttr(attrs, "type", "volume_type", "volumeType", "VolumeType")
		if vt == "" {
			vt = "gp3"
		}
		size := numAttr(attrs, "size", "Size")
		if size <= 0 {
			size = 8
		}
		return pricing.AWSEBSKey(vt), size
	case "azure.disk":
		sku := strAttr(attrs, "storage_account_type", "storageAccountType")
		size := numAttr(attrs, "disk_size_gb", "diskSizeGb")
		if size <= 0 {
			size = 128
		}
		return pricing.AzureDiskKey(sku, size), 1
	case "azure.snapshot":
		size := numAttr(attrs, "disk_size_gb", "diskSizeGb")
		if size <= 0 {
			size = 32
		}
		return pricing.KeyAzureSnapshotGB, size
	case "azure.plan":
		sku := strAttr(attrs, "sku_name", "skuName", "sku")
		if sku == "" {
			sku = "B1"
		}
		return pricing.AzurePlanKey(sku), 1
	case "aws.endpoint":
		vt := strAttr(attrs, "vpc_endpoint_type", "vpcEndpointType", "VpcEndpointType")
		if strings.EqualFold(vt, "Gateway") {
			return pricing.KeyAWSVPCEndpointHour, 0
		}
		n := listLen(attrs, "subnet_ids", "subnetIds")
		if n == 0 {
			n = 1
		}
		return pricing.KeyAWSVPCEndpointHour, float64(n)
	case "aws.lb":
		lt := strings.ToLower(strAttr(attrs, "load_balancer_type", "loadBalancerType", "Type"))
		if lt == "network" {
			return pricing.KeyAWSNLBHour, 1
		}
		return pricing.KeyAWSALBHour, 1
	default:
		return "", 0
	}
}

func kindTerraform(typ string) string {
	switch typ {
	case "aws_nat_gateway":
		return "aws.nat"
	case "aws_eip":
		return "aws.eip"
	case "aws_instance":
		return "aws.instance"
	case "aws_ebs_volume":
		return "aws.ebs"
	case "aws_vpc_endpoint":
		return "aws.endpoint"
	case "aws_lb", "aws_alb":
		return "aws.lb"
	case "azurerm_public_ip":
		return "azure.pip"
	case "azurerm_nat_gateway":
		return "azure.nat"
	case "azurerm_linux_virtual_machine", "azurerm_windows_virtual_machine", "azurerm_virtual_machine":
		return "azure.vm"
	case "azurerm_managed_disk":
		return "azure.disk"
	case "azurerm_snapshot":
		return "azure.snapshot"
	case "azurerm_service_plan", "azurerm_app_service_plan":
		return "azure.plan"
	default:
		return ""
	}
}

func kindPulumi(typ string) string {
	t := strings.ToLower(typ)
	switch {
	case strings.Contains(t, "natgateway"):
		if strings.Contains(t, "azure") {
			return "azure.nat"
		}
		return "aws.nat"
	case strings.Contains(t, "eip"), strings.Contains(t, "publicip"):
		if strings.Contains(t, "azure") {
			return "azure.pip"
		}
		return "aws.eip"
	case strings.Contains(t, "instance"):
		return "aws.instance"
	case strings.Contains(t, "volume"):
		return "aws.ebs"
	case strings.Contains(t, "vpcendpoint"):
		return "aws.endpoint"
	case strings.Contains(t, "loadbalancer"):
		return "aws.lb"
	case strings.Contains(t, "linuxvirtualmachine"), strings.Contains(t, "virtualmachine"):
		return "azure.vm"
	default:
		return ""
	}
}

func kindCloudFormation(typ string) string {
	switch typ {
	case "AWS::EC2::NatGateway":
		return "aws.nat"
	case "AWS::EC2::EIP":
		return "aws.eip"
	case "AWS::EC2::Instance":
		return "aws.instance"
	case "AWS::EC2::Volume":
		return "aws.ebs"
	case "AWS::EC2::VPCEndpoint":
		return "aws.endpoint"
	case "AWS::ElasticLoadBalancingV2::LoadBalancer":
		return "aws.lb"
	default:
		return ""
	}
}
