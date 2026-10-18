package findings

// JSONSchema is the documented schema for `azguard resources --output json`.
// `azguard resources --schema` prints this document.
const JSONSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "azguard.findings.v1",
  "title": "azguard resources report",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema", "schema_version", "generated_at", "findings", "total_estimated_monthly_usd", "currency", "errors"],
  "properties": {
    "schema": {"const": "azguard.findings.v1"},
    "schema_version": {"const": "1"},
    "generated_at": {"type": "string", "format": "date-time", "description": "UTC RFC3339 timestamp."},
    "currency": {"const": "USD"},
    "total_estimated_monthly_usd": {
      "type": "number",
      "description": "Sum of estimated_monthly_usd. If two findings share a provider and resource_id, the higher estimate is counted once."
    },
    "errors": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Provider or rule errors. A partial report can still contain findings."
    },
    "findings": {
      "type": "array",
      "items": {"$ref": "#/$defs/finding"}
    }
  },
  "$defs": {
    "finding": {
      "type": "object",
      "additionalProperties": false,
      "required": ["resource_id", "name", "provider", "region", "rule_id", "title", "evidence", "estimated_monthly_usd", "currency", "created_by", "safe_to_delete", "safe_to_delete_hint", "price_source"],
      "properties": {
        "resource_id": {"type": "string", "description": "ARM resource id or AWS resource id / ARN."},
        "name": {"type": "string"},
        "provider": {"enum": ["azure", "aws"]},
        "region": {"type": "string"},
        "rule_id": {"type": "string", "description": "Stable rule id. See azguard resources --rules."},
        "title": {"type": "string"},
        "evidence": {"type": "string", "description": "Why the rule matched, including the inputs to the cost estimate."},
        "estimated_monthly_usd": {"type": "number", "description": "List-price estimate for a 730-hour month. Not an invoice."},
        "currency": {"const": "USD"},
        "created_by": {"type": "string", "description": "Principal from Azure systemData.createdBy or an AWS CreatedBy/Owner tag. Empty when unknown."},
        "safe_to_delete": {"type": "boolean", "description": "True when nothing is attached. Never means data loss is impossible. azguard resources does not delete."},
        "safe_to_delete_hint": {"type": "string"},
        "price_source": {"enum": ["bundled", "azure-retail", "aws-price-list", "unpriced"], "description": "azure-retail and aws-price-list come from the live price APIs. bundled is the built-in list-price table used when the API is unavailable. unpriced means there is no meter (the resource is free or the size is unknown)."}
      }
    }
  }
}
`
