package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/events"
)

type cloudTrailPage struct {
	Events    []cloudTrailEvent `json:"Events"`
	NextToken string            `json:"NextToken"`
}

type cloudTrailEvent struct {
	EventID         string  `json:"EventId"`
	EventName       string  `json:"EventName"`
	EventTime       awsTime `json:"EventTime"`
	Username        string  `json:"Username"`
	Resources       []ctRes `json:"Resources"`
	CloudTrailEvent string  `json:"CloudTrailEvent"`
}

type ctRes struct {
	ResourceType string `json:"ResourceType"`
	ResourceName string `json:"ResourceName"`
}

type awsTime struct{ time.Time }

func (t *awsTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || string(b) == "" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		parsed, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return err
			}
		}
		t.Time = parsed.UTC()
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	t.Time = time.Unix(int64(n), 0).UTC()
	return nil
}

type trailBody struct {
	EventName         string         `json:"eventName"`
	AWSRegion         string         `json:"awsRegion"`
	ErrorCode         string         `json:"errorCode"`
	UserIdentity      trailIdentity  `json:"userIdentity"`
	RequestParameters map[string]any `json:"requestParameters"`
	ResponseElements  map[string]any `json:"responseElements"`
}

type trailIdentity struct {
	Type        string `json:"type"`
	ARN         string `json:"arn"`
	AccessKeyID string `json:"accessKeyId"`
	UserName    string `json:"userName"`
}

// ParseLookupEvents reads a CloudTrail LookupEvents response.
func ParseLookupEvents(body []byte) ([]events.Event, string, error) {
	var page cloudTrailPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, "", fmt.Errorf("parse LookupEvents: %w", err)
	}
	out := make([]events.Event, 0, len(page.Events))
	for _, rec := range page.Events {
		ev := events.Event{
			ID:       rec.EventID,
			Provider: "aws",
			Name:     rec.EventName,
			At:       rec.EventTime.UTC(),
			Status:   "Succeeded",
			Principal: events.Principal{
				Display: rec.Username,
			},
		}
		if len(rec.Resources) > 0 {
			ev.ResourceID = rec.Resources[0].ResourceName
		}
		if rec.CloudTrailEvent != "" {
			var inner trailBody
			if err := json.Unmarshal([]byte(rec.CloudTrailEvent), &inner); err != nil {
				return nil, "", fmt.Errorf("parse CloudTrailEvent %s: %w", rec.EventID, err)
			}
			if inner.EventName != "" {
				ev.Name = inner.EventName
			}
			ev.Region = inner.AWSRegion
			if inner.ErrorCode != "" {
				ev.Status = inner.ErrorCode
			}
			ev.Principal = events.Principal{
				Display:     firstNonEmpty(inner.UserIdentity.UserName, rec.Username, inner.UserIdentity.ARN),
				Type:        inner.UserIdentity.Type,
				ARN:         inner.UserIdentity.ARN,
				AccessKeyID: inner.UserIdentity.AccessKeyID,
			}
			if id := resourceFromTrail(inner); id != "" {
				ev.ResourceID = id
			}
			ev.Detail = trailDetails(inner)
		}
		out = append(out, ev)
	}
	return out, page.NextToken, nil
}

func resourceFromTrail(inner trailBody) string {
	if id := stringField(inner.ResponseElements, "allocationId", "natGatewayId", "volumeId", "stackId", "loadBalancerArn", "instanceId"); id != "" {
		return id
	}
	if nested := mapField(inner.ResponseElements, "natGateway"); nested != nil {
		if id := stringField(nested, "natGatewayId"); id != "" {
			return id
		}
	}
	if wrapper := mapField(inner.ResponseElements, "CreateNatGatewayResponse"); wrapper != nil {
		if nested := mapField(wrapper, "natGateway"); nested != nil {
			if id := stringField(nested, "natGatewayId"); id != "" {
				return id
			}
		}
	}
	if nested := mapField(inner.ResponseElements, "vpcEndpoint"); nested != nil {
		if id := stringField(nested, "vpcEndpointId"); id != "" {
			return id
		}
	}
	if items := objectItems(inner.ResponseElements, "instancesSet"); len(items) > 0 {
		if id := stringField(items[0], "instanceId"); id != "" {
			return id
		}
	}
	if id := stringField(inner.RequestParameters, "stackName", "dbInstanceIdentifier", "dBInstanceIdentifier"); id != "" {
		return id
	}
	return ""
}

