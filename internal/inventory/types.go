// Package inventory holds the normalized read-only records the findings rules evaluate.
// Parsers in the cloud packages produce these from recorded or live API responses.
package inventory

import "time"

// Volume is an EBS volume.
type Volume struct {
	ID              string
	Name            string
	Region          string
	VolumeType      string
	State           string
	CreatedBy       string
	SizeGB          float64
	AttachmentCount int
}

// Address is an Elastic IP allocation.
type Address struct {
	AllocationID string
	PublicIP     string
	Region       string
	CreatedBy    string
	Associated   bool
}

// ENIPublic is a public IPv4 address on a network interface.
type ENIPublic struct {
	ENIID        string
	AllocationID string
	PublicIP     string
	Region       string
	CreatedBy    string
}

// NATGateway is an AWS NAT gateway plus the bytes it processed in the lookback window.
type NATGateway struct {
	ID        string
	Region    string
	State     string
	CreatedBy string
	CreatedAt time.Time
	BytesOut  float64
}

// VPCEndpoint is a VPC endpoint. Gateway endpoints are free and are not findings.
type VPCEndpoint struct {
	ID           string
	Region       string
	CreatedBy    string
	EndpointType string
	ServiceName  string
	State        string
	AZCount      int
}

// Instance is an EC2 instance and the EBS volumes still attached to it.
type Instance struct {
	ID           string
	Name         string
	Region       string
	InstanceType string
	State        string
	CreatedBy    string
	Volumes      []AttachedVolume
}

// AttachedVolume is an EBS volume attached to a stopped instance.
type AttachedVolume struct {
	ID         string
	VolumeType string
	SizeGB     float64
}

// LoadBalancer is a load balancer and its target health.
type LoadBalancer struct {
	ARN       string
	Name      string
	Region    string
	Type      string
	CreatedBy string
	Healthy   int
	Total     int
}

// Snapshot is an EBS or Azure snapshot.
type Snapshot struct {
	ID        string
	Name      string
	Region    string
	SKU       string
	CreatedBy string
	SizeGB    float64
	CreatedAt time.Time
}

// AttachVolumeSizes copies size and type from volumes onto instance attachments.
func AttachVolumeSizes(instances []Instance, volumes []Volume) []Instance {
	byID := make(map[string]Volume, len(volumes))
	for _, v := range volumes {
		byID[v.ID] = v
	}
	out := make([]Instance, len(instances))
	copy(out, instances)
	for i := range out {
		vols := make([]AttachedVolume, len(out[i].Volumes))
		copy(vols, out[i].Volumes)
		for j := range vols {
			if src, ok := byID[vols[j].ID]; ok {
				if vols[j].SizeGB == 0 {
					vols[j].SizeGB = src.SizeGB
				}
				if vols[j].VolumeType == "" {
					vols[j].VolumeType = src.VolumeType
				}
			}
		}
		out[i].Volumes = vols
	}
	return out
}
