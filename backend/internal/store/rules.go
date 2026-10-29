package store

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"fragwatch/internal/rating"
)

// Rules are the flexible half of a filter: a tree of conditions joined by AND or OR, any of them
// negated, nested as deep as the question needs ("auctions nobody bid on for 5 days, or anything
// under half its typical price, but never names with digits"). The flat Filter fields cover the
// everyday chips; a rule tree is ANDed on top of them.
//
// Every leaf names a field from ruleFields and a comparison it allows; values are bound as
// parameters, never spliced into SQL. A leaf whose value is missing for a name (no price, no
// score) counts as false, so NOT does what people mean: "NOT under 30 TON" keeps unpriced names.
type Rule struct {
	Op    string          `json:"op,omitempty"` // group: "and" | "or"
	Not   bool            `json:"not,omitempty"`
	Rules []Rule          `json:"rules,omitempty"`
	Field string          `json:"field,omitempty"` // leaf
	Cmp   string          `json:"cmp,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

const (
	maxRuleDepth = 5
	maxRuleNodes = 60
)

type ruleField struct {
	kind string // num | set | text | bool
	expr string // SQL, for num and bool; set and text name their column
	vals []string
}

var ruleStatuses = []string{"auction", "sale", "market", "owned", "available"}

// ruleFields are what a rule can look at. Relative fields compare a name with something else:
// value_ratio with the typical price for its AI score, vs_last_sale with what it last sold for.
var ruleFields = map[string]ruleField{
	"score":          {kind: "num", expr: "score"},
	"buyout":         {kind: "num", expr: "buyout"},
	"price":          {kind: "num", expr: "price"},
	"len":            {kind: "num", expr: "len"},
	"last_sale":      {kind: "num", expr: "CASE WHEN frag_status='sold' THEN frag_price END"},
	"value_ratio":    {kind: "num"}, // built from the typical prices
	"vs_last_sale":   {kind: "num", expr: "(buyout / NULLIF(CASE WHEN frag_status='sold' THEN frag_price END, 0))"},
	"age_days":       {kind: "num", expr: "((EXTRACT(EPOCH FROM now()) - first_seen) / 86400.0)"},
	"ends_in_hours":  {kind: "num", expr: "(CASE WHEN frag_status='auction' THEN (frag_ends_at - EXTRACT(EPOCH FROM now())) / 3600.0 END)"},
	"quiet_days":     {kind: "num"}, // days since the price last moved (a bid, a repricing, a listing)
	"status":         {kind: "set", expr: "status", vals: ruleStatuses},
	"kind":           {kind: "set", expr: "category", vals: rating.Kinds},
	"market":         {kind: "set", expr: "lower(chain_market)"},
	"name":           {kind: "text", expr: "name"},
	"meaning":        {kind: "text", expr: "meaning"},
	"keywords":       {kind: "text"}, // any stem in the name (underscores ignored) or its meaning
	"has_digits":     {kind: "bool", expr: "has_digit = 1"},
	"has_underscore": {kind: "bool", expr: "has_under = 1"},
	"junk":           {kind: "bool", expr: "junk = 1"},
	"buy_now":        {kind: "bool", expr: "buyout IS NOT NULL"},
	"rated":          {kind: "bool", expr: "score IS NOT NULL"},
	"watched":        {kind: "bool", expr: "COALESCE(un_watched, 0) = 1"},
}

var numCmps = map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">=", "eq": "=", "ne": "<>"}

// RuleFieldsUsed reports whether a tree reads any of these fields (to load what they need).
func (r *Rule) uses(field string) bool {
	if r == nil {
		return false
	}
	if r.Field == field {
		return true
	}
	for i := range r.Rules {
		if r.Rules[i].uses(field) {
			return true
		}
	}
	return false
}

// Validate checks a tree without a database: shape, size, fields, comparisons and values.
func (r *Rule) Validate() error {
	_, _, err := r.compile(Typical{})
	return err
}

func (r *Rule) compile(typical Typical) (string, []any, error) {
	n := 0
	return r.node(typical, 0, &n)
}

func (r *Rule) node(typical Typical, depth int, n *int) (string, []any, error) {
	if *n++; *n > maxRuleNodes {
		return "", nil, fmt.Errorf("too many rules (max %d)", maxRuleNodes)
	}
	if depth > maxRuleDepth {
		return "", nil, fmt.Errorf("rules nest too deep (max %d)", maxRuleDepth)
	}
	var sql string
	var args []any
	if r.Field == "" {
		join := " AND "
		switch r.Op {
		case "or":
			join = " OR "
		case "and", "":
		default:
			return "", nil, fmt.Errorf("unknown group %q", r.Op)
		}
		var parts []string
		for i := range r.Rules {
			s, a, err := r.Rules[i].node(typical, depth+1, n)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, s)
			args = append(args, a...)
		}
		if len(parts) == 0 {
			sql = "TRUE" // an empty group asks for nothing
		} else {
			sql = "(" + strings.Join(parts, join) + ")"
		}
	} else {
		s, a, err := r.leaf(typical)
		if err != nil {
			return "", nil, err
		}
		sql, args = "COALESCE("+s+", FALSE)", a
	}
	if r.Not {
		sql = "(NOT " + sql + ")"
	}
	return sql, args, nil
}

func (r *Rule) leaf(typical Typical) (string, []any, error) {
	f, ok := ruleFields[r.Field]
	if !ok {
		return "", nil, fmt.Errorf("unknown field %q", r.Field)
	}
	bad := func() (string, []any, error) {
		return "", nil, fmt.Errorf("%s: %q does not fit %s", r.Field, r.Cmp, string(r.Value))
	}
	switch f.kind {
	case "num":
		expr := f.expr
		switch r.Field {
		case "value_ratio":
			expr = valueExpr(typical)
		case "quiet_days":
			// days since the last price event, or since we first saw the name when it never moved
			expr = "((EXTRACT(EPOCH FROM now()) - GREATEST(first_seen, COALESCE((SELECT MAX(e.ts) FROM events e WHERE e.name = usernames.name AND e.kind IN ('price','listed')), 0))) / 86400.0)"
		}
		if r.Cmp == "between" {
			var v [2]float64
			if json.Unmarshal(r.Value, &v) != nil || v[0] > v[1] {
				return bad()
			}
			return "(" + expr + " BETWEEN ? AND ?)", []any{v[0], v[1]}, nil
		}
		op, ok := numCmps[r.Cmp]
		var v float64
		if !ok || json.Unmarshal(r.Value, &v) != nil {
			return bad()
		}
		return "(" + expr + " " + op + " ?)", []any{v}, nil
	case "set":
		var vs []string
		if json.Unmarshal(r.Value, &vs) != nil || len(vs) == 0 || len(vs) > 20 || (r.Cmp != "in" && r.Cmp != "nin") {
			return bad()
		}
		args := []any{}
		for _, v := range vs {
			v = strings.ToLower(strings.TrimSpace(v))
			if f.vals != nil && !slices.Contains(f.vals, v) {
				return "", nil, fmt.Errorf("%s: unknown value %q", r.Field, v)
			}
			args = append(args, v)
		}
		cond := f.expr + " IN (" + placeholders(len(args)) + ")"
		if r.Cmp == "nin" {
			cond = "(" + f.expr + " IS NULL OR " + f.expr + " NOT IN (" + placeholders(len(args)) + "))"
		}
		return cond, args, nil
	case "text":
		if r.Field == "keywords" {
			var ks []string
			if json.Unmarshal(r.Value, &ks) != nil || len(ks) == 0 || len(ks) > 30 || r.Cmp != "any" {
				return bad()
			}
			var ors []string
			var args []any
			for _, k := range ks {
				if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
					ors = append(ors, "replace(name, '_', '') LIKE ? ESCAPE '\\' OR meaning ILIKE ?")
					args = append(args, "%"+likeEsc(strings.ReplaceAll(k, "_", ""))+"%", "%"+likeEsc(k)+"%")
				}
			}
			if len(ors) == 0 {
				return bad()
			}
			return "(" + strings.Join(ors, " OR ") + ")", args, nil
		}
		var v string
		if json.Unmarshal(r.Value, &v) != nil || strings.TrimSpace(v) == "" || len(v) > 64 {
			return bad()
		}
		v = strings.ToLower(strings.TrimSpace(v))
		switch r.Cmp {
		case "contains":
			return "(" + f.expr + " ILIKE ? ESCAPE '\\')", []any{"%" + likeEsc(v) + "%"}, nil
		case "starts":
			return "(" + f.expr + " ILIKE ? ESCAPE '\\')", []any{likeEsc(v) + "%"}, nil
		case "ends":
			return "(" + f.expr + " ILIKE ? ESCAPE '\\')", []any{"%" + likeEsc(v)}, nil
		case "eq":
			return "(lower(" + f.expr + ") = ?)", []any{v}, nil
		}
		return bad()
	case "bool":
		var v bool
		if r.Cmp != "is" || json.Unmarshal(r.Value, &v) != nil {
			return bad()
		}
		if v {
			return "(" + f.expr + ")", nil, nil
		}
		return "(NOT (" + f.expr + "))", nil, nil
	}
	return bad()
}

// ParseRules reads a rule tree from JSON (a query parameter, a tracker); empty means none.
func ParseRules(js string) (*Rule, error) {
	if strings.TrimSpace(js) == "" {
		return nil, nil
	}
	var r Rule
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// Simplify removes nesting that says nothing: a group with one rule becomes that rule (its NOT
// carried over), and a group inside a group of the same kind melts into it. The tree means the
// same; it only reads flatter.
func (r *Rule) Simplify() *Rule {
	if r == nil || r.Field != "" {
		return r
	}
	op := r.Op
	if op == "" {
		op = "and"
	}
	var kids []Rule
	for i := range r.Rules {
		c := r.Rules[i].Simplify()
		if c.Field == "" && !c.Not && (c.Op == op || (c.Op == "" && op == "and")) {
			kids = append(kids, c.Rules...) // same join, no NOT: its rules are ours
			continue
		}
		kids = append(kids, *c)
	}
	if len(kids) == 1 {
		one := kids[0]
		one.Not = one.Not != r.Not // NOT of NOT cancels
		return &one
	}
	return &Rule{Op: op, Not: r.Not, Rules: kids}
}
