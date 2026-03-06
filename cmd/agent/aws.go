package main

import (
	"context"
	"fmt"

	awscloud "github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/cost"
	"github.com/azguard/azguard/internal/storage"
	"github.com/spf13/cobra"
)

var awsCostClient *awscloud.CostClient

func awsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aws",
		Short: "AWS free tier monitoring and alerts",
		Long: `Monitor your AWS free tier usage and get alerts before unexpected charges.

Examples:
  azguard aws status              Check AWS free tier usage
  azguard aws scan                Scan services approaching limits
  azguard aws alerts --threshold 80  Set alert threshold
  azguard aws resources           List free tier eligible services
  azguard aws cost                View AWS cost breakdown`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Run the root PersistentPreRunE first for config/db
			if err := cmd.Root().PersistentPreRunE(cmd, args); err != nil {
				return err
			}

			awsCostClient = awscloud.NewCostClient(
				cfg.AWS.AccessKey,
				cfg.AWS.SecretKey,
				cfg.AWS.SessionToken,
				cfg.AWS.Region,
			)

			if !awsCostClient.IsConfigured() {
				fmt.Println("⚠️  AWS credentials not found.")
				fmt.Println("Configure with: aws configure")
				fmt.Println("Or set: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables")
			}

			return nil
		},
	}

	cmd.AddCommand(awsStatusCmd())
	cmd.AddCommand(awsScanCmd())
	cmd.AddCommand(awsAlertsCmd())
	cmd.AddCommand(awsResourcesCmd())
	cmd.AddCommand(awsCostCmd())

	return cmd
}

func awsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check your AWS free tier usage summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			fmt.Println("\n🛡️  AWS Free Tier Status")
			fmt.Println("═══════════════════════════════")

			if !awsCostClient.IsConfigured() {
				return fmt.Errorf("AWS credentials not configured")
			}

			usages, err := awsCostClient.GetFreeTierUsage(ctx)
			if err != nil {
				fmt.Printf("Note: Could not fetch free tier data: %v\n", err)
				fmt.Println("Falling back to cost-based analysis...")

				// Fallback to Cost Explorer
				startDate, endDate := cost.GetCurrentMonthDateRange()
				result, costErr := awsCostClient.QueryCostsByService(ctx, startDate, endDate)
				if costErr != nil {
					return fmt.Errorf("failed to fetch AWS costs: %w", costErr)
				}

				fmt.Printf("Current Month Spend: $%.2f\n", result.TotalCost)
				if result.TotalCost == 0 {
					fmt.Println("✅ Status: No charges detected")
				} else if result.TotalCost < 1.0 {
					fmt.Println("⚠️  Status: Minor charges detected")
				} else {
					fmt.Println("❌ Status: Charges exceeding free tier")
				}
				fmt.Println()
				return nil
			}

			// Summarize free tier usage
			warningCount := 0
			overCount := 0
			for _, u := range usages {
				if u.PercentUsed >= 100 {
					overCount++
				} else if u.PercentUsed >= 80 {
					warningCount++
				}
			}

			fmt.Printf("Services Tracked: %d\n", len(usages))
			if overCount > 0 {
				fmt.Printf("❌ Over Limit: %d services\n", overCount)
			}
			if warningCount > 0 {
				fmt.Printf("⚠️  Warning: %d services approaching limits\n", warningCount)
			}
			if overCount == 0 && warningCount == 0 {
				fmt.Println("✅ Status: All services within free tier limits")
			}

			// Show alerts
			alerts, err := db.GetAlerts()
			if err == nil && len(alerts) > 0 {
				fmt.Printf("\n🔔 Active Alerts: %d\n", len(alerts))
			}

			fmt.Println()
			return nil
		},
	}
}

func awsScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan AWS services approaching free tier limits",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			fmt.Println("\n🔍 AWS Free Tier Scan")
			fmt.Println("═══════════════════════════════")

			if !awsCostClient.IsConfigured() {
				return fmt.Errorf("AWS credentials not configured")
			}

			usages, err := awsCostClient.GetFreeTierUsage(ctx)
			if err != nil {
				return fmt.Errorf("failed to fetch free tier usage: %w", err)
			}

			if len(usages) == 0 {
				fmt.Println("No free tier usage data found.")
				return nil
			}

			issuesFound := false
			fmt.Println("\nBy Service:")
			fmt.Println("─────────────────────────────────")

			for _, u := range usages {
				status := "✅"
				if u.PercentUsed >= 100 {
					status = "❌ OVER"
					issuesFound = true
				} else if u.PercentUsed >= 80 {
					status = "⚠️  WARN"
					issuesFound = true
				}

				fmt.Printf("%s %-30s %.1f / %.0f %s (%.1f%%)\n",
					status, u.ServiceName+":", u.ActualUsage, u.FreeTierLimit, u.Unit, u.PercentUsed)

				if u.Description != "" {
					fmt.Printf("     %s\n", u.Description)
				}
			}

			if !issuesFound {
				fmt.Println("\n✅ All AWS services within free tier limits!")
			} else {
				fmt.Println("\n⚠️  Some services are approaching or exceeding free tier limits.")
				fmt.Println("Run 'azguard aws resources' for details on each service.")
			}
			fmt.Println()
			return nil
		},
	}
}

