// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Predefined (named) queries: an SHCL sidecar file next to the DDL (design
// "Predefined queries"). Views reference them by name via startup_named_query,
// and the UIs offer them in a dropdown.
package ddl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	shcl "github.com/jim-collier/shcl/source/go"
)

// NamedQuery is one predefined query.
type NamedQuery struct {
	Name   string
	View   string  // view it is offered on; "" = every view
	Sort   float64 // dropdown order; unspecified sorts first in file order
	Active bool
	SQL    string
}

// QueriesPath is the sidecar convention: the DDL path with its extension
// replaced by ".queries.shcl" (issues.shcl -> issues.queries.shcl). Discovery
// looks for a lone schema file, so it must skip this one - see config.PWDSchema.
func QueriesPath(ddlPath string) string {
	return strings.TrimSuffix(ddlPath, filepath.Ext(ddlPath)) + ".queries.shcl"
}

// ScriptPath is the code sidecar: same base name, ".lua". The DDL's code:
// keys name functions defined in it.
func ScriptPath(ddlPath string) string {
	return strings.TrimSuffix(ddlPath, filepath.Ext(ddlPath)) + ".lua"
}

// queriesSchema is the sidecar's vocabulary. Small enough to live here rather
// than in its own file, and it sits beside the reader so the two stay in step.
const queriesSchema = `
field: "query_name[*]"
	type: string
	repeat: 0, 4096
	desc: One named query. Its value is the query name.
field: "query_name[*].view"
	type: string
	desc: View it is offered on; empty offers it on every view.
field: "query_name[*].sort"
	type: float
	desc: Dropdown order; unspecified sorts first, in file order.
field: "query_name[*].active"
	type: bool
field: "query_name[*].sql"
	desc: Untyped: SQL is read verbatim, so commas stay content.
`

var (
	queriesOnce sync.Once
	queriesDocV *shcl.Document
)

func queriesSchemaDoc() *shcl.Document {
	queriesOnce.Do(func() { queriesDocV = shcl.Parse(queriesSchema) })
	return queriesDocV
}

// ParseQueriesFile reads the sidecar; a missing file simply means no queries.
func ParseQueriesFile(path string) ([]NamedQuery, []string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	queries, warns, err := ParseQueries(b)
	if err != nil {
		return nil, warns, fmt.Errorf("%s: %w", path, err)
	}
	return queries, warns, nil
}

// ParseQueries parses queries text. Soft problems (nameless, duplicate, no SQL)
// drop the entry with a warning, matching DDL load validation. The result is
// dropdown-ordered: by sort value, file order within ties.
func ParseQueries(src []byte) ([]NamedQuery, []string, error) {
	doc, _ := shcl.ParseWith(string(src), shcl.Loose) // same tolerance as the DDL
	var warns []string
	report := func(diags []shcl.Diagnostic) {
		for _, d := range shcl.SuppressDeclaredRepeats(queriesSchemaDoc(), diags) {
			if isSchemaFault(d) {
				warns = append(warns, "built-in schema fault: "+d.Message)
				continue
			}
			warns = append(warns, fmt.Sprintf("line %d: %s", d.Line, d.Message))
		}
	}
	report(doc.Diagnostics())
	report(doc.Validate(queriesSchemaDoc()))

	// No duplicate-name check: two query_name instances with the same name are
	// one node by SHCL's merge rule, so a repeat cannot reach here as a second
	// entry - it lands as extra children on the first.
	root := cursor{doc: doc}
	var out []NamedQuery
	for i, name := range root.instances("query_name") {
		at := root.instance("query_name", i)
		if name == "" {
			warns = append(warns, "a query with no name was dropped")
			continue
		}
		query := NamedQuery{Name: name, View: at.str("view"), Active: at.boolOr("active", true)}
		if sort := at.floatPtr("sort"); sort != nil {
			query.Sort = *sort
		}
		// Read verbatim: SQL holds commas, which a normal string read would
		// treat as array separators. A fenced raw block reads the same way.
		if sql, ok := AsSQL(at.raw("sql")); ok {
			query.SQL = sql
		} else {
			query.SQL = strings.TrimSpace(at.raw("sql"))
		}
		if query.SQL == "" {
			warns = append(warns, fmt.Sprintf("query %q has no SQL and was dropped", name))
			continue
		}
		out = append(out, query)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sort < out[j].Sort })
	return out, warns, nil
}
