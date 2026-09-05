package llm

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// StrictSchema reflects a Go struct into a JSON schema shaped for OpenAI's
// strict structured-output mode: additionalProperties:false and every
// property listed in required (invopop already does both by default here).
// The one thing it post-processes is invopop's "oneOf" nullable-union
// pattern (from the `jsonschema:"nullable"` tag), rewritten to "anyOf" since
// that's the keyword strict mode documents for optional fields.
func StrictSchema(v any) map[string]any {
	r := &jsonschema.Reflector{DoNotReference: true}

	b, err := json.Marshal(r.Reflect(v))
	if err != nil {
		panic(err) // schema shape is fixed at compile time; a failure here is a programmer error
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}

	delete(m, "$schema")
	rewriteOneOf(m)
	return m
}

func rewriteOneOf(node any) {
	switch n := node.(type) {
	case map[string]any:
		if v, ok := n["oneOf"]; ok {
			n["anyOf"] = v
			delete(n, "oneOf")
		}
		for _, v := range n {
			rewriteOneOf(v)
		}
	case []any:
		for _, v := range n {
			rewriteOneOf(v)
		}
	}
}
