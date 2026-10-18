package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cdobratz/azguard/internal/account"
	"github.com/cdobratz/azguard/internal/config"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var (
		azureKind  string
		awsKind    string
		spendLimit string
		azCredit   string
		azExpires  string
		awsCredit  string
		awsExpires string
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Record Azure and AWS account type once",
		Long: `Record whether the Azure subscription is free or Pay-As-You-Go, and whether the
AWS account is on the Free plan or the Paid plan.

Azure can also be detected from 'az account show' at status time. AWS has no
equivalent read in this CLI, so the answer is stored here. Running init again
does not ask when an answer is already stored, unless you pass --force or a flag.

Cloud secrets are not written. Use environment variables or the OS keychain.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			account := cfg.Account
			if !force && account.Azure != "" && azureKind == "" {
				azureKind = account.Azure
			}
			if !force && account.AWS != "" && awsKind == "" {
				awsKind = account.AWS
			}
			var err error
			if azureKind == "" {
				azureKind, err = ask("Azure account type (free or payg): ")
				if err != nil {
					return err
				}
			}
			if awsKind == "" {
				awsKind, err = ask("AWS plan (free or paid): ")
				if err != nil {
					return err
				}
			}
			azureKind = strings.ToLower(strings.TrimSpace(azureKind))
			awsKind = strings.ToLower(strings.TrimSpace(awsKind))
			switch azureKind {
			case "free", "payg":
			default:
				return fmt.Errorf("--azure must be free or payg")
			}
			switch awsKind {
			case "free", "paid":
			default:
				return fmt.Errorf("--aws must be free or paid")
			}
			if spendLimit == "" && !force {
				spendLimit = account.AWSSpendLimit
			}
			next := config.AccountConfig{
				Azure:                azureKind,
				AWS:                  awsKind,
				AWSSpendLimit:        strings.ToLower(strings.TrimSpace(spendLimit)),
				AzureCreditRemaining: firstNonEmpty(azCredit, account.AzureCreditRemaining),
				AzureCreditExpires:   firstNonEmpty(azExpires, account.AzureCreditExpires),
				AWSCreditRemaining:   firstNonEmpty(awsCredit, account.AWSCreditRemaining),
				AWSCreditExpires:     firstNonEmpty(awsExpires, account.AWSCreditExpires),
			}
			if err := config.WriteAccount("", next); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved account type: Azure %s, AWS %s\n", azureKind, awsKind)
			return nil
		},
	}
	cmd.Flags().StringVar(&azureKind, "azure", "", "Azure account type: free or payg")
	cmd.Flags().StringVar(&awsKind, "aws", "", "AWS plan: free or paid")
	cmd.Flags().StringVar(&spendLimit, "aws-spend-limit", "", "Whether an AWS spend limit exists: yes or no")
	cmd.Flags().StringVar(&azCredit, "azure-credit", "", "Azure credit remaining, if you know it")
	cmd.Flags().StringVar(&azExpires, "azure-credit-expires", "", "Azure credit expiry date")
	cmd.Flags().StringVar(&awsCredit, "aws-credit", "", "AWS credit remaining, if you know it")
	cmd.Flags().StringVar(&awsExpires, "aws-credit-expires", "", "AWS credit expiry date")
	cmd.Flags().BoolVar(&force, "force", false, "Replace answers already stored by init")
	return cmd
}

func ask(prompt string) (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		return "", fmt.Errorf("pass --azure free|payg and --aws free|paid (stdin is not a terminal)")
	}
	fmt.Fprint(os.Stderr, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func azureProfile(ctx context.Context) account.Profile {
	stored := account.AzureFromConfig(cfg.Account.Azure, cfg.Account.AzureCreditRemaining, cfg.Account.AzureCreditExpires)
	live, err := account.LookupAzure(ctx)
	if err != nil || live.Kind == "" {
		return stored
	}
	if live.CreditRemaining == nil {
		live.CreditRemaining = stored.CreditRemaining
		live.CreditExpires = stored.CreditExpires
	}
	return live
}

func awsProfile() account.Profile {
	return account.AWSFromConfig(cfg.Account.AWS, cfg.Account.AWSSpendLimit, cfg.Account.AWSCreditRemaining, cfg.Account.AWSCreditExpires)
}
