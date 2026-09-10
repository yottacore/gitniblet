// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

// View rendering: the resolved block tree becomes nested flexbox divs, leaves
// become panels that stay empty until their All button asks for rows - same
// contract as the TUI. Styles are computed here (ints only) so the template
// stays dumb.
package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/jim-collier/nano-git-db/internal/core/schema"
)

// blockVM is one block ready for the recursive template.
type blockVM struct {
	View     string
	Title    string
	Leaf     bool
	Index    int          // leaf index; how block rows are addressed
	Style    template.CSS // flex share, set by the parent
	DirStyle template.CSS // container: how children split
	Children []*blockVM
}

func (s *server) blockVM(view string, block *schema.ViewBlock, idx *int) *blockVM {
	node := &blockVM{View: view}
	if len(block.Children) > 0 {
		dir := "column"
		if block.Dir == "col" {
			dir = "row"
		}
		node.DirStyle = template.CSS("display:flex;flex-direction:" + dir + ";gap:6px;min-height:0;")
		props := block.Proportions()
		for i, child := range block.Children {
			childVM := s.blockVM(view, child, idx)
			childVM.Style = template.CSS(fmt.Sprintf("flex:%d 1 0;min-height:0;overflow:auto;", props[i]))
			node.Children = append(node.Children, childVM)
		}
		return node
	}
	node.Leaf = true
	node.Index = *idx
	*idx++
	roSuffix := ""
	if block.Readonly {
		roSuffix = ", ro"
	}
	node.Title = fmt.Sprintf("%s: %s (%s%s)", block.Name, block.Table, block.Type, roSuffix)
	return node
}

// view pulls and validates the view path segment; nil means already handled.
func (s *server) view(w http.ResponseWriter, r *http.Request) *schema.ViewSpec {
	spec := s.cat.View(r.PathValue("view"))
	if spec == nil {
		http.Error(w, "no such view", http.StatusNotFound)
	}
	return spec
}

func (s *server) viewPage(w http.ResponseWriter, r *http.Request) {
	spec := s.view(w, r)
	if spec == nil {
		return
	}
	idx := 0
	root := s.blockVM(spec.Name, spec.Root, &idx)
	root.Style = "height:85vh;" // anchor the flex tree; children split this
	data := map[string]any{
		"Name": spec.Name, "Root": root,
		"Queries": s.cat.QueriesFor(spec.Name),
	}
	// startup_named_query: an hx-trigger=load div fetches its dataset into the
	// first block as soon as the view renders. Unresolvable names degrade to
	// the normal empty open.
	if spec.StartupNamedQuery != "" && s.cat.NamedQuery(spec.StartupNamedQuery) != nil {
		data["StartupQuery"] = spec.StartupNamedQuery
	}
	s.render(w, "view.html", data)
}

// viewQuery runs a predefined query; the dataset renders readonly with the
// query's own column order (v1 uses the view's first block).
func (s *server) viewQuery(w http.ResponseWriter, r *http.Request) {
	if s.view(w, r) == nil {
		return
	}
	query := s.cat.NamedQuery(r.URL.Query().Get("name"))
	if query == nil {
		http.Error(w, "no such query", http.StatusNotFound)
		return
	}
	cols, rows, err := s.api.QueryRows(query.SQL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "qrows.html", map[string]any{"Name": query.Name, "Cols": cols, "Rows": rows})
}

// viewBlockRows loads one leaf block's dataset. Grid and tree_grid share the
// template: a grid is just a tree where everything is a root.
func (s *server) viewBlockRows(w http.ResponseWriter, r *http.Request) {
	spec := s.view(w, r)
	if spec == nil {
		return
	}
	i, err := strconv.Atoi(r.PathValue("i"))
	if err != nil || i < 0 || i >= len(spec.Leaves) {
		http.Error(w, "no such block", http.StatusNotFound)
		return
	}
	block := spec.Leaves[i]
	data := map[string]any{
		"Type": block.Type, "Table": block.Table, "Readonly": block.Readonly,
		"Cols": s.cat.ColumnsFor(block.Table), "View": spec.Name, "CommentsTarget": -1,
	}
	// A row of a list block that feeds a comments pane gets a link that loads
	// that row's thread into the pane.
	if leaves := spec.CommentsLeavesFor(i); len(leaves) > 0 {
		data["CommentsTarget"] = leaves[0]
	}
	switch block.Type {
	case "comments":
		// Empty until a row is picked from its list block; the link carries the id.
		s.render(w, "vcomments.html", map[string]any{
			"View": spec.Name, "Index": i, "Table": block.Table, "Selected": false,
		})
		return
	case "form":
		// One-record panel; block linking is future work, show the first row.
		rows, err := s.cat.LiveRows(s.api, block.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		row := map[string]string{}
		if len(rows) > 0 {
			row = rows[0]
		}
		type fv struct{ F, V string }
		var pairs []fv
		for _, f := range s.cat.Fields[block.Table] {
			pairs = append(pairs, fv{f, row[f]})
		}
		data["FieldVals"] = pairs
	case "tree_grid":
		tree, err := s.cat.TreeRows(s.api, block.Table, block.ParentField)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data["Rows"] = tree
	default: // grid
		rows, err := s.cat.LiveRows(s.api, block.Table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tree := make([]schema.TreeRow, len(rows))
		for j, row := range rows {
			tree[j] = schema.TreeRow{Row: row}
		}
		data["Rows"] = tree
	}
	s.render(w, "vrows.html", data)
}

// viewBlockComments renders (GET) or appends-then-renders (POST) the thread of
// one row in a comments pane. The row id rides the query string, so the pane
// follows whatever the list block last selected.
func (s *server) viewBlockComments(w http.ResponseWriter, r *http.Request) {
	spec := s.view(w, r)
	if spec == nil {
		return
	}
	i, err := strconv.Atoi(r.PathValue("i"))
	if err != nil || i < 0 || i >= len(spec.Leaves) || spec.Leaves[i].Type != "comments" {
		http.Error(w, "no such comments block", http.StatusNotFound)
		return
	}
	block := spec.Leaves[i]
	id := r.URL.Query().Get("id")
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if text := r.PostForm.Get("comment"); text != "" && id != "" {
			if _, err := s.api.CommentAdd(block.Table, id, text); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	data := map[string]any{
		"View": spec.Name, "Index": i, "Table": block.Table,
		"ID": id, "Selected": id != "",
	}
	if id != "" {
		comments, err := s.api.CommentsFor(block.Table, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data["Comments"] = comments
	}
	s.render(w, "vcomments.html", data)
}
