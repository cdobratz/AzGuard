package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cdobratz/azguard/internal/account"
	awscloud "github.com/cdobratz/azguard/internal/cloud/aws"
	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/cdobratz/azguard/internal/config"
)

func TestStatusCreditLines(t *testing.T) {
	origAzure := readAzureCredit
	origAWS := readAWSCredit
	t.Cleanup(func() {
		readAzureCredit = origAzure
		readAWSCredit = origAWS
		awsCostClient = nil
		cfg = nil
	})
	readAzureCredit = func(context.Context) (azure.CreditBalance, error) {
		return azure.CreditBalance{}, errors.New("no billing account")
	}
	cfg = &config.Config{Account: config.AccountConfig{
		Azure:                "payg",
		AzureCreditRemaining: "25",
		AzureCreditExpires:   "2026-12-01",
	}}
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	stored := account.AzureFromConfig(cfg.Account.Azure, cfg.Account.AzureCreditRemaining, cfg.Account.AzureCreditExpires)
	text := account.FormatAzureAt(attachAzureCredit(context.Background(), stored, stored), 40, now)
	if !strings.Contains(text, "Pay-As-You-Go") || !strings.Contains(text, "25.00") || !strings.Contains(text, "Days left: 13") || !strings.Contains(text, "Credit source: config") {
		t.Fatalf("azure config:\n%s", text)
	}
	if strings.Contains(text, "200") || strings.Contains(text, "% of") {
		t.Fatalf("payg still mentions a fixed grant:\n%s", text)
	}

	readAzureCredit = func(context.Context) (azure.CreditBalance, error) {
		return azure.CreditBalance{Remaining: 70, Expires: time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), Soonest: true}, nil
	}
	apiText := account.FormatAzureAt(attachAzureCredit(context.Background(), stored, stored), 40, now)
	if !strings.Contains(apiText, "70.00") || !strings.Contains(apiText, "azure-consumption") || !strings.Contains(apiText, "soonest") || !strings.Contains(apiText, "Days left: 27") {
		t.Fatalf("azure api:\n%s", apiText)
	}

	cfg.Account.AWS = "paid"
	cfg.Account.AWSCreditRemaining = "15"
	cfg.Account.AWSCreditExpires = "2027-01-15"
	awsCostClient = nil
	awsStored := account.AWSFromConfig(cfg.Account.AWS, "", cfg.Account.AWSCreditRemaining, cfg.Account.AWSCreditExpires)
	paid := account.FormatAWSAt(attachAWSCredit(context.Background(), awsStored), 9, now)
	if !strings.Contains(paid, "Paid plan") || !strings.Contains(paid, "15.00") || !strings.Contains(paid, "not configured") {
		t.Fatalf("aws config:\n%s", paid)
	}
	if strings.Contains(paid, "200") || strings.Contains(paid, "% of") {
		t.Fatalf("paid still mentions a fixed grant:\n%s", paid)
	}

	awsCostClient = &awscloud.CostClient{AccessKey: "AKIATEST", SecretKey: "secret"}
	readAWSCredit = func(context.Context) (awscloud.CreditBalance, error) {
		return awscloud.CreditBalance{Remaining: 8, Expires: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)}, nil
	}
	live := account.DescribeAWSAt(attachAWSCredit(context.Background(), awsStored), now)
	if !strings.Contains(live, "8.00") || !strings.Contains(live, "aws-billing") || !strings.Contains(live, "Days left: 2") {
		t.Fatalf("aws api:\n%s", live)
	}
}
