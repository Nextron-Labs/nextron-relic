package config

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestSchemaMatchesTypes guards against drift between the Go types and the
// published JSON Schema: every yaml field must be a schema property (and vice
// versa) and enumerations must match the validator.
func TestSchemaMatchesTypes(t *testing.T) {
	var schema map[string]interface{}
	if err := json.Unmarshal(JSONSchema, &schema); err != nil {
		t.Fatal(err)
	}
	defs := schema["$defs"].(map[string]interface{})
	resolve := func(n map[string]interface{}) map[string]interface{} {
		if ref, ok := n["$ref"].(string); ok {
			return defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]interface{})
		}
		return n
	}
	var check func(path string, typ reflect.Type, node map[string]interface{})
	check = func(path string, typ reflect.Type, node map[string]interface{}) {
		node = resolve(node)
		switch typ.Kind() {
		case reflect.Struct:
			props, _ := node["properties"].(map[string]interface{})
			if node["additionalProperties"] != false {
				t.Errorf("%s: additionalProperties must be false", path)
			}
			var goNames, schemaNames []string
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				name := strings.Split(f.Tag.Get("yaml"), ",")[0]
				goNames = append(goNames, name)
				p, ok := props[name].(map[string]interface{})
				if !ok {
					t.Errorf("%s.%s missing in schema", path, name)
					continue
				}
				if f.Type == reflect.TypeOf(Size(0)) || f.Type == reflect.TypeOf(Duration(0)) {
					continue
				}
				check(path+"."+name, f.Type, p)
			}
			for k := range props {
				schemaNames = append(schemaNames, k)
			}
			sort.Strings(goNames)
			sort.Strings(schemaNames)
			if strings.Join(goNames, ",") != strings.Join(schemaNames, ",") {
				t.Errorf("%s: fields differ\n go:     %v\n schema: %v", path, goNames, schemaNames)
			}
		case reflect.Slice:
			if items, ok := node["items"].(map[string]interface{}); ok {
				check(path+"[]", typ.Elem(), items)
			} else {
				t.Errorf("%s: expected array items", path)
			}
		}
	}
	check("", reflect.TypeOf(Config{}), schema)

	props := schema["properties"].(map[string]interface{})
	enum := func(node interface{}) []string {
		n := resolve(node.(map[string]interface{}))
		var out []string
		for _, v := range n["enum"].([]interface{}) {
			out = append(out, v.(string))
		}
		return out
	}
	limits := props["limits"].(map[string]interface{})["properties"].(map[string]interface{})
	output := props["output"].(map[string]interface{})["properties"].(map[string]interface{})
	collect := props["collect"].(map[string]interface{})["properties"].(map[string]interface{})
	for name, pair := range map[string][2][]string{
		"method":      {enum(collect["method"]), ValidMethods},
		"hashes":      {enum(collect["hashes"].(map[string]interface{})["items"]), ValidHashes},
		"oversize":    {enum(limits["oversize"]), ValidOversize},
		"priority":    {enum(limits["priority"]), ValidPriorities},
		"compression": {enum(output["compression"]), ValidCompression},
	} {
		if strings.Join(pair[0], ",") != strings.Join(pair[1], ",") {
			t.Errorf("enum %s: schema %v, validator %v", name, pair[0], pair[1])
		}
	}
}
