package preflight

import (
	"os"
	"strings"
	"testing"
)

func TestCommandFixtures(t *testing.T) {
	vm := mustRead(t, "testdata/az_vm_create.txt")
	res, err := Inspect(string(vm))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Billable || !res.Priced || res.MonthlyUSD != 7.59 {
		t.Fatalf("vm = %+v", res)
	}
	if !strings.Contains(res.Message, "virtual machine (Standard_B1s)") || !strings.Contains(res.Message, "$7.59/month") {
		t.Fatalf("message = %q", res.Message)
	}

	awsCmd := mustRead(t, "testdata/aws_run_instances.txt")
	res, err = Inspect(string(awsCmd))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Priced || res.MonthlyUSD != 7.59 || res.Summary != "aws ec2 run-instances" {
		t.Fatalf("run-instances = %+v", res)
	}

	nat := mustRead(t, "testdata/az_nat_gateway.txt")
	res, err = Inspect(string(nat))
	if err != nil {
		t.Fatal(err)
	}
	const want = "this will create a NAT Gateway at about $32.85/month"
	if res.Message != want {
		t.Fatalf("message = %q", res.Message)
	}
	if res.Note == "" || !strings.Contains(res.Note, "730") {
		t.Fatalf("note = %q", res.Note)
	}
}

func TestTerraformPlanFixture(t *testing.T) {
	body := mustRead(t, "../iac/testdata/terraform.json")
	res, err := Inspect(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "terraform" || !res.Priced || !res.Billable {
		t.Fatalf("plan = %+v", res)
	}
	if res.MonthlyUSD != 70.63 {
		t.Fatalf("monthly = %.2f", res.MonthlyUSD)
	}
	if !strings.Contains(res.Message, "this will create a NAT Gateway at about $32.85/month") {
		t.Fatalf("message = %q", res.Message)
	}
	for _, it := range res.Items {
		if it.Address == "aws_instance.existing" || it.Address == "aws_nat_gateway.old" {
			t.Fatalf("non-create included: %+v", it)
		}
	}
}

func TestInspectWithPlanPricesApply(t *testing.T) {
	body := mustRead(t, "../iac/testdata/terraform.json")
	res, err := InspectWithPlan("terraform apply tfplan", body)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Priced || res.MonthlyUSD != 70.63 || !strings.Contains(res.Summary, "terraform plan") {
		t.Fatalf("apply = %+v", res)
	}
	bare, err := Inspect("terraform apply")
	if err != nil {
		t.Fatal(err)
	}
	if !bare.Billable || bare.Priced || bare.MonthlyUSD != 0 {
		t.Fatalf("bare apply = %+v", bare)
	}
	if !strings.Contains(bare.Message, "no bundled monthly price") {
		t.Fatalf("message = %q", bare.Message)
	}
}

func TestReadOnlyAndGatewayEndpoint(t *testing.T) {
	for _, command := range []string{
		"aws ec2 describe-instances",
		"az group list",
		"terraform plan",
		"pulumi preview",
		"cdk diff",
		"aws ec2 create-vpc-endpoint --vpc-endpoint-type Gateway",
	} {
		res, err := Inspect(command)
		if err != nil {
			t.Fatal(err)
		}
		if res.Billable {
			t.Fatalf("%s: %+v", command, res)
		}
	}
}

func TestKnownCommandPrices(t *testing.T) {
	cases := []struct {
		command string
		usd     float64
	}{
		{"aws ec2 allocate-address", 3.65},
		{"aws ec2 create-nat-gateway", 32.85},
		{"aws ec2 create-volume --volume-type gp3 --size 100", 8},
		{"aws elbv2 create-load-balancer", 16.43},
		{"az network public-ip create -g rg -n pip", 3.65},
		{`bash -lc "aws ec2 run-instances --instance-type t3.micro"`, 7.59},
	}
	for _, tt := range cases {
		res, err := Inspect(tt.command)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Priced || res.MonthlyUSD != tt.usd {
			t.Fatalf("%s: %+v", tt.command, res)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	if _, err := Inspect("  "); err == nil {
		t.Fatal("expected an error")
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
