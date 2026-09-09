// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// View resolution: turn the DDL's raw layout blocks into something the
// front-ends can render blindly - tables verified, types normalized, split
// hints reduced to direction + percent. Resolving once here keeps the TUI and
// web renderings from drifting.
package schema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/ddl"
)

// ViewBlock is one resolved layout block. A leaf (no children) binds a table;
// a container only arranges its children.
type ViewBlock struct {
	Name        string
	Table       string // leaves only
	Type        string // form|grid|tree_grid (leaves only)
	ParentField string // tree_grid only
	Readonly    bool
	Dir         string // how Children split: "row" (stacked) or "col" (side by side)
	Percent     int    // share of the parent split; 0 = split the leftover evenly
	Children    []*ViewBlock
}

// ViewSpec is one renderable view. Root is a synthetic container over the
// view's top-level blocks so front-ends render exactly one block tree. Leaves
// lists the data-bearing blocks in depth-first order; that index is how
// front-ends address a block.
type ViewSpec struct {
	Name              string
	StartupNamedQuery string
	Access            ddl.AccessRule // who may open the view (flat rule)
	Root              *ViewBlock
	Leaves            []*ViewBlock
}

// resolveViews validates every schema view against the catalog's tables.
// Blocks over unknown tables are dropped (warned), unknown block types and
// broken tree_grids degrade to plain grids, and views left with no leaves are
// dropped whole - so the front-ends never see a block they cannot render.
func (c *Catalog) resolveViews(schemas ...*ddl.Schema) {
	seen := map[string]bool{}
	for _, s := range schemas {
		for _, view := range s.Views {
			if view.Name == "" || seen[view.Name] {
				if view.Name != "" {
					c.warnf("view %q: duplicate name, keeping the first", view.Name)
				}
				continue
			}
			seen[view.Name] = true
			spec := ViewSpec{Name: view.Name, StartupNamedQuery: view.StartupNamedQuery, Access: view.Access}
			readonly := view.Readonly != nil && *view.Readonly
			if len(view.Layout) > 0 {
				spec.Root = c.resolveBlock(view.Name, ddl.Block{Children: view.Layout}, readonly, &spec.Leaves)
			}
			if len(spec.Leaves) == 0 {
				c.warnf("view %q: no renderable blocks, dropped", view.Name)
				continue
			}
			c.Views = append(c.Views, spec)
		}
	}
}

// resolveDefaultView picks the view the UIs open on start: the DDL's
// ui: default_view when it names a usable view, else the first resolved view
// per the design ("defaults to the first one defined").
func (c *Catalog) resolveDefaultView(schemas ...*ddl.Schema) {
	for _, s := range schemas {
		if s.DefaultView == "" {
			continue
		}
		if c.View(s.DefaultView) != nil {
			c.DefaultView = s.DefaultView
			return
		}
		c.warnf("default_view %q is not a usable view, falling back to the first one", s.DefaultView)
		break
	}
	if len(c.Views) > 0 {
		c.DefaultView = c.Views[0].Name
	}
}

// View finds a resolved view by name - the front-ends' guard against a view
// name arriving from user input.
func (c *Catalog) View(name string) *ViewSpec {
	for i := range c.Views {
		if c.Views[i].Name == name {
			return &c.Views[i]
		}
	}
	return nil
}

// PrimaryForComments returns the leaf index whose selection a comments leaf
// follows: the first non-comments leaf over the same table, or -1 when the
// view has no such list block (the pane then stays empty until a row arrives).
func (v *ViewSpec) PrimaryForComments(commentsIdx int) int {
	table := v.Leaves[commentsIdx].Table
	for i, leaf := range v.Leaves {
		if i != commentsIdx && leaf.Type != "comments" && leaf.Table == table {
			return i
		}
	}
	return -1
}

// CommentsLeavesFor returns the comments leaves that follow a list leaf's
// selection - the panes to refresh when its selected row changes.
func (v *ViewSpec) CommentsLeavesFor(primaryIdx int) []int {
	table := v.Leaves[primaryIdx].Table
	var out []int
	for i, leaf := range v.Leaves {
		if i != primaryIdx && leaf.Type == "comments" && leaf.Table == table {
			out = append(out, i)
		}
	}
	return out
}

