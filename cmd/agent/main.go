package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/config"
	"github.com/azguard/azguard/internal/cost"
	"github.com/azguard/azguard/internal/storage"
	"github.com/spf13/cobra"
)

var (
	cfg          *config.Config
	db           *storage.DB
	costSvc      *cost.Service
	outputFormat string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "azguard",
		Short: "azguard - Protect against Azure free tier bill shock",
		Long: `One command to make sure your Azure free tier doesn't surprise you with a bill.
		
Examples:
  azguard scan              Scan for free tier overages
  azguard resources        List all resources with status
  azguard budget add 5     Add a $5 budget alert
  azguard watch            Monitor costs daily`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			db, err = storage.New(cfg.Storage.Path)
			if err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}

			tokenProvider, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
				"tenant_id":     cfg.Azure.TenantID,
				"client_id":     cfg.Azure.ClientID,
				"client_secret": cfg.Azure.ClientSecret,
			})
			if err != nil {
				return fmt.Errorf("failed to create token provider: %w", err)
			}

			azureCostClient := azure.NewCostClient(cfg.Azure.SubscriptionID, tokenProvider)
			costSvc = cost.NewService(db, azureCostClient)

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if db != nil {
				return db.Close()
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format: table, json, csv")

	rootCmd.AddCommand(scanCmd())
	rootCmd.AddCommand(watchCmd())
	rootCmd.AddCommand(budgetCmd())
	rootCmd.AddCommand(resourcesCmd())
	rootCmd.AddCommand(cleanupCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(costCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Quick overview of your Azure free tier status",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			summary, err := costSvc.GetCurrentCosts(ctx)
			if err != nil {
				return err
			}

			// Calculate free tier status
			limit := 200.0 // Approximate monthly free tier value in USD
			percentUsed := (summary.TotalCost / limit) * 100

			fmt.Println("\n🛡️  Azure Free Tier Status")
			fmt.Println("═══════════════════════════════")
			fmt.Printf("Subscription: %s\n", cfg.Azure.SubscriptionID)
			fmt.Printf("Current Spend: $%.2f / $%.2f free\n", summary.TotalCost, limit)

			if percentUsed >= 100 {
				fmt.Println("⚠️  Status: OVER LIMIT")
			} else if percentUsed >= 80 {
				fmt.Println("⚠️  Status: WARNING (>80%)")
			} else {
				fmt.Println("✅ Status: OK")
			}

			// Check alerts
			alerts, err := db.GetAlerts()
			if err == nil && len(alerts) > 0 {
				fmt.Printf("\n🔔 Active Alerts: %d\n", len(alerts))
				for _, a := range alerts {
					if a.Enabled {
						triggered := ""
						if summary.TotalCost >= a.Threshold {
							triggered = " (TRIGGERED)"
						}
						fmt.Printf("  • %s: $%.2f%s\n", a.Name, a.Threshold, triggered)
					}
				}
			}

			fmt.Println()
			return nil
		},
	}
}

func scanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan subscription for free tier overages",
		Long: `Audit your subscription against Azure free tier limits.
Shows which services are approaching or exceeding their free allocations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Fetch latest costs
			startDate, endDate := cost.GetCurrentMonthDateRange()
			if err := costSvc.FetchAndStoreCosts(ctx, startDate, endDate); err != nil {
				fmt.Printf("Note: Could not fetch live data: %v\n", err)
			}

			summary, err := costSvc.GetCostSummary(cost.CostFilter{})
			if err != nil {
				return err
			}

			_, err = cost.LoadFreeTierConfig()
			if err != nil {
				return err
			}

			fmt.Println("\n🔍 Azure Free Tier Scan")
			fmt.Println("═══════════════════════════════")

			if len(summary.ByService) == 0 {
				fmt.Println("No costs recorded yet. Run 'azguard fetch' first.")
				return nil
			}

			// Check each service against free tier limits
			limit := 200.0 // Approximate monthly free tier value
			percentUsed := (summary.TotalCost / limit) * 100

			fmt.Printf("\nTotal Spend: $%.2f / $%.2f free tier\n", summary.TotalCost, limit)
			fmt.Printf("Usage: %.1f%%\n\n", percentUsed)

			fmt.Println("By Service:")
			fmt.Println("─────────────────────────────────")

			issuesFound := false
			for service, c := range summary.ByService {
				status := "✅"
				limitAmount := 0.0

				// Map Azure service names to free tier limits
				switch strings.ToLower(service) {
				case "virtual machines", "virtualmachine":
					limitAmount = 0.01 * 750 // B1s VM approximation
				case "storage", "blob storage":
					limitAmount = 0.023 * 5 // 5GB storage
				case "functions", "azure functions":
					limitAmount = 0.0
				case "sql database", "sql":
					limitAmount = 0.0
				case "app service", "appservice":
					limitAmount = 0.05 * 750
				}

				if limitAmount > 0 {
					servicePercent := (c / limitAmount) * 100
					if servicePercent >= 100 {
						status = "❌ OVER"
						issuesFound = true
					} else if servicePercent >= 80 {
						status = "⚠️  WARNING"
						issuesFound = true
					}
				}

				fmt.Printf("%s %-20s $%.2f\n", status, service+":", c)
			}

			if !issuesFound {
				fmt.Println("\n✅ All services within free tier limits!")
			} else {
				fmt.Println("\n⚠️  Some services may have overages. Run 'azguard resources' for details.")
			}
			fmt.Println()
			return nil
		},
	}
}

func watchCmd() *cobra.Command {
	var interval string
	return &cobra.Command{
		Use:   "watch",
		Short: "Continuous monitoring with alerts",
		Long:  `Monitor costs at regular intervals and alert when thresholds are reached.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🛡️  azguard watch - Continuous Monitoring")
			fmt.Println("═══════════════════════════════════════")
			fmt.Println("This feature is coming soon!")
			fmt.Println("For now, use 'azguard status' in a cron job:")
			fmt.Println("  */30 * * * * azguard status")
			_ = interval
			return nil
		},
	}
}

func budgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Manage budget alerts",
		Long:  `Set up budget alerts to get notified before unexpected charges.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add [amount]",
		Short: "Add a budget alert",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var amount float64
			fmt.Sscanf(args[0], "%f", &amount)

			// Validate amount
			if amount < 1 || amount > 100 {
				return fmt.Errorf("budget amount should be between $1 and $100")
			}

			alert := storage.Alert{
				Name:      fmt.Sprintf("budget-%.0f", amount),
				Threshold: amount,
				Enabled:   true,
			}

			if err := db.SaveAlert(alert); err != nil {
				return err
			}

			fmt.Printf("✅ Budget alert set: $%.2f\n", amount)
			fmt.Println("   You'll be notified when costs exceed this amount.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all budget alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			alerts, err := db.GetAlerts()
			if err != nil {
				return err
			}

			if len(alerts) == 0 {
				fmt.Println("No budget alerts configured.")
				fmt.Println("Use 'azguard budget add 5' to set a $5 budget.")
				return nil
			}

			fmt.Println("\n🔔 Budget Alerts")
			fmt.Println("─────────────────────────────")
			for _, a := range alerts {
				status := "✅ Enabled"
				if !a.Enabled {
					status = "❌ Disabled"
				}
				fmt.Printf("$%.2f - %s\n", a.Threshold, status)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a budget alert",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := db.DeleteAlert(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("✅ Alert '%s' removed\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "presets",
		Short: "Show preset budget options",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := cost.LoadFreeTierConfig()
			if err != nil {
				return err
			}

			fmt.Println("\n💰 Budget Presets")
			fmt.Println("─────────────────────────────")
			for _, preset := range config.Budgets {
				fmt.Printf("  $%-2.0f  %s\n", preset.Amount, preset.Description)
				fmt.Printf("         Run: azguard budget add %.0f\n\n", preset.Amount)
			}
			return nil
		},
	})

	return cmd
}

func resourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resources",
		Short: "List running resources with free tier status",
		Long:  `Show all Azure resources and their free tier status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("\n📋 Azure Resources")
			fmt.Println("═══════════════════════════════")
			fmt.Println("This feature requires Azure CLI integration.")
			fmt.Println("Run: az cli resource list --output table")
			fmt.Println()
			fmt.Println("To check specific resources:")
			fmt.Println("  az vm list -o table")
			fmt.Println("  az storage account list -o table")
			fmt.Println("  az functionapp list -o table")
			return nil
		},
	}
}

func cleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Interactive cleanup of orphaned resources",
		Long:  `Help identify and remove unused resources to prevent charges.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("\n🧹 Resource Cleanup Guide")
			fmt.Println("═══════════════════════════════")
			fmt.Println("Common resources to check for cleanup:")
			fmt.Println()
			fmt.Println("1. Stop unused Virtual Machines:")
			fmt.Println("   az vm stop --name <vm-name> --resource-group <rg>")
			fmt.Println()
			fmt.Println("2. Delete unused storage accounts:")
			fmt.Println("   az storage account delete --name <storage-name>")
			fmt.Println()
			fmt.Println("3. Remove unused app services:")
			fmt.Println("   az webapp delete --name <app-name> --resource-group <rg>")
			fmt.Println()
			fmt.Println("4. Check for orphaned disks:")
			fmt.Println("   az disk list -o table")
			return nil
		},
	}
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("\n⚙️  azguard Configuration")
			fmt.Println("═══════════════════════════════")
			fmt.Printf("Azure Subscription: %s\n", cfg.Azure.SubscriptionID)
			fmt.Printf("Auth Method: %s\n", cfg.Azure.AuthMethod)
			fmt.Printf("Storage Path: %s\n", cfg.Storage.Path)
			fmt.Println()
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := "azure." + args[0]
			if args[0] == "subscription" {
				key = "azure.subscription_id"
			}
			if args[0] == "auth" {
				key = "azure.auth_method"
			}
			return db.SetConfig(key, args[1])
		},
	})

	return cmd
}

func costCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Advanced cost management",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Show current month costs",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			summary, err := costSvc.GetCurrentCosts(ctx)
			if err != nil {
				return err
			}
			return printCostSummary(summary)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "fetch",
		Short: "Fetch and store costs from Azure",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			startDate, endDate := cost.GetCurrentMonthDateRange()
			if err := costSvc.FetchAndStoreCosts(ctx, startDate, endDate); err != nil {
				return err
			}
			fmt.Println("✅ Costs fetched and stored")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "Show cost history",
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := costSvc.GetCostHistory(30)
			if err != nil {
				return err
			}
			return printCostSummary(summary)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "forecast",
		Short: "Show cost forecast",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			forecast, err := costSvc.GetForecast(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("Next month forecast: $%.2f (confidence: %s)\n", forecast.NextMonth, forecast.Confidence)
			return nil
		},
	})

	return cmd
}

func printCostSummary(summary *cost.CostSummary) error {
	switch outputFormat {
	case "json":
		b, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	default:
		fmt.Printf("\n📊 Azure Costs - %s\n", summary.Period)
		fmt.Printf("Total: $%.2f %s\n", summary.TotalCost, summary.Currency)

		if len(summary.ByService) > 0 {
			fmt.Println("\nBy Service:")
			for service, c := range summary.ByService {
				fmt.Printf("  %-20s $%.2f\n", service+":", c)
			}
		}
	}
	return nil
}
