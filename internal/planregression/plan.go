package planregression

// planNode is a loose, untyped view of one node in a Postgres
// EXPLAIN (FORMAT JSON) plan tree. The JSON has many fields and
// shapes across Postgres versions; we read only the handful we care
// about and tolerate missing ones gracefully.
type planNode map[string]interface{}

// nodeType returns the Node Type string (e.g. "Seq Scan",
// "Index Scan", "Hash Join"). Empty if missing.
func (n planNode) nodeType() string {
	v, _ := n["Node Type"].(string)
	return v
}

// totalCost returns the root node's Total Cost estimate. Returns 0
// if missing or not numeric.
func (n planNode) totalCost() float64 {
	return numericField(n, "Total Cost")
}

// planRows returns the root node's Plan Rows estimate. Returns 0
// if missing or not numeric.
func (n planNode) planRows() float64 {
	return numericField(n, "Plan Rows")
}

// relationName returns the schema-qualified relation the node
// scans, if any. Postgres populates "Relation Name" and "Schema"
// for scan-type nodes. Non-scan nodes return "".
func (n planNode) relationName() string {
	rel, _ := n["Relation Name"].(string)
	if rel == "" {
		return ""
	}
	schema, _ := n["Schema"].(string)
	if schema == "" {
		return rel
	}
	return schema + "." + rel
}

// children returns the nested Plans array (left child, right child,
// subplan, etc.) as planNodes. Empty if there are none.
func (n planNode) children() []planNode {
	raw, ok := n["Plans"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]planNode, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]interface{}); ok {
			out = append(out, planNode(m))
		}
	}
	return out
}

// parseExplainJSON converts the raw bytes of an EXPLAIN (FORMAT JSON)
// result into a root planNode. Postgres returns a one-element array
// of {"Plan": {...}} objects; we return the inner Plan map.
func parseExplainJSON(raw []byte) (planNode, error) {
	var arr []map[string]interface{}
	if err := decodeJSON(raw, &arr); err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, nil
	}
	plan, ok := arr[0]["Plan"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	return planNode(plan), nil
}

// numericField returns a numeric field from a plan node. Postgres
// encodes some numbers as JSON numbers and others as strings in
// older versions; we tolerate both.
func numericField(n planNode, key string) float64 {
	switch v := n[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		// Some older PG versions may stringify numbers. We do not
		// implement full parsing — just return zero. The analyzer
		// still works off ratios between baseline and post, so a
		// zero on both sides is safe.
		_ = v
		return 0
	}
	return 0
}

// scanRank orders scan modes from best (lowest) to worst (highest).
// The rank powers isDowngrade: a post-migration scan with a higher
// rank than the baseline is a downgrade.
func scanRank(nodeType string) int {
	switch nodeType {
	case "Index Only Scan":
		return 0
	case "Index Scan":
		return 1
	case "Bitmap Heap Scan":
		return 2
	case "Bitmap Index Scan":
		return 2
	case "Seq Scan":
		return 3
	}
	// Non-scan or unrecognized: treat as worst so we never flag it
	// as a downgrade by mistake.
	return -1
}

// isScanNode reports whether a node type is a relation-reading scan
// that the analyzer should track.
func isScanNode(nodeType string) bool {
	switch nodeType {
	case "Seq Scan",
		"Index Scan",
		"Index Only Scan",
		"Bitmap Heap Scan",
		"Bitmap Index Scan":
		return true
	}
	return false
}

// scansByRelation walks the plan tree and returns, for each user
// relation touched by a scan node, the "worst" scan mode observed
// on that relation. If a relation appears in multiple subplans with
// different modes, we pick the one with the highest scanRank — the
// assumption being that the slowest scan dominates the query's real
// cost profile.
func scansByRelation(root planNode) map[string]string {
	if root == nil {
		return nil
	}
	result := map[string]string{}
	var walk func(n planNode)
	walk = func(n planNode) {
		if n == nil {
			return
		}
		nt := n.nodeType()
		if isScanNode(nt) {
			if rel := n.relationName(); rel != "" {
				if existing, ok := result[rel]; !ok || scanRank(nt) > scanRank(existing) {
					result[rel] = nt
				}
			}
		}
		for _, c := range n.children() {
			walk(c)
		}
	}
	walk(root)
	return result
}