func (c *Catalog) warnf(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

func (c *Catalog) resolveBlock(view string, block ddl.Block, inheritRO bool, leaves *[]*ViewBlock) *ViewBlock {
	readonly := inheritRO
	if block.Readonly != nil {
		readonly = *block.Readonly
	}
	resolved := &ViewBlock{Name: block.Name, Readonly: readonly}
	resolved.Percent = locPercent(block.Location)

	if len(block.Children) > 0 { // container: a table here would be ambiguous
		if block.Table != "" {
			c.warnf("view %q block %q: has child blocks, ignoring its table", view, block.Name)
		}
		// The first positioned child decides the split axis.
		resolved.Dir = "row"
		for _, child := range block.Children {
			if dir := locDir(child.Location); dir != "" {
				resolved.Dir = dir
				break
			}
		}
		for _, child := range block.Children {
			if resolvedChild := c.resolveBlock(view, child, readonly, leaves); resolvedChild != nil {
				resolved.Children = append(resolved.Children, resolvedChild)
			}
		}
		if len(resolved.Children) == 0 {
			return nil
		}
		return resolved
	}

	if block.Table == "" || !c.Has(block.Table) {
		c.warnf("view %q block %q: unknown table %q, dropped", view, block.Name, block.Table)
		return nil
	}
	resolved.Table = block.Table
	resolved.Type = block.Type
	switch block.Type {
	case "", "grid":
		resolved.Type = "grid"
	case "form":
	case "tree_grid":
		resolved.ParentField = block.ParentField
		if !fieldOf(c.Fields[block.Table], block.ParentField) {
			c.warnf("view %q block %q: tree_grid parent_field %q is not a field of %q, rendering as grid",
				view, block.Name, block.ParentField, block.Table)
			resolved.Type, resolved.ParentField = "grid", ""
		}
	case "comments":
		// A detail pane over the table's built-in comments component; it follows
		// a sibling list block's selected row. Only meaningful when the table
		// opted in to comments - otherwise there is nothing to show.
		if !c.Features[block.Table].Comments {
			c.warnf("view %q block %q: table %q has no comments feature, dropped",
				view, block.Name, block.Table)
			return nil
		}
	default:
		c.warnf("view %q block %q: unknown type %q, rendering as grid", view, block.Name, block.Type)
		resolved.Type = "grid"
	}
	*leaves = append(*leaves, resolved)
	return resolved
}

func fieldOf(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return name != ""
		}
	}
	return false
}

// locDir maps a location hint's direction word to a split axis. v0 places
// blocks in DDL order, so only the axis and percent of the hint are used;
// the relative-to element is ignored.
func locDir(location []string) string {
	for _, part := range location {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "left", "right":
			return "col"
		case "above", "below":
			return "row"
		}
	}
	return ""
}

func locPercent(location []string) int {
	for _, part := range location {
		part = strings.TrimSpace(part)
		if numStr, ok := strings.CutSuffix(part, "%"); ok {
			if pct, err := strconv.Atoi(strings.TrimSpace(numStr)); err == nil && pct >= 1 && pct <= 99 {
				return pct
			}
		}
	}
	return 0
}

// Proportions turns the children's percent hints into integer shares that sum
// to 100: hinted children keep their percent, the rest split the remainder.
func (b *ViewBlock) Proportions() []int {
	out := make([]int, len(b.Children))
	left, unhinted := 100, 0
	for i, child := range b.Children {
		if child.Percent > 0 {
			out[i] = child.Percent
			left -= child.Percent
		} else {
			unhinted++
		}
	}
	for i := range out {
		if out[i] == 0 {
			out[i] = max(1, left/max(1, unhinted))
		}
	}
	return out
}

// TreeRow is one row of a tree_grid dataset: Depth is how far below a root it
// sits along the parent chain.
type TreeRow struct {
	Depth int
	Row   map[string]string
}

// TreeRows orders a table's live rows depth-first along the self-referential
// parentField. Rows with an empty or unknown parent are roots. A cycle would
// never be reached from a root, so any row still unvisited afterwards starts
// its own root - bad data degrades the layout, never hides rows or hangs.
func (c *Catalog) TreeRows(api *crud.API, table, parentField string) ([]TreeRow, error) {
	if !fieldOf(c.Fields[table], parentField) {
		return nil, fmt.Errorf("table %q has no field %q", table, parentField)
	}
	rows, err := c.LiveRows(api, table)
	if err != nil {
		return nil, err
	}
	byID := map[string]bool{}
	for _, r := range rows {
		byID[r["id"]] = true
	}
	kids := map[string][]int{}
	var roots []int
	for i, r := range rows {
		parent := r[parentField]
		if parent == "" || !byID[parent] || parent == r["id"] {
			roots = append(roots, i)
		} else {
			kids[parent] = append(kids[parent], i)
		}
	}
	out := make([]TreeRow, 0, len(rows))
	visited := make([]bool, len(rows))
	var walk func(i, depth int)
	walk = func(i, depth int) {
		if visited[i] {
			return
		}
		visited[i] = true
		out = append(out, TreeRow{Depth: depth, Row: rows[i]})
		for _, k := range kids[rows[i]["id"]] {
			walk(k, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	for i := range rows {
		walk(i, 0)
	}
	return out, nil
}
