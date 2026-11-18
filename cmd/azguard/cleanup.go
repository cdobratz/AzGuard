package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/cleanup"
	awscloud "github.com/cdobratz/azguard/internal/cloud/aws"
	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/cdobratz/azguard/internal/findings"
	"github.com/cdobratz/azguard/internal/pricing"
	"github.com/spf13/cobra"
)

func cleanupCmd() *cobra.Command {
	var (
		cloud         string
		regions       []string
		allRegions    bool
		types         []string
		tags          []string
		older         string
		snapshotDays  int
		apply         bool
		yes           bool
		includeUnsafe bool
	)
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Delete idle and orphaned resources found by azguard resources",
		Long: `Delete forgotten resources that azguard resources already finds.

The default is a dry run. It lists resource id, type, region, age, and estimated
monthly cost, and it does not delete anything. Pass --apply and type yes to delete.
--yes skips the prompt for scripts. --yes without --apply still changes nothing.

A resource tagged azguard:keep=true is never deleted. Values yes and 1 are protected
too. If tags were not loaded, the resource is skipped.

Only findings with safe_to_delete are eligible unless you pass --include-unsafe.
That flag adds stopped VMs, old snapshots, idle NAT gateways, in-use addresses,
load balancers, interface endpoints, and log analytics workspaces. Deleting those
can drop data or change connectivity.

  azguard cleanup
  azguard cleanup --type disk --region eastus --older-than 30d
  azguard cleanup --apply
  azguard cleanup --apply --yes --cloud aws --type ebs --older-than 720h
  azguard cleanup --tag env=dev --output json

Filters: repeat --type, --tag key=value, and --region. --older-than takes 720h or 30d.
Resources younger than that, and resources whose age is unknown, are skipped.
An unassociated Elastic IP is released once even when two rules match it.

Credentials are the same chain as azguard resources: Azure CLI, service principal,
or managed identity, and for AWS the keychain, environment variables, or the AWS CLI
(profiles and SSO). Secrets are not written by this command.

The summary counts would-delete, deleted, skipped, and failed items.
Exit code 1 means a delete failed or discovery failed.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch cloud {
			case "all", "azure", "aws":
			default:
				return fmt.Errorf("--cloud must be all, azure, or aws")
			}
			opt := cleanup.Options{
				Apply:         apply,
				Yes:           yes,
				Types:         types,
				Regions:       regions,
				IncludeUnsafe: includeUnsafe,
				Tags:          map[string]string{},
			}
			if older != "" {
				d, err := cleanup.ParseOlderThan(older)
				if err != nil {
					return err
				}
				opt.OlderThan = d
			}
			for _, tag := range tags {
				key, val, err := cleanup.ParseTagFlag(tag)
				if err != nil {
					return err
				}
				if err := cleanup.PutTag(opt.Tags, key, val); err != nil {
					return err
				}
			}
			opt.Confirm = func(n int) (bool, error) {
				promptOut := cmd.OutOrStdout()
				if outputFormat == "json" || outputFormat == "csv" {
					promptOut = cmd.ErrOrStderr()
				}
				return confirmCleanup(cmd.InOrStdin(), promptOut, n)
			}

			ctx := context.Background()
			scanOpts := findings.Options{SnapshotAge: time.Duration(snapshotDays) * 24 * time.Hour}
			book := pricing.Bundled()
			var found []findings.Finding
			var errs []string
			if cloud == "all" || cloud == "azure" {
				azFound, azErrs := scanAzure(ctx, book, scanOpts)
				found = append(found, azFound...)
				errs = append(errs, azErrs...)
			}
			var awsReader *awscloud.Reader
			if cloud == "all" || cloud == "aws" {
				var awsFound []findings.Finding
				var awsErrs []string
				switch {
				case allRegions:
					awsFound, awsReader, awsErrs = scanAWS(ctx, "", true, book, scanOpts)
					found = append(found, awsFound...)
					errs = append(errs, awsErrs...)
				case len(regions) == 0:
					awsFound, awsReader, awsErrs = scanAWS(ctx, "", false, book, scanOpts)
					found = append(found, awsFound...)
					errs = append(errs, awsErrs...)
				default:
					for _, region := range regions {
						awsFound, awsReader, awsErrs = scanAWS(ctx, region, false, book, scanOpts)
						found = append(found, awsFound...)
						errs = append(errs, awsErrs...)
					}
				}
			}
			if tagErr := attachLoadBalancerTags(ctx, awsReader, found); tagErr != nil {
				errs = append(errs, tagErr.Error())
			}
			if priceErrs := enrichPrices(ctx, found, book, awsReader); len(priceErrs) > 0 {
				for _, e := range priceErrs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "price: %s\n", e)
				}
			}
			found = findings.Reprice(found, book)
			opt.DiscoveryErrors = errs

			del, err := newCleanupDeleter(awsReader)
			if err != nil {
				return err
			}
			return runCleanup(cmd, found, opt, del)
		},
	}
	cmd.Flags().StringVar(&cloud, "cloud", "all", "Which cloud to scan: all, azure, or aws")
	cmd.Flags().StringArrayVar(&regions, "region", nil, "Only this region. Repeat for more than one. For AWS, limits the scan unless --all-regions is set")
	cmd.Flags().BoolVar(&allRegions, "all-regions", false, "Scan every enabled AWS region (DescribeRegions)")
	cmd.Flags().StringArrayVar(&types, "type", nil, "Rule id or short type (disk, ebs, eip, snapshot, nat, nic, plan). Repeat to allow more than one")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Required tag key=value. Repeat to require every tag. azguard:keep=true is always protected")
	cmd.Flags().StringVar(&older, "older-than", "", "Only resources older than this age (720h or 30d). Unknown ages are skipped")
	cmd.Flags().IntVar(&snapshotDays, "snapshot-days", 30, "Discovery threshold for old snapshots, same as azguard resources")
	cmd.Flags().BoolVar(&apply, "apply", false, "Delete the selected resources. Without this flag, cleanup only prints the plan")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the confirmation prompt. Has no effect unless --apply is set")
	cmd.Flags().BoolVar(&includeUnsafe, "include-unsafe", false, "Also delete findings that are not marked safe_to_delete")
	return cmd
}

func runCleanup(cmd *cobra.Command, found []findings.Finding, opt cleanup.Options, del cleanup.Deleter) error {
	w := cmd.OutOrStdout()
	preview := cleanup.Plan(found, opt)
	human := outputFormat == "" || outputFormat == "table"
	if !opt.Apply {
		if err := cleanup.Write(w, preview, outputFormat); err != nil {
			return err
		}
		return preview.Err()
	}
	if human && len(preview.WouldDelete) > 0 {
		if opt.Yes {
			preview.Note = "These resources will be deleted."
		} else {
			preview.Note = "These resources will be deleted if you type yes."
		}
		if err := cleanup.Write(w, preview, outputFormat); err != nil {
			return err
		}
	}
	ok, err := cleanup.Proceed(opt, len(preview.WouldDelete))
	if err != nil {
		return err
	}
	if !ok {
		if len(preview.WouldDelete) > 0 {
			if human {
				if _, err := fmt.Fprintln(w, "Confirmation was not given. Nothing was deleted."); err != nil {
					return err
				}
			} else {
				preview.Note = "Confirmation was not given. Nothing was deleted."
				if err := cleanup.Write(w, preview, outputFormat); err != nil {
					return err
				}
			}
		} else if err := cleanup.Write(w, preview, outputFormat); err != nil {
			return err
		}
		return preview.Err()
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	final := cleanup.Apply(ctx, preview, del)
	if err := cleanup.Write(w, final, outputFormat); err != nil {
		return err
	}
	return final.Err()
}

func confirmCleanup(r io.Reader, w io.Writer, n int) (bool, error) {
	if _, err := fmt.Fprintf(w, "Delete %d resource(s)? Type yes to continue: ", n); err != nil {
		return false, err
	}
	var answer string
	if _, err := fmt.Fscanln(r, &answer); err != nil {
		return false, nil
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

func attachLoadBalancerTags(ctx context.Context, reader *awscloud.Reader, found []findings.Finding) error {
	if reader == nil || !reader.Enabled() {
		return nil
	}
	groups := map[string][]string{}
	for _, f := range found {
		if f.RuleID != "aws.elb.no_healthy_targets" || f.ResourceID == "" {
			continue
		}
		groups[f.Region] = append(groups[f.Region], f.ResourceID)
	}
	if len(groups) == 0 {
		return nil
	}
	loaded := map[string]map[string]string{}
	var errs []string
	for region, arns := range groups {
		got, err := reader.LoadBalancerTags(ctx, region, arns)
		if err != nil {
			errs = append(errs, fmt.Sprintf("aws load balancer tags (%s): %s", region, err.Error()))
			continue
		}
		for arn, tags := range got {
			loaded[arn] = tags
		}
	}
	for i := range found {
		if found[i].RuleID != "aws.elb.no_healthy_targets" {
			continue
		}
		if tags, ok := loaded[found[i].ResourceID]; ok {
			found[i].Tags = tags
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

type providerDeleter struct {
	aws   *awscloud.Reader
	azure *azure.Reader
}

func newCleanupDeleter(awsReader *awscloud.Reader) (cleanup.Deleter, error) {
	var az *azure.Reader
	if cfg != nil && cfg.Azure.SubscriptionID != "" {
		token, err := azure.NewTokenProvider(cfg.Azure.AuthMethod, map[string]string{
			"tenant_id":     cfg.Azure.TenantID,
			"client_id":     cfg.Azure.ClientID,
			"client_secret": cfg.Azure.ClientSecret,
		})
		if err != nil {
			return nil, fmt.Errorf("azure auth: %w", err)
		}
		az = azure.NewReader(cfg.Azure.SubscriptionID, token)
	}
	return &providerDeleter{aws: awsReader, azure: az}, nil
}

func (p *providerDeleter) Delete(ctx context.Context, item cleanup.Item) error {
	switch item.Action {
	case cleanup.ActionAzureDelete:
		if p.azure == nil {
			return fmt.Errorf("azure is not configured")
		}
		return p.azure.DeleteResource(ctx, item.ResourceID)
	case cleanup.ActionAWSDeleteVolume:
		return p.awsDelete(item, func() error { return p.aws.DeleteVolume(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSReleaseAddress:
		return p.awsDelete(item, func() error { return p.aws.ReleaseAddress(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSDeleteNatGateway:
		return p.awsDelete(item, func() error { return p.aws.DeleteNATGateway(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSDeleteVpcEndpoints:
		return p.awsDelete(item, func() error { return p.aws.DeleteVPCEndpoint(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSTerminateInstances:
		return p.awsDelete(item, func() error { return p.aws.TerminateInstance(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSDeleteSnapshot:
		return p.awsDelete(item, func() error { return p.aws.DeleteSnapshot(ctx, item.Region, item.ResourceID) })
	case cleanup.ActionAWSDeleteLoadBalancer:
		return p.awsDelete(item, func() error { return p.aws.DeleteLoadBalancer(ctx, item.Region, item.ResourceID) })
	default:
		return fmt.Errorf("unsupported cleanup action %q", item.Action)
	}
}

func (p *providerDeleter) awsDelete(item cleanup.Item, fn func() error) error {
	if p.aws == nil || !p.aws.Enabled() {
		return fmt.Errorf("aws is not configured")
	}
	if strings.TrimSpace(item.Region) == "" {
		return fmt.Errorf("missing region for %s", item.ResourceID)
	}
	return fn()
}
