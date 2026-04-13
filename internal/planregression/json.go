package planregression

import "encoding/json"

// decodeJSON is a thin wrapper around encoding/json for plan-parsing.
// It exists so the plan.go file does not have to import encoding/json
// and so tests can substitute a fake decoder if they ever need to.
func decodeJSON(raw []byte, out interface{}) error {
	return json.Unmarshal(raw, out)
}
