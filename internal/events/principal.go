package events

import (
	"regexp"
	"strings"
)

var guidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Automation reports whether the principal looks like automation rather than a human SSO sign-in.
// Access keys and service principals are automation. IAM Identity Center roles (AWSReservedSSO)
// and user UPNs are not.
func Automation(p Principal) (bool, string) {
	idtyp := strings.ToLower(strings.TrimSpace(p.IDTyp))
	if idtyp == "app" || strings.EqualFold(p.Type, "app") {
		return true, "service principal (idtyp=app)"
	}
	if p.AppID != "" && !hasHumanName(p) {
		return true, "service principal (app id)"
	}
	if guidRe.MatchString(strings.TrimSpace(p.Caller)) && !hasHumanName(p) {
		return true, "service principal (object id)"
	}
	switch strings.ToLower(strings.TrimSpace(p.Type)) {
	case "awsservice":
		return true, "AWS service principal"
	case "iamuser":
		if p.AccessKeyID != "" {
			return true, "IAM user access key"
		}
	case "assumedrole":
		blob := strings.ToLower(p.ARN + " " + p.Display)
		if strings.Contains(blob, "awsreservedsso") || strings.Contains(blob, "aws-sso") {
			return false, ""
		}
		return true, "assumed role (not IAM Identity Center SSO)"
	}
	return false, ""
}

func hasHumanName(p Principal) bool {
	return strings.Contains(p.UPN, "@") || strings.Contains(p.Display, "@") || strings.Contains(p.Caller, "@")
}
