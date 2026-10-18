package guard

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/cdobratz/azguard/internal/pricing"
)

// Classification is the cost guess for one shell command.
type Classification struct {
	Billable bool
	Priced   bool
	USD      float64
	Summary  string
}

// Classify recognizes billable cloud commands and prices the ones in the bundled table.
// A session budget of zero does not change this result; the caller decides whether to block.
func Classify(command string) Classification {
	parts := splitChain(command)
	var out Classification
	incomplete := false
	var notes []string
	for _, part := range parts {
		one := classifyOne(part)
		if !one.Billable {
			continue
		}
		out.Billable = true
		notes = append(notes, one.Summary)
		if !one.Priced {
			incomplete = true
			continue
		}
		out.USD += one.USD
	}
	if out.Billable && !incomplete && len(notes) > 0 {
		out.Priced = true
	}
	out.USD = pricing.RoundUSD(out.USD)
	out.Summary = strings.Join(notes, "; ")
	return out
}

func classifyOne(command string) Classification {
	fields := tokenize(command)
	fields = skipPrefix(fields)
	if len(fields) == 0 {
		return Classification{}
	}
	switch fields[0] {
	case "az":
		return classifyAzure(fields)
	case "aws":
		return classifyAWS(fields)
	case "terraform":
		if hasToken(fields, "apply") {
			return Classification{Billable: true, Summary: "terraform apply"}
		}
	case "pulumi":
		if hasToken(fields, "up") {
			return Classification{Billable: true, Summary: "pulumi up"}
		}
	case "cdk":
		if hasToken(fields, "deploy") {
			return Classification{Billable: true, Summary: "cdk deploy"}
		}
	}
	return Classification{}
}

func classifyAzure(fields []string) Classification {
	if !hasToken(fields, "create") {
		return Classification{}
	}
	book := pricing.Bundled()
	joined := strings.ToLower(strings.Join(fields, " "))
	switch {
	case strings.Contains(joined, "public-ip"):
		return priced("az public-ip create", book, pricing.KeyAzurePublicIPStandard, "", 1)
	case strings.Contains(joined, "nat") && strings.Contains(joined, "gateway"):
		return priced("az nat gateway create", book, pricing.KeyAzureNATGateway, "", 1)
	case hasToken(fields, "vm"):
		size, _ := flagValue(fields, "--size")
		if size == "" {
			size = "Standard_B1s"
		}
		return priced("az vm create", book, "azure.vm."+strings.ToLower(size), "", 1)
	case hasToken(fields, "disk"):
		return priced("az disk create", book, "azure.disk.hdd.s10", "", 1)
	case hasToken(fields, "plan"):
		return priced("az plan create", book, "azure.plan.b1", "", 1)
	default:
		return Classification{Billable: true, Summary: "az create"}
	}
}

func classifyAWS(fields []string) Classification {
	book := pricing.Bundled()
	for _, f := range fields[1:] {
		switch f {
		case "run-instances":
			instanceType, _ := flagValue(fields, "--instance-type")
			if instanceType == "" {
				instanceType = "t3.micro"
			}
			count := flagFloat(fields, "--count", 1)
			return priced("aws ec2 run-instances", book, pricing.AWSEC2Key(instanceType), "", count)
		case "allocate-address":
			return priced("aws ec2 allocate-address", book, pricing.KeyAWSPublicIPv4, "", 1)
		case "create-nat-gateway":
			return priced("aws ec2 create-nat-gateway", book, pricing.KeyAWSNATGateway, "", 1)
		case "create-volume":
			vol, _ := flagValue(fields, "--volume-type")
			if vol == "" {
				vol = "gp3"
			}
			size := flagFloat(fields, "--size", 8)
			return priced("aws ec2 create-volume", book, pricing.AWSEBSKey(vol), "", size)
		case "create-vpc-endpoint":
			kind, _ := flagValue(fields, "--vpc-endpoint-type")
			if strings.EqualFold(kind, "Gateway") {
				return Classification{Summary: "aws gateway vpc endpoint (no hourly charge)"}
			}
			return priced("aws ec2 create-vpc-endpoint", book, pricing.KeyAWSVPCEndpointHour, "", 1)
		case "create-load-balancer":
			return priced("aws elbv2 create-load-balancer", book, pricing.KeyAWSALBHour, "", 1)
		}
		if strings.HasPrefix(f, "create-") {
			return Classification{Billable: true, Summary: "aws " + f}
		}
	}
	return Classification{}
}

func priced(summary string, book *pricing.Book, key, region string, qty float64) Classification {
	usd, _, ok := book.Monthly(key, region, qty)
	if !ok {
		return Classification{Billable: true, Summary: summary}
	}
	return Classification{Billable: true, Priced: true, USD: usd, Summary: summary}
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
