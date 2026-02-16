package aws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Signer struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

func NewSigner(accessKey, secretKey, sessionToken string) *Signer {
	return &Signer{
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
	}
}

func (s *Signer) Sign(req *http.Request, body []byte) {
	region := "us-east-1"
	service := "ce"
	amzDate := time.Now().UTC().Format("20060102T150405Z")
	dateStamp := time.Now().UTC().Format("20060102")

	req.Header.Set("X-Amz-Date", amzDate)

	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	canonicalQueryString := req.URL.Query().Encode()

	payloadHash := sha256Hash(string(body))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\nx-amz-target:%s\n",
		req.Header.Get("Content-Type"),
		req.Host,
		payloadHash,
		amzDate,
		req.Header.Get("X-Amz-Target"),
	)

	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date;x-amz-target"

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)

	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)

	hashedCanonicalRequest := sha256Hash(canonicalRequest)

	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		amzDate,
		credentialScope,
		hashedCanonicalRequest,
	)

	signingKey := s.getSignatureKey(dateStamp, region, service)
	signature := hmacSHA256Hex(signingKey, stringToSign)

	authHeader := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		s.AccessKey,
		credentialScope,
		signedHeaders,
		signature,
	)

	req.Header.Set("Authorization", authHeader)

	if s.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", s.SessionToken)
	}
}

func (s *Signer) getSignatureKey(dateStamp, region, service string) []byte {
	kSecret := []byte("AWS4" + s.SecretKey)
	kDate := hmacSHA256(kSecret, dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return kSigning
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	io.WriteString(h, data)
	return h.Sum(nil)
}

func hmacSHA256Hex(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	io.WriteString(h, data)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func createCanonicalQueryString(params map[string]string) string {
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, percentEncode(params[k])))
	}
	return strings.Join(parts, "&")
}

func percentEncode(s string) string {
	var encoded strings.Builder
	for _, c := range s {
		if isUnreserved(c) {
			encoded.WriteRune(c)
		} else {
			encoded.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return encoded.String()
}

func isUnreserved(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}
