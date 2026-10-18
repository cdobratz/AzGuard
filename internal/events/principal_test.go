package events

import "testing"

func TestAutomation(t *testing.T) {
	tests := []struct {
		name   string
		p      Principal
		want   bool
		reason string
	}{
		{name: "azure app", p: Principal{IDTyp: "app", AppID: "app-1", Caller: "11111111-2222-3333-4444-555555555555"}, want: true, reason: "service principal (idtyp=app)"},
		{name: "azure object id", p: Principal{Caller: "11111111-2222-3333-4444-555555555555"}, want: true, reason: "service principal (object id)"},
		{name: "azure user", p: Principal{IDTyp: "user", Caller: "casey@contoso.com", UPN: "casey@contoso.com", Display: "casey@contoso.com"}, want: false},
		{name: "iam access key", p: Principal{Type: "IAMUser", AccessKeyID: "AKIAEXAMPLE", Display: "ci-bot"}, want: true, reason: "IAM user access key"},
		{name: "sso role", p: Principal{Type: "AssumedRole", ARN: "arn:aws:sts::123:assumed-role/AWSReservedSSO_Admin_abc/casey", Display: "casey"}, want: false},
		{name: "ci role", p: Principal{Type: "AssumedRole", ARN: "arn:aws:sts::123:assumed-role/gha-deploy/session", Display: "gha-deploy"}, want: true, reason: "assumed role (not IAM Identity Center SSO)"},
		{name: "aws service", p: Principal{Type: "AWSService", Display: "ec2.amazonaws.com"}, want: true, reason: "AWS service principal"},
		{name: "root", p: Principal{Type: "Root", Display: "root"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := Automation(tt.p)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("Automation() = %v %q, want %v %q", got, reason, tt.want, tt.reason)
			}
		})
	}
}