func awsAlertsCmd() *cobra.Command {
	var threshold float64

	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Manage AWS free tier alert thresholds",
		RunE: func(cmd *cobra.Command, args []string) error {
			if threshold > 0 {
				// Set alert threshold
				if threshold < 1 || threshold > 100 {
					return fmt.Errorf("threshold must be between 1 and 100 (percent)")
				}

				alert := storage.Alert{
					Name:      fmt.Sprintf("aws-threshold-%.0f", threshold),
					Threshold: threshold,
					Enabled:   true,
				}
				if err := db.SaveAlert(alert); err != nil {
					return err
				}
				fmt.Printf("✅ AWS alert threshold set: %.0f%%\n", threshold)
				fmt.Println("   You'll be notified when any AWS service exceeds this percentage of its free tier limit.")
				return nil
			}

			// List existing alerts
			alerts, err := db.GetAlerts()
			if err != nil {
				return err
			}

			fmt.Println("\n🔔 AWS Free Tier Alerts")
			fmt.Println("─────────────────────────────")

			found := false
			for _, a := range alerts {
				if len(a.Name) >= 3 && a.Name[:3] == "aws" {
					found = true
					status := "✅ Enabled"
					if !a.Enabled {
						status = "❌ Disabled"
					}
					fmt.Printf("  %s: %.0f%% - %s\n", a.Name, a.Threshold, status)
				}
			}

			if !found {
				fmt.Println("No AWS alerts configured.")
				fmt.Println("Use 'azguard aws alerts --threshold 80' to set an alert at 80% usage.")
			}
			return nil
		},
	}

	cmd.Flags().Float64Var(&threshold, "threshold", 0, "Alert at this percentage of free tier limit (1-100)")

	return cmd
}

func awsResourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resources",
		Short: "List AWS free tier eligible services and current usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			fmt.Println("\n📋 AWS Free Tier Resources")
			fmt.Println("═══════════════════════════════")

			if !awsCostClient.IsConfigured() {
				fmt.Println("AWS credentials not configured. Showing known free tier services:")
				printAWSFreeTierServices()
				return nil
			}

			usages, err := awsCostClient.GetFreeTierUsage(ctx)
			if err != nil {
				fmt.Printf("Could not fetch live data: %v\n", err)
				fmt.Println("Showing known free tier services:")
				printAWSFreeTierServices()
				return nil
			}

			for _, u := range usages {
				status := "✅ FREE"
				if u.PercentUsed >= 100 {
					status = "❌ OVER"
				} else if u.PercentUsed >= 80 {
					status = "⚠️  WARN"
				}

				fmt.Printf("\n%s %s\n", status, u.ServiceName)
				fmt.Printf("  Usage: %.1f / %.0f %s (%.1f%%)\n", u.ActualUsage, u.FreeTierLimit, u.Unit, u.PercentUsed)
				if u.ForecastUsage > 0 {
					fmt.Printf("  Forecast: %.1f %s by end of month\n", u.ForecastUsage, u.Unit)
				}
				if u.Description != "" {
					fmt.Printf("  %s\n", u.Description)
				}
			}

			fmt.Println()
			return nil
		},
	}
}

func awsCostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cost",
		Short: "View AWS cost breakdown for current month",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			fmt.Println("\n📊 AWS Cost Breakdown")
			fmt.Println("═══════════════════════════════")

			if !awsCostClient.IsConfigured() {
				return fmt.Errorf("AWS credentials not configured")
			}

			startDate, endDate := cost.GetCurrentMonthDateRange()
			result, err := awsCostClient.QueryCostsByService(ctx, startDate, endDate)
			if err != nil {
				return fmt.Errorf("failed to query AWS costs: %w", err)
			}

			fmt.Printf("Period: %s to %s\n", startDate, endDate)
			fmt.Printf("Total: $%.2f %s\n", result.TotalCost, result.Currency)

			if len(result.Records) > 0 {
				fmt.Println("\nBy Service:")
				fmt.Println("─────────────────────────────────")
				for _, r := range result.Records {
					if r.Cost > 0.001 {
						fmt.Printf("  %-35s $%.4f\n", r.ServiceName+":", r.Cost)
					}
				}
			}

			if result.TotalCost == 0 {
				fmt.Println("\n✅ No charges this month — still within free tier!")
			}

			fmt.Println()
			return nil
		},
	}
}

func printAWSFreeTierServices() {
	services := []struct {
		name  string
		limit string
	}{
		{"Amazon EC2", "750 hrs/month t2.micro (12 months)"},
		{"Amazon S3", "5 GB standard storage (12 months)"},
		{"AWS Lambda", "1M requests/month (always free)"},
		{"Amazon RDS", "750 hrs/month db.t2.micro (12 months)"},
		{"Amazon DynamoDB", "25 GB storage (always free)"},
		{"Amazon CloudFront", "1 TB data transfer/month (always free)"},
		{"Amazon SNS", "1M publishes/month (always free)"},
		{"Amazon SQS", "1M requests/month (always free)"},
		{"Amazon CloudWatch", "10 custom metrics (always free)"},
		{"Amazon API Gateway", "1M REST API calls/month (12 months)"},
		{"Amazon Cognito", "50K MAUs (always free)"},
		{"Amazon EBS", "30 GB SSD storage (12 months)"},
	}

	for _, s := range services {
		fmt.Printf("  %-25s %s\n", s.name, s.limit)
	}

	fmt.Println("\nConfigure AWS credentials to see your actual usage:")
	fmt.Println("  aws configure")
}
