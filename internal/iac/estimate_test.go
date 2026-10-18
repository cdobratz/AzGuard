package iac

import (
	"os"
	"testing"

	"github.com/cdobratz/azguard/internal/pricing"
)

func TestTerraformPlanFixture(t *testing.T) {
	body := mustRead(t, "testdata/terraform.json")
	est, err := Parse(body, pricing.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if est.Format != "terraform" || !est.Complete {
		t.Fatalf("estimate = %+v", est)
	}
	// NAT 32.85 + EIP 3.65 + t3.micro 7.59 + gp3 100GB 8.00 + interface endpoint 7.30
	// + gateway 0 + Azure public IP 3.65 + Standard_B1s 7.59
	const want = 32.85 + 3.65 + 7.59 + 8.00 + 7.30 + 3.65 + 7.59
	if est.MonthlyUSD != pricing.RoundUSD(want) {
		t.Fatalf("monthly = %.2f, want %.2f lines=%+v", est.MonthlyUSD, pricing.RoundUSD(want), est.Lines)
	}
	for _, line := range est.Lines {
		if line.Address == "aws_instance.existing" || line.Address == "aws_nat_gateway.old" {
			t.Fatalf("non-create included: %+v", line)
		}
	}
}

func TestPulumiPreviewFixture(t *testing.T) {
	body := mustRead(t, "testdata/pulumi.ndjson")
	est, err := Parse(body, pricing.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if est.Format != "pulumi" || !est.Complete {
		t.Fatalf("estimate = %+v", est)
	}
	if est.MonthlyUSD != pricing.RoundUSD(32.85+3.65) {
		t.Fatalf("monthly = %.2f lines=%+v", est.MonthlyUSD, est.Lines)
	}
}

func TestCloudFormationAndYAML(t *testing.T) {
	body := mustRead(t, "testdata/template.json")
	est, err := Parse(body, pricing.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if est.Format != "cloudformation" || !est.Complete {
		t.Fatalf("estimate = %+v", est)
	}
	if est.MonthlyUSD != pricing.RoundUSD(32.85+7.59) {
		t.Fatalf("json monthly = %.2f lines=%+v", est.MonthlyUSD, est.Lines)
	}
	yamlBody := mustRead(t, "testdata/template.yaml")
	yamlEst, err := Parse(yamlBody, pricing.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if yamlEst.MonthlyUSD != pricing.RoundUSD(8+3.65) {
		t.Fatalf("yaml monthly = %.2f lines=%+v", yamlEst.MonthlyUSD, yamlEst.Lines)
	}
}

func TestCDKManifest(t *testing.T) {
	est, err := EstimateFile("testdata/cdk.out/manifest.json", pricing.Bundled())
	if err != nil {
		t.Fatal(err)
	}
	if est.Format != "cdk" || !est.Complete {
		t.Fatalf("estimate = %+v", est)
	}
	// NAT 32.85 + t3.small 0.0208*730 = 15.184 → 15.18
	if est.MonthlyUSD != pricing.RoundUSD(32.85+15.18) {
		t.Fatalf("monthly = %.2f lines=%+v", est.MonthlyUSD, est.Lines)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