func trailDetails(inner trailBody) map[string]string {
	d := map[string]string{}
	set := func(key, val string) {
		if val != "" && d[key] == "" {
			d[key] = val
		}
	}
	set("instanceType", stringField(inner.RequestParameters, "instanceType"))
	set("volumeType", stringField(inner.RequestParameters, "volumeType"))
	set("vpcEndpointType", stringField(inner.RequestParameters, "vpcEndpointType"))
	if v, ok := numberField(inner.RequestParameters, "size"); ok {
		set("size", strconv.FormatFloat(v, 'f', -1, 64))
	}
	if v, ok := numberField(inner.RequestParameters, "minCount"); ok {
		set("count", strconv.FormatFloat(v, 'f', -1, 64))
	}
	if items := objectItems(inner.RequestParameters, "instancesSet"); len(items) > 0 {
		set("instanceType", stringField(items[0], "instanceType"))
		if v, ok := numberField(items[0], "minCount"); ok {
			set("count", strconv.FormatFloat(v, 'f', -1, 64))
		}
	}
	return d
}

func objectItems(m map[string]any, key string) []map[string]any {
	set := mapField(m, key)
	if set == nil {
		return nil
	}
	raw, ok := set["items"]
	if !ok || raw == nil {
		return nil
	}
	switch t := raw.(type) {
	case []any:
		var out []map[string]any
		for _, item := range t {
			obj, ok := item.(map[string]any)
			if ok {
				out = append(out, obj)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{t}
	default:
		return nil
	}
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		}
	}
	return ""
}

func mapField(m map[string]any, key string) map[string]any {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return obj
}

func numberField(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		n, err := strconv.ParseFloat(t, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// BillableEventNames are the CloudTrail management events watch polls.
// Gateway VPC endpoints are priced out later; the lookup still asks for CreateVpcEndpoint.
var BillableEventNames = []string{
	"RunInstances",
	"CreateNatGateway",
	"AllocateAddress",
	"CreateVpcEndpoint",
	"CreateStack",
	"CreateDBInstance",
	"CreateLoadBalancer",
	"CreateVolume",
}

// LookupEvents reads CloudTrail management events in region between since and until.
// Each billable event name is queried separately so a noisy account does not crowd them out of the first page.
func (r *Reader) LookupEvents(ctx context.Context, region string, since, until time.Time) ([]events.Event, error) {
	var all []events.Event
	for _, name := range BillableEventNames {
		ev, err := r.lookupEventName(ctx, region, since, until, name)
		if err != nil {
			return nil, err
		}
		all = append(all, ev...)
	}
	return all, nil
}

func (r *Reader) lookupEventName(ctx context.Context, region string, since, until time.Time, name string) ([]events.Event, error) {
	token := ""
	var all []events.Event
	for page := 0; page < 20; page++ {
		payload := map[string]any{
			"LookupAttributes": []map[string]string{
				{"AttributeKey": "EventName", "AttributeValue": name},
			},
			"StartTime":  since.UTC().Unix(),
			"EndTime":    until.UTC().Unix(),
			"MaxResults": 50,
		}
		if token != "" {
			payload["NextToken"] = token
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body, err := r.postJSON(ctx, "https://cloudtrail."+region+".amazonaws.com/", region, "cloudtrail", "com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101.LookupEvents", raw)
		if err != nil {
			return nil, fmt.Errorf("LookupEvents %s: %w", name, err)
		}
		ev, next, err := ParseLookupEvents(body)
		if err != nil {
			return nil, err
		}
		all = append(all, ev...)
		if next == "" {
			return all, nil
		}
		token = next
	}
	return nil, fmt.Errorf("LookupEvents %s: stopped after 20 pages", name)
}
