package aws

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// signV4 applies AWS Signature Version 4 to req. payload is the exact body bytes.
func signV4(req *http.Request, payload []byte, accessKey, secret, token, region, service string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(payload)

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	if token != "" {
		req.Header.Set("X-Amz-Security-Token", token)
	}

	allowed := []string{"content-type", "host", "x-amz-date", "x-amz-target"}
	if token != "" {
		allowed = append(allowed, "x-amz-security-token")
	}
	var signed []string
	for _, h := range allowed {
		if req.Header.Get(h) != "" || h == "host" {
			signed = append(signed, h)
		}
	}
	sort.Strings(signed)

	var canonHeaders strings.Builder
	for _, h := range signed {
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(strings.TrimSpace(req.Header.Get(h)))
		canonHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(signed, ";")

	canonURI := req.URL.EscapedPath()
	if canonURI == "" {
		canonURI = "/"
	}
	canonical := strings.Join([]string{
		req.Method,
		canonURI,
		req.URL.RawQuery,
		canonHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	signature := hex.EncodeToString(hmacSHA256(signatureKey(secret, dateStamp, region, service), []byte(stringToSign)))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature))
}

func signatureKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
