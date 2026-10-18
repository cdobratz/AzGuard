package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	awscloud "github.com/azguard/azguard/internal/cloud/aws"
	"github.com/azguard/azguard/internal/cloud/azure"
	"github.com/azguard/azguard/internal/events"
	"github.com/azguard/azguard/internal/pricing"
	"github.com/azguard/azguard/internal/watch"
	"github.com/spf13/cobra"
)

func watchCmd() *cobra.Command {
	var (
		interval      time.Duration
		lookback      time.Duration
		once          bool
		webhook       string
		desktop       bool
		cloud         string
		region        string
		allRegions    bool
		statePath     string
		webhookSchema bool
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Poll for new billable resources and alert",
		Long: `Poll the Azure Activity Log and AWS CloudTrail LookupEvents for new billable resources.

The default interval is 15 minutes. Azure Activity Log records usually appear within a few minutes.
CloudTrail LookupEvents usually returns an event within about 15 minutes, and it can take longer.
An alert arrives after that provider delay plus at most one poll interval. This is not a live stream.

Each alert names the creating principal, an estimated monthly list price from the bundled table
(730 hours), and whether the principal looks like automation (a service principal or an access key,
rather than a human user or IAM Identity Center role).

Alerts print to the terminal. Desktop notifications are on unless you pass --desktop=false.
--webhook posts one Slack- and Discord-compatible JSON body per poll that finds something new.

  azguard watch
  azguard watch --once --interval 15m
  azguard watch --webhook https://hooks.slack.com/services/...
  azguard watch --webhook-schema

Read-only permissions: the Azure Reader role includes
Microsoft.Insights/eventtypes/values/read. On AWS, cloudtrail:LookupEvents is enough for this command.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if webhookSchema {
				fmt.Fprint(cmd.OutOrStdout(), watch.WebhookSchema)
				return nil
			}
			switch cloud {
			case "all", "azure", "aws":
			default:
				return fmt.Errorf("--cloud must be all, azure, or aws")
			}
			if statePath == "" {
				statePath = defaultWatchStatePath()
			}
			sources, notes := watchSources(cloud, region, allRegions)
			for _, note := range notes {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(sources) == 0 {
				return fmt.Errorf("no cloud credentials are configured for watch")
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return watch.Run(ctx, watch.Options{
				Interval:  interval,
				Lookback:  lookback,
				Once:      once,
				Webhook:   webhook,
				Desktop:   desktop,
				StatePath: statePath,
				Sources:   sources,
				Out:       cmd.OutOrStdout(),
				Err:       cmd.ErrOrStderr(),
			})
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", watch.DefaultInterval, "How often to poll")
	cmd.Flags().DurationVar(&lookback, "lookback", 0, "How far back each poll looks (default: the interval)")
	cmd.Flags().BoolVar(&once, "once", false, "Poll one time and exit")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Slack or Discord incoming webhook URL")
	cmd.Flags().BoolVar(&desktop, "desktop", true, "Show a desktop notification for each new resource")
	cmd.Flags().StringVar(&cloud, "cloud", "all", "Which cloud to poll: all, azure, or aws")
	cmd.Flags().StringVar(&region, "region", "", "AWS region to poll (default: configured region)")
	cmd.Flags().BoolVar(&allRegions, "all-regions", false, "Poll every enabled AWS region")
	cmd.Flags().StringVar(&statePath, "state", "", "File of event IDs already alerted (default: ~/.azguard/watch.json)")
	cmd.Flags().BoolVar(&webhookSchema, "webhook-schema", false, "Print the webhook JSON shape and exit")
	return cmd
}

func defaultWatchStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".azguard", "watch.json")
}

func watchSources(cloud, regionFlag string, allRegions bool) ([]watch.Source, []string) {
	var sources []watch.Source
	var notes []string
	if cloud == "all" || cloud == "azure" {
		if cfg.Azure.SubscriptionID == "" {
			notes = append(notes, "Azure subscription is not configured; skipped Azure Activity Log")
		} else if err := azure.ValidateSubscriptionID(cfg.Azure.SubscriptionID); err != nil {
			notes = append(notes, "Azure Activity Log: "+err.Error())
		} else {
			token, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
				"tenant_id":     cfg.Azure.TenantID,
				"client_id":     cfg.Azure.ClientID,
				"client_secret": cfg.Azure.ClientSecret,
			})
			if err != nil {
				notes = append(notes, "Azure auth: "+err.Error())
			} else {
				sources = append(sources, azureWatch{reader: azure.NewReader(cfg.Azure.SubscriptionID, token)})
			}
		}
	}
	if cloud == "all" || cloud == "aws" {
		costClient := awscloud.NewCostClient(cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SessionToken, cfg.AWS.Region)
		reader := awscloud.ReaderFromCost(costClient)
		if regionFlag != "" {
			reader.Region = regionFlag
		}
		if !reader.Enabled() {
			notes = append(notes, "AWS credentials are not configured; skipped CloudTrail")
		} else {
			regions := []string{reader.Region}
			if allRegions {
				listed, err := reader.Regions(context.Background())
				if err != nil {
					notes = append(notes, "aws regions: "+err.Error())
				} else if len(listed) > 0 {
					regions = listed
				}
			}
			sources = append(sources, awsWatch{reader: reader, regions: regions})
		}
	}
	return sources, notes
}

type azureWatch struct {
	reader *azure.Reader
}

func (s azureWatch) Poll(ctx context.Context, since, until time.Time) ([]watch.Alert, error) {
	ev, err := s.reader.ActivityEvents(ctx, since, until)
	if err != nil {
		return nil, err
	}
	return watch.AlertsFrom(ev, pricing.Bundled()), nil
}

type awsWatch struct {
	reader  *awscloud.Reader
	regions []string
}

func (s awsWatch) Poll(ctx context.Context, since, until time.Time) ([]watch.Alert, error) {
	var all []events.Event
	var first error
	for _, region := range s.regions {
		ev, err := s.reader.LookupEvents(ctx, region, since, until)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		all = append(all, ev...)
	}
	return watch.AlertsFrom(all, pricing.Bundled()), first
}
