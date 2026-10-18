// Package account describes whether an Azure or AWS account can be charged.
package account

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Kind is one of the four account states status knows about.
type Kind string

const (
	AzureFree Kind = "azure-free"
	AzurePAYG Kind = "azure-payg"
	AWSFree   Kind = "aws-free"
	AWSPaid   Kind = "aws-paid"
)

// Profile is what status prints for one cloud.
type Profile struct {
	Kind            Kind
	SpendingLimit   *bool
	CreditRemaining *float64
	CreditExpires   string
}

// ParseAzureAccount reads `az account show --output json`.
// A spending limit of On, or a free-trial quota, is a free account.
// Pay-As-You-Go (spending limit Off) has no platform cap.
func ParseAzureAccount(body []byte) (Profile, error) {
	var doc struct {
		SubscriptionPolicies struct {
			QuotaID       string `json:"quotaId"`
			SpendingLimit string `json:"spendingLimit"`
		} `json:"subscriptionPolicies"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Profile{}, fmt.Errorf("parse azure account: %w", err)
	}
	quota := strings.ToLower(doc.SubscriptionPolicies.QuotaID)
	limit := strings.ToLower(strings.TrimSpace(doc.SubscriptionPolicies.SpendingLimit))
	p := Profile{}
	switch {
	case limit == "on" || limit == "current" || strings.Contains(quota, "freetrial") || strings.Contains(quota, "student") || strings.Contains(quota, "free"):
		p.Kind = AzureFree
		on := true
		if limit == "off" {
			on = false
		}
		p.SpendingLimit = &on
	case limit == "off" || strings.Contains(quota, "payasyougo"):
		p.Kind = AzurePAYG
		off := false
		p.SpendingLimit = &off
	}
	return p, nil
}

// LookupAzure runs `az account show` and parses it.
func LookupAzure(ctx context.Context) (Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "az", "account", "show", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return Profile{}, err
	}
	return ParseAzureAccount(out)
}

// AzureFromConfig builds a profile from values stored by `azguard init`.
func AzureFromConfig(kind, credit, expires string) Profile {
	return fromConfig(kind, "", credit, expires, true)
}

// AWSFromConfig builds a profile from values stored by `azguard init`.
// spendLimit is "yes", "no", or empty when the user was not asked.
func AWSFromConfig(kind, spendLimit, credit, expires string) Profile {
	return fromConfig(kind, spendLimit, credit, expires, false)
}

func fromConfig(kind, spendLimit, credit, expires string, azure bool) Profile {
	p := Profile{CreditExpires: strings.TrimSpace(expires)}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "free":
		if azure {
			p.Kind = AzureFree
		} else {
			p.Kind = AWSFree
		}
	case "payg", "pay-as-you-go":
		p.Kind = AzurePAYG
	case "paid":
		p.Kind = AWSPaid
	}
	switch strings.ToLower(strings.TrimSpace(spendLimit)) {
	case "yes", "on", "true":
		on := true
		p.SpendingLimit = &on
	case "no", "off", "false":
		off := false
		p.SpendingLimit = &off
	}
	if credit != "" {
		if n, err := strconv.ParseFloat(credit, 64); err == nil {
			p.CreditRemaining = &n
		}
	}
	return p
}

// FormatAzure renders status text. PAYG never includes a percent of a $200 free tier.
func FormatAzure(p Profile, spend float64) string {
	var b strings.Builder
	switch p.Kind {
	case AzurePAYG:
		b.WriteString("⚠ No spending limit: Pay-As-You-Go\n")
		fmt.Fprintf(&b, "Current spend: $%.2f\n", spend)
	case AzureFree:
		b.WriteString("Azure free account\n")
		if p.SpendingLimit != nil && *p.SpendingLimit {
			b.WriteString("Spending limit: on\n")
		}
		fmt.Fprintf(&b, "Current spend: $%.2f\n", spend)
		writeCredit(&b, p)
	default:
		b.WriteString("Azure account type is not set. Run 'azguard init --azure free' or 'azguard init --azure payg'.\n")
		fmt.Fprintf(&b, "Current spend: $%.2f\n", spend)
	}
	return b.String()
}

// DescribeAWS is the plan headline without a spend figure.
func DescribeAWS(p Profile) string {
	var b strings.Builder
	switch p.Kind {
	case AWSPaid:
		b.WriteString("AWS Paid plan\n")
		b.WriteString("Charges apply.\n")
		if p.SpendingLimit != nil && !*p.SpendingLimit {
			b.WriteString("No spend limit is stored for this account.\n")
		} else if p.SpendingLimit != nil && *p.SpendingLimit {
			b.WriteString("A spend limit is stored for this account.\n")
		}
	case AWSFree:
		b.WriteString("AWS Free plan\n")
		b.WriteString("This plan does not charge you unless you upgrade to Paid. See https://aws.amazon.com/free/\n")
		writeCredit(&b, p)
	default:
		b.WriteString("AWS plan is not set. Run 'azguard init --aws free' or 'azguard init --aws paid'.\n")
	}
	return b.String()
}

// FormatAWS renders status text. A Paid plan never includes a percent of a $200 free tier.
func FormatAWS(p Profile, spend float64) string {
	text := DescribeAWS(p)
	return text + fmt.Sprintf("Current spend: $%.2f\n", spend)
}

func writeCredit(b *strings.Builder, p Profile) {
	if p.CreditRemaining == nil && p.CreditExpires == "" {
		return
	}
	if p.CreditRemaining != nil {
		fmt.Fprintf(b, "Credit remaining: $%.2f", *p.CreditRemaining)
		if p.CreditExpires != "" {
			fmt.Fprintf(b, " (expires %s)", p.CreditExpires)
		}
		b.WriteString("\n")
		return
	}
	fmt.Fprintf(b, "Credit expires: %s\n", p.CreditExpires)
}
