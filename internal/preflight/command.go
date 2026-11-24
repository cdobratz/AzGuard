package preflight

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/cdobratz/azguard/internal/pricing"
)

type hit struct {
	billable bool
	priced   bool
	usd      float64
	summary  string
	items    []Item
}

func inspectCommand(command string) Result {
	parts := splitChain(command)
	var notes []string
	var items []Item
	incomplete := false
	billable := false
	var usd float64
	for _, part := range parts {
		one := classifyOne(part)
		if !one.billable {
			continue
		}
		billable = true
		if one.summary != "" {
			notes = append(notes, one.summary)
		}
		items = append(items, one.items...)
		if !one.priced {
			incomplete = true
			continue
		}
		usd += one.usd
	}
	res := Result{
		Format:     "command",
		Billable:   billable,
		MonthlyUSD: pricing.RoundUSD(usd),
		Summary:    strings.Join(notes, "; "),
		Items:      items,
	}
	if billable && !incomplete && len(notes) > 0 {
		res.Priced = true
	}
	return res
}

func isIaC(command string) bool {
	for _, part := range splitChain(command) {
		fields := skipPrefix(tokenize(part))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "terraform":
			if hasToken(fields, "apply") {
				return true
			}
		case "pulumi":
			if hasToken(fields, "up") {
				return true
			}
		case "cdk":
			if hasToken(fields, "deploy") {
				return true
			}
		}
	}
	return false
}

func classifyOne(command string) hit {
	fields := skipPrefix(tokenize(command))
	if len(fields) == 0 {
		return hit{}
	}
	switch fields[0] {
	case "az":
		return classifyAzure(fields)
	case "aws":
		return classifyAWS(fields)
	case "terraform":
		if hasToken(fields, "apply") {
			return hit{billable: true, summary: "terraform apply"}
		}
	case "pulumi":
		if hasToken(fields, "up") {
			return hit{billable: true, summary: "pulumi up"}
		}
	case "cdk":
		if hasToken(fields, "deploy") {
			return hit{billable: true, summary: "cdk deploy"}
		}
	}
	return hit{}
}

func classifyAzure(fields []string) hit {
	if !hasToken(fields, "create") {
		return hit{}
	}
	book := pricing.Bundled()
	joined := strings.ToLower(strings.Join(fields, " "))
	switch {
	case strings.Contains(joined, "public-ip"):
		return priced("az public-ip create", "public IP", book, pricing.KeyAzurePublicIPStandard, 1)
	case strings.Contains(joined, "nat") && strings.Contains(joined, "gateway"):
		return priced("az nat gateway create", "NAT Gateway", book, pricing.KeyAzureNATGateway, 1)
	case hasToken(fields, "vm"):
		size, _ := flagValue(fields, "--size")
		if size == "" {
			size = "Standard_B1s"
		}
		return priced("az vm create", "virtual machine ("+size+")", book, pricing.AzureVMKey(size), 1)
	case hasToken(fields, "disk"):
		return priced("az disk create", "managed disk", book, "azure.disk.hdd.s10", 1)
	case hasToken(fields, "plan"):
		return priced("az plan create", "App Service plan", book, "azure.plan.b1", 1)
	default:
		return hit{billable: true, summary: "az create"}
	}
}

func classifyAWS(fields []string) hit {
	book := pricing.Bundled()
	for _, f := range fields[1:] {
		switch f {
		case "run-instances":
			instanceType, _ := flagValue(fields, "--instance-type")
			if instanceType == "" {
				instanceType = "t3.micro"
			}
			count := flagFloat(fields, "--count", 1)
			return priced("aws ec2 run-instances", "EC2 instance ("+instanceType+")", book, pricing.AWSEC2Key(instanceType), count)
		case "allocate-address":
			return priced("aws ec2 allocate-address", "public IP", book, pricing.KeyAWSPublicIPv4, 1)
		case "create-nat-gateway":
			return priced("aws ec2 create-nat-gateway", "NAT Gateway", book, pricing.KeyAWSNATGateway, 1)
		case "create-volume":
			vol, _ := flagValue(fields, "--volume-type")
			if vol == "" {
				vol = "gp3"
			}
			size := flagFloat(fields, "--size", 8)
			return priced("aws ec2 create-volume", "EBS volume", book, pricing.AWSEBSKey(vol), size)
		case "create-vpc-endpoint":
			kind, _ := flagValue(fields, "--vpc-endpoint-type")
			if strings.EqualFold(kind, "Gateway") {
				return hit{summary: "aws gateway vpc endpoint (no hourly charge)"}
			}
			return priced("aws ec2 create-vpc-endpoint", "VPC endpoint", book, pricing.KeyAWSVPCEndpointHour, 1)
		case "create-load-balancer":
			return priced("aws elbv2 create-load-balancer", "load balancer", book, pricing.KeyAWSALBHour, 1)
		}
		if strings.HasPrefix(f, "create-") {
			return hit{billable: true, summary: "aws " + f}
		}
	}
	return hit{}
}

func priced(summary, label string, book *pricing.Book, key string, qty float64) hit {
	usd, _, ok := book.Monthly(key, "", qty)
	item := Item{Kind: key, Label: label}
	if !ok {
		return hit{billable: true, summary: summary, items: []Item{item}}
	}
	item.Priced = true
	item.MonthlyUSD = usd
	return hit{billable: true, priced: true, usd: usd, summary: summary, items: []Item{item}}
}

func splitChain(command string) []string {
	var parts []string
	var b strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if s != "" {
			parts = append(parts, s)
		}
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			b.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			b.WriteByte(c)
		case !inSingle && !inDouble && (c == ';' || c == '\n'):
			flush()
		case !inSingle && !inDouble && c == '&' && i+1 < len(command) && command[i+1] == '&':
			flush()
			i++
		case !inSingle && !inDouble && c == '|' && i+1 < len(command) && command[i+1] == '|':
			flush()
			i++
		default:
			b.WriteByte(c)
		}
	}
	flush()
	if len(parts) == 0 {
		return []string{command}
	}
	return parts
}

func tokenize(command string) []string {
	var out []string
	var b strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range command {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case unicode.IsSpace(r) && !inSingle && !inDouble:
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func skipPrefix(fields []string) []string {
	for i := 0; i < 5; i++ {
		for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
			fields = fields[1:]
		}
		if len(fields) > 0 && fields[0] == "sudo" {
			fields = fields[1:]
			continue
		}
		if len(fields) >= 3 && (fields[0] == "bash" || fields[0] == "sh" || fields[0] == "zsh") && isShellC(fields[1]) {
			fields = tokenize(strings.Join(fields[2:], " "))
			continue
		}
		return fields
	}
	return fields
}

func isShellC(flag string) bool {
	return flag == "-c" || flag == "-lc" || (strings.HasPrefix(flag, "-") && strings.Contains(flag, "c") && !strings.HasPrefix(flag, "--"))
}

func hasToken(fields []string, want string) bool {
	for _, f := range fields {
		if f == want {
			return true
		}
	}
	return false
}

func flagValue(fields []string, name string) (string, bool) {
	for i, f := range fields {
		if f == name && i+1 < len(fields) {
			return fields[i+1], true
		}
		if strings.HasPrefix(f, name+"=") {
			return strings.TrimPrefix(f, name+"="), true
		}
	}
	return "", false
}

func flagFloat(fields []string, name string, fallback float64) float64 {
	v, ok := flagValue(fields, name)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
