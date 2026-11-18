// Package configs embeds the versioned free-tier limit tables that ship with azguard.
package configs

import _ "embed"

// AzureFreeTierYAML is configs/free_tier_limits.yaml.
//
//go:embed free_tier_limits.yaml
var AzureFreeTierYAML []byte

// AWSFreeTierYAML is configs/aws_free_tier_limits.yaml.
//
//go:embed aws_free_tier_limits.yaml
var AWSFreeTierYAML []byte
