package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/agent/agent/internal/cloud/azure"
	"github.com/agent/agent/internal/config"
	"github.com/agent/agent/internal/cost"
	"github.com/agent/agent/internal/storage"
	"github.com/spf13/cobra"
)

var (
	cfg           *config.Config
	db            *storage.DB
	costSvc       *cost.Service
	outputFormat  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent - Build software and track cloud costs",
		Long:  `A CLI tool for software development and cloud cost management.`,
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

	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(costCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "get [key]",
		Short: "Get config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := db.GetConfig(args[0])
			if err != nil {
				return err
			}
			if value == "" {
				value = config.GetString(args[0])
			}
			fmt.Println(value)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return db.SetConfig(args[0], args[1])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all config",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Ollama URL: %s\n", cfg.Ollama.BaseURL)
			fmt.Printf("Ollama Model: %s\n", cfg.Ollama.Model)
			fmt.Printf("Anthropic Model: %s\n", cfg.Anthropic.Model)
			fmt.Printf("Azure Auth: %s\n", cfg.Azure.AuthMethod)
			fmt.Printf("Azure Subscription: %s\n", cfg.Azure.SubscriptionID)
			fmt.Printf("Storage Path: %s\n", cfg.Storage.Path)
			return nil
		},
	})

	return cmd
}

func costCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Manage cloud costs",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Show current month costs",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			summary, err := costSvc.GetCurrentCosts(ctx)
			if err != nil {
				return fmt.Errorf("failed to get current costs: %w", err)
			}
			return printCostSummary(summary)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "Show cost history",
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := costSvc.GetCostHistory(30)
			if err != nil {
				return fmt.Errorf("failed to get cost history: %w", err)
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
				return fmt.Errorf("failed to fetch costs: %w", err)
			}
			fmt.Println("Costs fetched and stored successfully")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "summary",
		Short: "Show cost summary from local storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			startDate, endDate := cost.GetCurrentMonthDateRange()
			summary, err := costSvc.GetCostSummary(cost.CostFilter{
				StartDate: startDate,
				EndDate:   endDate,
			})
			if err != nil {
				return fmt.Errorf("failed to get cost summary: %w", err)
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
				return fmt.Errorf("failed to get forecast: %w", err)
			}
			fmt.Printf("Forecast for next month: $%.2f (confidence: %s)\n", forecast.NextMonth, forecast.Confidence)
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
	case "csv":
		fmt.Println("service,cost")
		for service, c := range summary.ByService {
			fmt.Printf("%s,%.2f\n", service, c)
		}
	default:
		fmt.Printf("\n📊 Azure Costs - %s\n", summary.Period)
		fmt.Println("─────────────────────────────")
		fmt.Printf("Total Cost: $%.2f %s\n\n", summary.TotalCost, summary.Currency)

		if len(summary.ByService) > 0 {
			fmt.Println("By Service:")
			for service, c := range summary.ByService {
				fmt.Printf("  %-20s $%.2f\n", service+":", c)
			}
		}

		if summary.Forecast != nil {
			fmt.Printf("\n📈 Forecast next month: $%.2f\n", summary.Forecast.NextMonth)
		}
	}
	return nil
}
