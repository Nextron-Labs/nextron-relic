package config

import _ "embed"

// JSONSchema is the JSON Schema (draft 2020-12) of configuration version 1.
// It covers structure, types and enumerations; semantic checks (path
// patterns, artifact names, OS restrictions) need Parse/Validate or
// 'relic validate'.
//
//go:embed schema/relic-config-v1.schema.json
var JSONSchema []byte
