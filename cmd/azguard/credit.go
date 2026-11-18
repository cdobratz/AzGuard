package main

import (
	"context"

	"github.com/cdobratz/azguard/internal/account"
	awscloud "github.com/cdobratz/azguard/internal/cloud/aws"
	"github.com/cdobratz/azguard/internal/cloud/azure"
)

const (
	azureCreditConfigNote = "Azure credit was not read from Microsoft.Consumption lots. That read-only API needs a Microsoft Customer Agreement billing account (Microsoft.Billing/billingAccounts/read). Free trial credit is not exposed on the subscription. Showing the amount stored by azguard init."
	awsCreditConfigNote   = "AWS credit was not read from the Billing GetCredits API (billing:GetCredits, read-only). Showing the amount stored by azguard init."
	awsCreditNoCredsNote  = "AWS credentials are not configured, so Billing GetCredits was not called. Showing the amount stored by azguard init."
	soonestCreditNote     = "Days left uses the soonest expiration among active credits."
)

var readAzureCredit = func(ctx context.Context) (azure.CreditBalance, error) {
	provider, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
		"tenant_id":     cfg.Azure.TenantID,
		"client_id":     cfg.Azure.ClientID,
		"client_secret": cfg.Azure.ClientSecret,
	})
	if err != nil {
		return azure.CreditBalance{}, err
	}
	return azure.NewCreditClient(provider).Balance(ctx)
}

var readAWSCredit = func(ctx context.Context) (awscloud.CreditBalance, error) {
	return awscloud.BillingFromCost(awsCostClient).RemainingCredit(ctx)
}

func attachAzureCredit(ctx context.Context, base, stored account.Profile) account.Profile {
	bal, err := readAzureCredit(ctx)
	if err != nil {
		return account.ApplyCredit(base, stored, account.APICredit{Note: azureCreditConfigNote})
	}
	return account.ApplyCredit(base, stored, apiCredit(bal.Remaining, bal.ExpiresText(), bal.Soonest, "azure-consumption"))
}

func attachAWSCredit(ctx context.Context, stored account.Profile) account.Profile {
	if awsCostClient == nil || !awsCostClient.IsConfigured() {
		return account.ApplyCredit(stored, stored, account.APICredit{Note: awsCreditNoCredsNote})
	}
	bal, err := readAWSCredit(ctx)
	if err != nil {
		return account.ApplyCredit(stored, stored, account.APICredit{Note: awsCreditConfigNote})
	}
	return account.ApplyCredit(stored, stored, apiCredit(bal.Remaining, bal.ExpiresText(), bal.Soonest, "aws-billing"))
}

func apiCredit(remaining float64, expires string, soonest bool, source string) account.APICredit {
	api := account.APICredit{Available: true, Remaining: remaining, Expires: expires, Source: source}
	if soonest {
		api.Note = soonestCreditNote
	}
	return api
}
