package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agent/agent/internal/cloud/azure"
	"github.com/agent/agent/internal/config"
	"github.com/agent/agent/internal/cost"
	"github.com/agent/agent/internal/executors"
	"github.com/agent/agent/internal/llm"
	"github.com/agent/agent/internal/storage"
	"github.com/agent/agent/internal/tools"
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
	rootCmd.AddCommand(devCmd())

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

	cmd.AddCommand(&cobra.Command{
		Use:   "trend",
		Short: "Show cost trend analysis",
		RunE: func(cmd *cobra.Command, args []string) error {
			trend, err := costSvc.GetTrendAnalysis()
			if err != nil {
				return fmt.Errorf("failed to get trend: %w", err)
			}
			return printTrendAnalysis(trend)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "report",
		Short: "Generate cost report",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := costSvc.GenerateReport()
			if err != nil {
				return fmt.Errorf("failed to generate report: %w", err)
			}
			return printReport(report)
		},
	})

	cmd.AddCommand(alertCmd())

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

		if len(summary.MonthlyBreakdown) > 0 {
			fmt.Println("\nMonthly Breakdown:")
			for _, m := range summary.MonthlyBreakdown {
				fmt.Printf("  %s: $%.2f\n", m.Month, m.TotalCost)
			}
		}
	}
	return nil
}

func printTrendAnalysis(trend *cost.TrendAnalysis) error {
	switch outputFormat {
	case "json":
		b, err := json.MarshalIndent(trend, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	default:
		fmt.Println("\n📈 Cost Trend Analysis")
		fmt.Println("─────────────────────────────")
		fmt.Printf("Current Month:     $%.2f\n", trend.CurrentMonth)
		fmt.Printf("Previous Month:   $%.2f\n", trend.PreviousMonth)
		
		trendIcon := "➡️"
		if trend.Trend == "increasing" {
			trendIcon = "📈"
		} else if trend.Trend == "decreasing" {
			trendIcon = "📉"
		}
		
		fmt.Printf("Change:           %.2f%% %s\n", trend.ChangePercent, trendIcon)
		fmt.Printf("Trend:            %s\n", trend.Trend)
		fmt.Printf("6-Month Average:  $%.2f\n", trend.AverageMonthly)
		fmt.Printf("Next Month Proj: $%.2f\n", trend.Projection)
	}
	return nil
}

func printReport(report *cost.Report) error {
	switch outputFormat {
	case "json":
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	case "csv":
		fmt.Println("month,total_cost,currency")
		for _, m := range report.MonthlyData {
			fmt.Printf("%s,%.2f,%s\n", m.Month, m.TotalCost, m.Currency)
		}
	default:
		fmt.Println("\n📄 Cost Report - "+report.Period)
		fmt.Println("═══════════════════════════════════")
		fmt.Printf("Generated: %s\n", report.GeneratedAt)
		fmt.Printf("Period:    %s\n", report.Period)
		fmt.Printf("\n💰 Total Cost: $%.2f %s\n", report.TotalCost, report.Currency)
		fmt.Printf("📈 Forecast:   $%.2f\n", report.Forecast)
		
		if len(report.TopServices) > 0 {
			fmt.Println("\n🔝 Top Services:")
			for _, s := range report.TopServices {
				fmt.Printf("  %-20s $%.2f\n", s.Service+":", s.Cost)
			}
		}
		
		fmt.Printf("\n📊 Monthly Breakdown:\n")
		for _, m := range report.MonthlyData {
			fmt.Printf("  %s: $%.2f\n", m.Month, m.TotalCost)
		}
	}
	return nil
}

func alertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Manage budget alerts",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			alerts, err := db.GetAlerts()
			if err != nil {
				return err
			}
			if len(alerts) == 0 {
				fmt.Println("No alerts configured")
				return nil
			}
			fmt.Println("\n🔔 Budget Alerts")
			fmt.Println("─────────────────────────────")
			for _, a := range alerts {
				status := "✅ Enabled"
				if !a.Enabled {
					status = "❌ Disabled"
				}
				fmt.Printf("%s - $%.2f (%s)\n", a.Name, a.Threshold, status)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add [name] [threshold]",
		Short: "Add a new budget alert",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var threshold float64
			fmt.Sscanf(args[1], "%f", &threshold)
			alert := storage.Alert{
				Name:      args[0],
				Threshold: threshold,
				Enabled:   true,
			}
			if err := db.SaveAlert(alert); err != nil {
				return err
			}
			fmt.Printf("Alert '%s' created with threshold $%.2f\n", args[0], threshold)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Check current costs against alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			startDate, endDate := cost.GetCurrentMonthDateRange()
			summary, err := costSvc.GetCostSummary(cost.CostFilter{
				StartDate: startDate,
				EndDate:   endDate,
			})
			if err != nil {
				return err
			}

			alerts, err := db.GetAlerts()
			if err != nil {
				return err
			}

			if len(alerts) == 0 {
				fmt.Println("No alerts configured")
				return nil
			}

			fmt.Println("\n🔔 Alert Status")
			fmt.Println("─────────────────────────────")
			fmt.Printf("Current costs: $%.2f\n\n", summary.TotalCost)

			triggered := false
			for _, a := range alerts {
				if !a.Enabled {
					continue
				}
				percent := (summary.TotalCost / a.Threshold) * 100
				status := "✅ OK"
				if summary.TotalCost >= a.Threshold {
					status = "🚨 TRIGGERED"
					triggered = true
				}
				fmt.Printf("%s: $%.2f / $%.2f (%.1f%%) %s\n", 
					a.Name, summary.TotalCost, a.Threshold, percent, status)
			}

			if triggered {
				fmt.Println("\n⚠️  Budget alerts triggered!")
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [name]",
		Short: "Delete an alert",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := db.DeleteAlert(args[0]); err != nil {
				return err
			}
			fmt.Printf("Alert '%s' deleted\n", args[0])
			return nil
		},
	})

	return cmd
}

func devCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Software development tools",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "build [task]",
		Short: "Generate code using AI",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := llm.NewProvider("ollama", cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Anthropic.APIKey)
			if err != nil {
				provider, err = llm.NewProvider("anthropic", "", cfg.Anthropic.Model, cfg.Anthropic.APIKey)
				if err != nil {
					return fmt.Errorf("no LLM provider available: %w", err)
				}
			}

			gen := tools.NewCodeGenerator(provider)

			language, _ := cmd.Flags().GetString("language")
			output, _ := cmd.Flags().GetString("output")

			req := tools.GenerateRequest{
				Language: language,
				Task:     strings.Join(args, " "),
				Path:     output,
			}

			code, err := gen.Generate(req)
			if err != nil {
				return err
			}

			fmt.Println(code)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "review [path]",
		Short: "Review code using AI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := llm.NewProvider("ollama", cfg.Ollama.BaseURL, cfg.Ollama.Model, cfg.Anthropic.APIKey)
			if err != nil {
				provider, err = llm.NewProvider("anthropic", "", cfg.Anthropic.Model, cfg.Anthropic.APIKey)
				if err != nil {
					return fmt.Errorf("no LLM provider available: %w", err)
				}
			}

			reviewer := tools.NewCodeReviewer(provider)

			result, err := reviewer.Review(tools.ReviewRequest{Path: args[0]})
			if err != nil {
				return err
			}

			fmt.Printf("\n📝 Code Review: %s\n", args[0])
			fmt.Println("─────────────────────────────────")
			fmt.Printf("%s\n\n", result.Summary)

			if len(result.Issues) > 0 {
				fmt.Println("Issues found:")
				for _, issue := range result.Issues {
					fmt.Printf("  • %s\n", issue)
				}
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "test [path]",
		Short: "Run tests",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runner := tools.NewTestRunner()

			result, err := runner.Run(args[0])
			if err != nil {
				return err
			}

			if result.Passed {
				fmt.Println("✅ " + result.Summary)
			} else {
				fmt.Println("❌ " + result.Summary)
			}

			if outputFormat == "json" {
				json.NewEncoder(os.Stdout).Encode(result)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "run [command]",
		Short: "Run a command in PowerShell, Bash, or Azure CLI",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell, _ := cmd.Flags().GetString("shell")
			command := strings.Join(args, " ")

			var exec executor.Executor
			switch shell {
			case "powershell", "pwsh":
				exec = executor.NewPowerShellExecutor()
			case "bash", "sh":
				exec = executor.NewBashExecutor()
			case "cmd":
				exec = executor.NewCmdExecutor()
			case "az", "azure":
				exec = executor.NewAzureCLIExecutor()
			default:
				exec = executor.AutoDetectExecutor()
			}

			ctx := context.Background()
			result, err := exec.Execute(ctx, command)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			fmt.Print(result.Output)
			return nil
		},
	})

	cmd.Flags().StringP("language", "l", "python", "Programming language for code generation")
	cmd.Flags().StringP("output", "o", "", "Output file path")
	cmd.Flags().StringP("shell", "s", "", "Shell to use: powershell, bash, az, cmd")

	return cmd
}
