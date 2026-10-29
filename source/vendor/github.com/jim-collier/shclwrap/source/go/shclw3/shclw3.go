// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]
// SPDX-License-Identifier: MIT

// Package shclw3 is the shclw wrapper over shcl 3.0.
//
// Documents keep their lines as typed, so Text and Write give back what was
// read plus the edits, where 3.0 can manage that, and the canonical form
// where it can't. Rewrites says which. Write refuses with ErrRefused rather
// than drop a line.
package shclw3

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	shcl2 "github.com/jim-collier/shcl/source/go/v2"
	"github.com/jim-collier/shclwrap/source/go/shclw"
	shcl "github.com/yottacore/shcl/source/go/v2"
)

// ShclVersion is the shcl release this package wraps.
const ShclVersion = "3.0.0-dev+a010fdf8"

// FormatMajor is the newest file format this shcl reads.
const FormatMajor = shcl.FormatMajor

// Names from shclw, so a program needs only this import.
type (
	Status     = shclw.Status
	Strictness = shclw.Strictness
	Diag       = shclw.Diag
	Node       = shclw.Node
	Error      = shclw.Error
	ErrKind    = shclw.ErrKind
	Upgraded   = shclw.Upgraded
	Layer      = shclw.Layer
	Spot       = shclw.Spot
	Merge      = shclw.Merge
	Commented  = shclw.Commented
)

const (
	Good     = shclw.Good
	Empty    = shclw.Empty
	NotFound = shclw.NotFound
	BadType  = shclw.BadType
	Multiple = shclw.Multiple
	BadPath  = shclw.BadPath

	Loose    = shclw.Loose
	Standard = shclw.Standard
	Strict   = shclw.Strict
)

// KindOf is the ErrKind of err, or 0 when err didn't come from shclw.
func KindOf(err error) ErrKind { return shclw.KindOf(err) }

// Doc is a parsed document.
type Doc struct {
	d     *shcl.Document
	info  shclw.Info
	stamp bool
	up    *upgradedFile
	file  *readFile
	lay   *layerSet
	src   string // the text as loaded, for Merged
	newer int    // the newer format the text names, which makes it read-only
	level shcl.Strictness
	// the load's diagnostics, once a fill-in has parsed the text again
	loaded []shcl.Diagnostic
	// where a Write copied the file Open read, so the lines the load lost
	// are kept there
	backup string
}

// layerSet is what OpenLayers read. d is then their merge, rebuilt after
// each edit, and edits and saves go to docs[edit].
type layerSet struct {
	docs []*Doc // nil for a file that wasn't there
	edit int
}

// upgradedFile is the format Open upgraded the file from, and where the
// first Write over it put the original.
type upgradedFile struct {
	from   int
	backup string
}

// readFile is the file Open read, as it was then, so Write can tell it
// changed since. After a Write over it, it's what that Write left.
type readFile struct {
	path string
	st   os.FileInfo
	text string
	gone bool // an OpenLayers user file that wasn't there
}

var _ shclw.Doc = (*Doc)(nil)

// Lib is this package's calls, for code that picks a version at run time.
var Lib = shclw.Lib{
	Name:        "shcl " + ShclVersion,
	FormatMajor: FormatMajor,
	New:         func() shclw.Doc { return New() },
	Parse:       func(text string) shclw.Doc { return Parse(text) },
	ParseWith:   func(text string, s Strictness) (shclw.Doc, error) { return ParseWith(text, s) },
	OpenWith:    func(path string, s Strictness) (shclw.Doc, error) { return OpenWith(path, s) },
	OpenLayers:  func(layers ...Layer) (shclw.Doc, error) { return OpenLayers(layers...) },
	FormatOf:    FormatOf,
	Upgrade:     Upgrade,
}

// New is an empty document. It keeps lines like a loaded one, so it has
// none to lose and Rewrites says false.
func New() *Doc { return Parse("") }

// Parse never fails. Problems are in Diags.
func Parse(text string) *Doc {
	d, _ := ParseWith(text, shclw.Standard)
	return d
}

// ParseWith fails only at Strict, with ErrStrict. The document comes back
// either way.
func ParseWith(text string, s Strictness) (*Doc, error) {
	d, err := shcl.ParseKeepLines(text, strictness(s))
	if d == nil {
		d = shcl.New()
	}
	out := &Doc{d: d, src: text, level: strictness(s)}
	if f := FormatOf(text); f > FormatMajor {
		out.newer = f
	}
	if err == nil {
		return out, nil
	}
	e := &shclw.Error{Kind: shclw.ErrStrict, Err: err}
	var le *shcl.LoadError
	if errors.As(err, &le) {
		e.Diags = diags(le.Diagnostics)
	}
	return out, e
}

// Open reads and parses a file. A missing file is ErrNotFound. A file
// written for a newer format is read as this shcl reads it, and is
// read-only: Write on it is ErrTooNew. An older file is upgraded in
// memory, as Upgrade with from 0 does, and UpgradedFrom says so. Nothing is
// written until Write, and the first Write over the file backs it up. A 2.x
// file that reads two ways is ErrAmbiguous, and one shcl 2.0 reports errors
// on, or that can't be made to read as 2.0 read it, is ErrRefused, with the
// lines in Diags. On an error the document is empty.
func Open(path string) (*Doc, error) { return OpenWith(path, shclw.Standard) }

// OpenWith is Open at a given strictness, which applies to the upgraded text.
func OpenWith(path string, s Strictness) (*Doc, error) {
	text, err := shclw.ReadText(path)
	if err != nil {
		return New(), err
	}
	f := FormatOf(text)
	var up *upgradedFile
	body := text
	// A file naming this format is current, so it skips the upgrade's parse.
	if f < FormatMajor {
		u, err := Upgrade(text, 0)
		if err != nil {
			err.(*shclw.Error).Path = path
			return New(), err
		}
		if u.Changed {
			up = &upgradedFile{from: u.From}
			body = u.Text
		}
	}
	d, err := ParseWith(body, s)
	d.up = up
	d.file = &readFile{path: path, text: text}
	d.file.st, _ = os.Stat(path)
	if e, ok := err.(*shclw.Error); ok {
		e.Path = path
	}
	return d, err
}

// OpenLayers reads layers in order, lowest first, such as defaults, then a
// site file, then the user's, and reads as their merge: a later layer's
// value, array, or raw block takes the place of a lower one's, and blocks
// merge name by name, as shcl's Merge does. A missing file is skipped.
//
// Edits and saves go to the last file layer, so Text, Write, and the rest
// are that layer's, and Write writes only it. Text layers after it, such as
// this run's command line, still win reads. Each file is opened as Open
// does, and the first error is returned with an empty document. Diags are
// every layer's, with Layer set (design.md, Decisions).
func OpenLayers(layers ...Layer) (*Doc, error) {
	if len(layers) == 0 {
		return New(), nil
	}
	l := &layerSet{docs: make([]*Doc, len(layers)), edit: len(layers) - 1}
	for i := len(layers) - 1; i >= 0; i-- {
		if layers[i].File != "" {
			l.edit = i
			break
		}
	}
	for i, ly := range layers {
		if ly.File == "" {
			l.docs[i] = Parse(ly.Text)
			continue
		}
		x, err := Open(ly.File)
		switch {
		case shclw.KindOf(err) == shclw.ErrNotFound:
			if i == l.edit {
				l.docs[i] = New()
				l.docs[i].file = &readFile{path: ly.File, gone: true}
			}
		case err != nil:
			if e, ok := err.(*shclw.Error); ok {
				for j := range e.Diags {
					e.Diags[j].Layer = i + 1
				}
			}
			return New(), err
		default:
			l.docs[i] = x
		}
	}
	d := &Doc{lay: l}
	d.remerge()
	return d, nil
}

// remerge builds the reads' document from the layers again. Lower layers
// never change, but a merge changes its base, so it starts over.
func (d *Doc) remerge() {
	m := shcl.New()
	for _, x := range d.lay.docs {
		if x != nil {
			m.Merge(x.d)
		}
	}
	d.d = m
}

// top is where edits and saves go: the edit layer of a layered document,
// or the document itself.
func (d *Doc) top() *Doc {
	if d.lay == nil {
		return d
	}
	return d.lay.docs[d.lay.edit]
}

// edited rebuilds the merge after an edit to the edit layer.
func (d *Doc) edited() {
	if d.lay != nil {
		d.remerge()
	}
}

// FormatOf is the format a file's Format line names, or 0 with none.
func FormatOf(text string) int {
	n, ok := shcl.FormatVersion(text)
	if !ok {
		return 0
	}
	return n
}

// Upgrade converts a file written for an older format. With from 0 the
// file's Format line decides. A file with none is taken as 2.x when it has
// 2.x's info block, or errors on 3.0 that shcl 2.0 doesn't have, and
// otherwise as current. Pass from 2 for a file known to be from shcl 2.x or
// older. A 2.x file that reads two ways is ErrAmbiguous, and one shcl 2.0
// reports errors on is ErrRefused, with those errors in
// Diags. A 2.x file comes out reading what 2.0 read, backslash escapes
// included, and one that can't is ErrRefused too. 2.x's block is swapped for
// the current one where it sits, and the lines end as the file's did. Each
// line keeps its number, blank ones included.
func Upgrade(text string, from int) (Upgraded, error) {
	if f := max(FormatOf(text), from); f > FormatMajor {
		return Upgraded{Text: text, From: f}, tooNew("", f)
	}
	// The new block goes where 2.x's sat, so a mark shcl leaves be stands in.
	// A file with no Format line that has 2.x's block is 2.x (design.md,
	// Decisions).
	in, banner := text, false
	if FormatOf(text) == 0 {
		in, banner = shclw.OldBlock(text, infoEdit)
		if banner && from == 0 {
			from = 2
		}
	}
	fromV2 := from > 0 && from < FormatMajor
	u := shcl.Upgrade(in, fromV2)
	out := upgraded(u)
	// shcl says 2 for a file with no Format line, even one it leaves alone.
	if u.Current && !fromV2 && FormatOf(text) == 0 {
		out.From = FormatMajor
	}
	// Only a file taken as 2.x is read the 2.x way.
	if u.Current && !(fromV2 && u.Format < FormatMajor) {
		out.Text = text
		return out, nil
	}
	// Nothing in this file says it's 2.x, only its errors on 3.0. With errors
	// on 2.0 too, it's a current file with a typo (design.md, Decisions).
	old := shcl2.Parse(text)
	guessed := !fromV2 && FormatOf(text) == 0
	if guessed && oldErrors(old) != nil {
		return Upgraded{Text: text, From: FormatMajor}, nil
	}
	if u.Ambiguous > 0 {
		out.Text = text
		return out, &shclw.Error{Kind: shclw.ErrAmbiguous, Msg: fmt.Sprintf("%d lines read two ways", u.Ambiguous)}
	}
	if err := oldErrors(old); err != nil {
		return Upgraded{Text: text, From: u.Format}, err
	}
	// shcl gives the input back then, and this would read it the 2.x way.
	if len(u.ErrorsLeft) > 0 {
		return Upgraded{Text: text, From: u.Format}, &shclw.Error{Kind: shclw.ErrRefused,
			Msg: fmt.Sprintf("%d lines would still have an error once converted; fix them and upgrade again", len(u.ErrorsLeft))}
	}
	if out.Changed {
		out.Text = shclw.KeepBlanks(in, out.Text, infoEdit)
	}
	lost, err := checkReads(old, out.Text, FormatOf(text) == 1)
	if err != nil {
		return Upgraded{Text: text, From: u.Format}, err
	}
	out.Lost = lost
	var fixed string
	switch {
	case banner:
		fixed, err = shclw.PlaceBlock(shclw.SameEnds(text, out.Text, infoEdit), in != text, infoEdit)
	case !out.Changed:
		return out, nil
	default:
		fixed = shclw.SameEnds(text, out.Text, infoEdit)
	}
	if err != nil {
		return Upgraded{Text: text, From: u.Format}, err
	}
	out.Text, out.Changed = fixed, fixed != text
	out.Rewritten = shclw.RewrittenLines(text, fixed, lost)
	return out, nil
}

// UpgradeFile is Upgrade on a file. When it changes the file it first
// copies the original beside it, and Backup names the copy. A file that
// changed after it was read is left alone, and a failed write leaves the
// original in place (shclw.ReplaceRead).
func UpgradeFile(path string, from int) (Upgraded, error) {
	if from > FormatMajor {
		return Upgraded{From: from}, tooNew(path, from)
	}
	// A FIFO would block the read, and the write would put a file in place
	// of a device.
	if st, err := os.Stat(path); err == nil && !st.Mode().IsRegular() {
		return Upgraded{}, &shclw.Error{Kind: shclw.ErrIO, Path: path, Msg: "not a regular file"}
	}
	// One read for the 2.0 check, the conversion, and the backup. shcl's own
	// UpgradeFile would read the file a second time, after the check.
	text, err := shclw.ReadText(path)
	if err != nil {
		return Upgraded{}, err
	}
	u, err := Upgrade(text, from)
	if err != nil {
		if e, ok := err.(*shclw.Error); ok {
			e.Path = path
		}
		return Upgraded{From: u.From}, err
	}
	if !u.Changed {
		return u, nil
	}
	write := func(path, data string) error {
		if err := shcl.WriteFileAtomic(path, data); err != nil {
			return &shclw.Error{Kind: shclw.ErrIO, Path: path, Err: err}
		}
		return nil
	}
	if u.Backup, err = shclw.ReplaceRead(path, text, u.Text, keepOriginal(path, text, u.From), write); err != nil {
		return Upgraded{From: u.From}, err
	}
	return u, nil
}

// keepOriginal copies text, what path held when it was read, beside it under
// shcl's backup name for format from.
func keepOriginal(path, text string, from int) func() (string, error) {
	return func() (string, error) {
		name, err := shcl.WriteBackup(path, text, from)
		if err == nil {
			return name, nil
		}
		e := &shclw.Error{Kind: shclw.ErrIO, Path: path, Err: err}
		var ue *shcl.UpgradeError
		if errors.As(err, &ue) && ue.Kind == shcl.UpgradeBackupTaken {
			e.Kind = shclw.ErrBackupTaken
		}
		return "", e
	}
}

// oldErrors refuses a file taken as 2.x that shcl 2.0 reports errors on.
// The old program read those lines as it repaired them, or not at all, and
// an open raw block took the rest of the file. The upgrade would turn that
// into a clean file that means something else (design.md, Format and
// upgrade).
func oldErrors(old *shcl2.Document) *shclw.Error {
	var errs []Diag
	lines := map[int]bool{}
	for _, d := range old.Diagnostics() {
		if d.Severity == shcl2.SeverityError {
			errs = append(errs, Diag{Line: d.Line, Severity: shclw.SevError, Code: d.Code, Message: d.Message})
			lines[d.Line] = true
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &shclw.Error{Kind: shclw.ErrRefused, Diags: errs, Msg: fmt.Sprintf("shcl 2.0 reports errors on %d lines; fix them and upgrade again", len(lines))}
}

// checkReads reads every value 2.0 reads in old back against up, the
// upgraded text. shcl's migrate writes the text 2.x read, backslash escapes
// included (2026101910455900), so this checks its answer rather than
// trusting it. A name 2.0 read with an escape resolved stays as typed, so
// it's refused, unless the file names format 1, since 1.2 read names as
// typed. So is a value that reads differently. The lines with a value 2.0
// read that up doesn't have at all come back as lost, in order.
func checkReads(old *shcl2.Document, up string, namesAsTyped bool) ([]int, error) {
	now := shcl.Parse(up)
	var bad []Diag
	for _, f := range now.Fields() {
		if !namesAsTyped && v2Escapes(f.Name) != f.Name {
			bad = append(bad, carryDiag(old.Line(f.Path), "shcl 2.0 read this name with its backslash escapes resolved, and 3.0 reads it as typed; rename it and upgrade again"))
		}
	}
	var lost []int
	for _, p := range old.Paths() {
		had, has, lines := old.Count(p), now.Count(p), old.Lines(p)
		// Instances past the ones 3.0 still has are gone.
		for _, l := range lines[min(has, had):] {
			if l > 0 {
				lost = append(lost, l)
			}
		}
		if had != 1 && has != had {
			continue
		}
		// A repeated key is read one instance at a time, the i-th against
		// the i-th, as [#i] in 2.0 and (i) in 3.0 (2026102816332481).
		for i := 0; i < had; i++ {
			p2, p3 := p, p
			if had > 1 {
				p2, p3 = fmt.Sprintf("%s[#%d]", p, i), fmt.Sprintf("%s(%d)", p, i)
			}
			was, is := old.ReadStringArray(p2), now.ReadStringArray(p3)
			if was.Status == shcl2.Good && is.Status == shcl.Good && !slices.Equal(was.Value, is.Value) {
				bad = append(bad, carryDiag(lines[i], "the upgraded file doesn't read this value as shcl 2.0 read it"))
			}
			if has == had && was.Status == shcl2.Good && is.Status != shcl.Good && now.ReadRaw(p3).Status != shcl.Good {
				lost = append(lost, lines[i])
			}
		}
	}
	if len(bad) == 0 {
		slices.Sort(lost)
		return slices.Compact(lost), nil
	}
	lines := map[int]bool{}
	for _, b := range bad {
		lines[b.Line] = true
	}
	return nil, &shclw.Error{Kind: shclw.ErrRefused, Diags: bad,
		Msg: fmt.Sprintf("%d lines don't read as shcl 2.0 read them; fix them and upgrade again", len(lines))}
}

func carryDiag(line int, msg string) Diag {
	return Diag{Line: line, Severity: shclw.SevError, Code: "W002", Message: msg}
}

// v2Escapes is s as shcl 1.2 and 2.0 read it: \t, \n, \\, \", and \' are
// escapes, and any other backslash is itself.
func v2Escapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case '\\', '"', '\'':
			b.WriteByte(s[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func upgraded(u shcl.Upgraded) Upgraded {
	return Upgraded{Text: u.Text, From: u.Format, Changed: !u.Current, Unsure: u.Lost, Backup: u.Backup}
}

// Inner is the shcl document underneath, for calls shclw doesn't cover. On
// a layered document it's the merge, which OpenLayers builds again after
// every edit.
func (d *Doc) Inner() *shcl.Document { return d.d }

// Text is the document as it would be saved: its lines as typed, plus the
// edits, or the canonical form when 3.0 can't keep the lines.
func (d *Doc) Text() string {
	t := d.top()
	return t.finish(t.lines())
}

// Canonical is the document in canonical form.
func (d *Doc) Canonical() string {
	t := d.top()
	return t.finish(t.d.ToCanonical())
}

// lines is the save's text before the info block and the Format line.
func (d *Doc) lines() string {
	text, ok := d.d.ToTextKeepLines()
	if !ok {
		return d.d.ToCanonical()
	}
	return text
}

// finish is text with the info block and the Format line as InfoBlock and
// StampFormat asked. When an edit since made that impossible, it's the text
// without them, and Write says why.
func (d *Doc) finish(text string) string {
	if out, err := saved(text, d.info, d.stamp); err == nil {
		return out
	}
	return text
}

// saved is text as a save writes it. Where that has no Format line, 2.x's
// block is swapped for the current one where it sits, as the upgrade does,
// so the next Open doesn't take the file as 2.x (2026101513330010).
func saved(text string, info shclw.Info, stamp bool) (string, error) {
	out, err := shclw.FinishInfo(text, info, stamp, infoEdit)
	if err != nil || FormatOf(out) != 0 {
		return out, err
	}
	marked, found := shclw.OldBlock(out, infoEdit)
	if !found {
		return out, nil
	}
	return shclw.PlaceBlock(marked, marked != out, infoEdit)
}

// Write saves Text atomically. It refuses with ErrRefused when the load
// dropped lines the save would lose, the same check shcl's
// SaveFileKeepLines makes, and writes nothing when the info block or the
// Format line can't go in, since an edit made after InfoBlock or
// StampFormat. A save with no Format line has 2.x's info block swapped for
// the current one where it sat.
//
// The first Write over a file Open upgraded backs up the original first, as
// UpgradeFile does, and BackupName names the copy. WriteOpts.Backup does the
// same in place of the refusal. A Write over the file Open read leaves it
// alone with ErrChanged when it changed on disk since, and a document read
// from a newer format is ErrTooNew.
func (d *Doc) Write(path string) error { return d.WriteWith(path, shclw.WriteOpts{}) }

// WriteWith is Write with options. Force writes over a file that changed
// since it was read.
func (d *Doc) WriteWith(path string, o shclw.WriteOpts) error {
	if d.lay != nil {
		return d.top().WriteWith(path, o)
	}
	if d.newer > 0 {
		return tooNew(path, d.newer)
	}
	if err := shclw.CheckMode(path, o.Mode); err != nil {
		return err
	}
	f := d.file
	over := f != nil && !f.gone && !o.Create && f.over(path)
	text, kept := d.d.ToTextKeepLines()
	keepLost := false
	if lost := d.d.LostCount(); !kept && lost > 0 && d.backup == "" {
		if !o.Backup || !over {
			return &shclw.Error{Kind: shclw.ErrRefused, Path: path,
				Msg: fmt.Sprintf("%d lines would be lost", lost), Err: &shcl.SaveRefused{Path: path, Lost: lost}}
		}
		keepLost = true
	}
	if !kept {
		text = d.d.ToCanonical()
	}
	text, err := saved(text, d.info, d.stamp)
	if err != nil {
		err.(*shclw.Error).Path = path
		return err
	}
	if o.Create {
		if err := shclw.CreateNew(path, text, o.Mode); err != nil {
			return err
		}
		if f != nil && f.over(path) {
			f.text, f.gone = text, false
			f.st, _ = os.Stat(path)
		}
		return nil
	}
	write := writeAtomic
	if o.Mode != 0 {
		write = writeMode(o.Mode)
	}
	if f == nil || !f.over(path) {
		return write(path, text)
	}
	var backup func() (string, error)
	upFirst := d.up != nil && d.up.backup == ""
	switch {
	case upFirst:
		backup = keepOriginal(path, f.text, d.up.from)
	case keepLost:
		backup = keepOriginal(path, f.text, FormatMajor)
	}
	name, err := f.replace(path, text, backup, o.Force, write)
	if err != nil {
		return err
	}
	if upFirst {
		d.up.backup = name
	}
	if backup != nil {
		d.backup = name
	}
	f.text, f.gone = text, false
	f.st, _ = os.Stat(path)
	return nil
}

// replace writes text over the file Open read, unless it changed since.
// Only the moment between the last check and the rename is left open
// (design.md, Loading and saving).
func (f *readFile) replace(path, text string, backup func() (string, error), force bool, write func(path, text string) error) (string, error) {
	was := f.text
	if force {
		now, err := shclw.ReadText(path)
		if err != nil {
			return "", write(path, text)
		}
		was = now
	} else if f.gone {
		if _, err := os.Lstat(path); err == nil {
			return "", shclw.Changed(path)
		}
		return "", write(path, text)
	}
	return shclw.ReplaceRead(path, was, text, backup, write)
}

// writeMode is writeAtomic where a file it creates gets mode. One that's
// there keeps its own, as shcl's atomic write keeps it.
func writeMode(mode fs.FileMode) func(path, text string) error {
	return func(path, text string) error {
		if _, err := os.Lstat(path); err == nil {
			return writeAtomic(path, text)
		}
		err := shclw.CreateNew(path, text, mode)
		if shclw.KindOf(err) == shclw.ErrExists {
			return writeAtomic(path, text) // made in the meantime
		}
		return err
	}
}

func writeAtomic(path, text string) error {
	err := shcl.WriteFileAtomic(path, text)
	if err == nil {
		return nil
	}
	var we *shcl.WriteError
	if errors.As(err, &we) && we.Status == shcl.WriteNotFound {
		return &shclw.Error{Kind: shclw.ErrNotFound, Path: path, Err: err}
	}
	return &shclw.Error{Kind: shclw.ErrIO, Path: path, Err: err}
}

// over is whether path names the file Open read: the same path, or the same
// file by another name.
func (f *readFile) over(path string) bool {
	a, aerr := filepath.Abs(path)
	b, berr := filepath.Abs(f.path)
	if aerr == nil && berr == nil && a == b {
		return true
	}
	st, err := os.Stat(path)
	return err == nil && f.st != nil && os.SameFile(st, f.st)
}

// ReadOnly is whether the document was read from a newer format, which Write
// refuses with ErrTooNew. On a layered document it's the edit layer's.
func (d *Doc) ReadOnly() bool { return d.top().newer > 0 }

// UpgradedFrom is the format Open upgraded the file from, or 0 when it
// didn't upgrade it. Reads, Lines, Diags, and Text are all of the upgraded
// text.
func (d *Doc) UpgradedFrom() int {
	d = d.top()
	if d.up == nil {
		return 0
	}
	return d.up.from
}

// BackupName is where a Write copied the file Open read, or "" when none
// did. That's the first Write over an upgraded file, or one with
// WriteOpts.Backup that would have lost lines.
func (d *Doc) BackupName() string { return d.top().backup }

// Rewrites says whether Write puts the canonical form in place of the lines
// the document was loaded from. 3.0 does that when it can't keep them with
// the edits in, such as after a child is added under a flat dotted line,
// and comments can move. It builds the save's text to find out. InfoBlock
// and StampFormat change only their own lines, so they don't change the
// answer. A file Open upgraded is rewritten until the first Write over it.
func (d *Doc) Rewrites() bool {
	d = d.top()
	if d.up != nil && d.up.backup == "" {
		return true
	}
	_, kept := d.d.ToTextKeepLines()
	return !kept
}

// InfoBlock puts the info block, shcl's GenBanner, at the end of what the
// save writes, or with on false leaves it out. Either way an old block comes
// off first, in any spelling shcl has written, wherever it is. shcl's own
// SetBanner leaves one above the first field. A file whose Format line
// names another format is refused, and so is one that ends in a raw block
// that never closes.
func (d *Doc) InfoBlock(on bool) error {
	d = d.top()
	info := shclw.InfoOff
	if on {
		info = shclw.InfoOn
	}
	if _, err := shclw.FinishInfo(d.lines(), info, d.stamp, infoEdit); err != nil {
		return err
	}
	d.info = info
	return nil
}

// StampFormat adds the Format line at the end of what the save writes,
// unless a comment already names format 3. It refuses what InfoBlock
// refuses.
func (d *Doc) StampFormat() error {
	d = d.top()
	if _, err := shclw.FinishInfo(d.lines(), d.info, true, infoEdit); err != nil {
		return err
	}
	d.stamp = true
	return nil
}

// Diags is what the load found. A layered document has every layer's, in
// layer order, each with its Layer.
func (d *Doc) Diags() []Diag {
	if d.lay == nil {
		if d.loaded != nil {
			return diags(d.loaded)
		}
		return diags(d.d.Diagnostics())
	}
	var out []Diag
	for i, x := range d.lay.docs {
		if x == nil {
			continue
		}
		for _, g := range x.Diags() {
			g.Layer = i + 1
			out = append(out, g)
		}
	}
	return out
}

// Errors counts error diagnostics, in every layer of a layered document.
func (d *Doc) Errors() int {
	if d.lay == nil {
		if d.loaded != nil {
			n := 0
			for _, g := range d.loaded {
				if g.Severity == shcl.SeverityError {
					n++
				}
			}
			return n
		}
		return d.d.ErrorCount()
	}
	n := 0
	for _, x := range d.lay.docs {
		if x != nil {
			n += x.Errors()
		}
	}
	return n
}

// Unused is the lines the load couldn't use, which are the lines with an
// error, once each.
func (d *Doc) Unused() []Spot { return shclw.UnusedOf(d.Diags()) }

// Merged is each name the load kept one copy of, since a later copy had the
// same name and value, with every copy's line. On a layered document it's
// each layer's own, and not what the layers' merge folded.
func (d *Doc) Merged() []Merge {
	if d.lay == nil {
		return shclw.MergesOf(d.src, parseDoc, 0)
	}
	out := []Merge{}
	for i, x := range d.lay.docs {
		if x != nil {
			out = append(out, shclw.MergesOf(x.src, parseDoc, i+1)...)
		}
	}
	return out
}

func parseDoc(text string) shclw.Doc { return Parse(text) }

// Commented is each comment line that reads as one `key: value` once its
// `#` comes off, with the path it would set. A layered document's are the
// edit layer's.
func (d *Doc) Commented() []Commented {
	t := d.top()
	return shclw.CommentedOf(t.lines(), parseDoc)
}

// fill swaps path's commented `## Default` line for the setting, so the set
// that follows takes that line and the ones under it don't move
// (2026101513330020). It's only where the save keeps the file's lines and
// path isn't set yet. undo puts the document back, for a set that fails.
func (d *Doc) fill(path string) (undo func()) {
	undo = func() {}
	if d.d.Exists(path) {
		return undo
	}
	text, kept := d.d.ToTextKeepLines()
	if !kept {
		return undo
	}
	line := shclw.DefaultLine(text, path, parseDoc)
	if line == 0 {
		return undo
	}
	nd, _ := shcl.ParseKeepLines(shclw.Uncommented(text, line), d.level)
	if nd == nil || nd.ErrorCount() != d.d.ErrorCount() || !nd.Exists(path) {
		return undo
	}
	was, wasLoaded := d.d, d.loaded
	if d.loaded == nil {
		d.loaded = d.d.Diagnostics()
	}
	d.d = nd
	return func() { d.d, d.loaded = was, wasLoaded }
}

// Check validates the document against a schema document.
func (d *Doc) Check(schema shclw.Doc) []Diag {
	s, ok := schema.(*Doc)
	if !ok {
		return []Diag{otherVersion}
	}
	return diags(d.d.Validate(s.d))
}

// DropDeclared takes out the repeat and reopen hints for keys this schema
// says repeat.
func (d *Doc) DropDeclared(ds []Diag) []Diag {
	in := shcl.SuppressDeclaredRepeats(d.d, undiags(ds))
	return diags(shcl.SuppressDeclaredReopens(d.d, in))
}

func (d *Doc) Int(path string, fallback int64) int64   { return d.d.GetIntOr(path, fallback) }
func (d *Doc) Bool(path string, fallback bool) bool    { return d.d.GetBoolOr(path, fallback) }
func (d *Doc) Str(path string, fallback string) string { return d.d.GetStringOr(path, fallback) }
func (d *Doc) Strs(path string, fallback []string) []string {
	return d.d.GetStringArrayOr(path, fallback)
}
func (d *Doc) Ints(path string, fallback []int64) []int64 { return d.d.GetIntArrayOr(path, fallback) }
func (d *Doc) Raw(path string, fallback string) string    { return d.d.GetRawOr(path, fallback) }

// Float and Floats go through GetFloat, so infinity gets the fallback too.
func (d *Doc) Float(path string, fallback float64) float64 {
	if g := d.GetFloat(path); g.Ok() {
		return g.Value
	}
	return fallback
}

func (d *Doc) Floats(path string, fallback []float64) []float64 {
	if g := d.GetFloats(path); g.Ok() {
		return g.Value
	}
	return fallback
}

func (d *Doc) GetInt(path string) shclw.Got[int64] { return got(d.d.ReadInt(path)) }
func (d *Doc) GetFloat(path string) shclw.Got[float64] {
	return shclw.FiniteFloat(got(d.d.ReadFloat(path)))
}
func (d *Doc) GetBool(path string) shclw.Got[bool]     { return got(d.d.ReadBool(path)) }
func (d *Doc) GetStr(path string) shclw.Got[string]    { return got(d.d.ReadString(path)) }
func (d *Doc) GetStrs(path string) shclw.Got[[]string] { return got(d.d.ReadStringArray(path)) }
func (d *Doc) GetInts(path string) shclw.Got[[]int64]  { return got(d.d.ReadIntArray(path)) }
func (d *Doc) GetFloats(path string) shclw.Got[[]float64] {
	return shclw.FiniteFloats(got(d.d.ReadFloatArray(path)))
}
func (d *Doc) GetRaw(path string) shclw.Got[string] { return got(d.d.ReadRaw(path)) }

func (d *Doc) Has(path string) bool  { return d.d.Exists(path) }
func (d *Doc) Count(path string) int { return d.d.Count(path) }
func (d *Doc) Line(path string) int  { return d.d.Line(path) }

// Names lists the child names under path once each, in file order.
func (d *Doc) Names(path string) []string { return shclw.DedupeNames(d.d.Children(path)) }

// Each is every instance path names, each with a path that reads and edits
// it alone: 3.0's own instance path, with an index on every step that
// repeats. A wildcard slot that reaches no node or several has none, and is
// left out.
func (d *Doc) Each(path string) []Node {
	fields := d.d.ReadFields(path).Value
	out := make([]Node, 0, len(fields))
	for _, f := range fields {
		if f.Path != "" {
			out = append(out, Node{Path: f.Path, Value: f.Value, Line: f.Line})
		}
	}
	return out
}

// Child is base with one more name on the end, quoted if it needs it.
func (d *Doc) Child(base, name string) string {
	if base == "" {
		return shcl.QuoteSegment(name)
	}
	return base + "." + shcl.QuoteSegment(name)
}

// First is path narrowed to its first instance in the file, so a read or an
// edit takes that one where it would say Multiple. A path with no instances
// comes back as it is.
func (d *Doc) First(path string) string { return shclw.FirstOf(d, path) }

// Last is First for the last instance in the file, the one that wins where
// a later line overrides an earlier one.
func (d *Doc) Last(path string) string { return shclw.LastOf(d, path) }

// Lines is the line of each instance of path, in file order. An instance a
// put made has none, and is a 0 at the end.
func (d *Doc) Lines(path string) []int { return shclw.LinesOf(d, path) }

// Put sets path to v. It takes ints, floats, bools, strings, and slices of
// strings, ints, and floats. A value that doesn't read back the same is
// ErrNoReadBack, with the edit left in place.
func (d *Doc) Put(path string, v any) error {
	if d.lay != nil {
		defer d.edited()
		return d.top().Put(path, v)
	}
	val, err := shclw.Normalize(path, v)
	if err != nil {
		return err
	}
	undo := d.fill(path)
	var st shcl.SetStatus
	switch val.Kind {
	case shclw.KindInt:
		st = d.d.SetInt(path, val.Int)
	case shclw.KindFloat:
		st = d.d.SetFloat(path, val.Float)
	case shclw.KindBool:
		st = d.d.SetBool(path, val.Bool)
	case shclw.KindStr:
		st = d.d.SetString(path, val.Str)
	case shclw.KindStrs:
		st = d.d.SetStringArray(path, val.Strs)
	case shclw.KindInts:
		st = d.d.SetIntArray(path, val.Ints)
	case shclw.KindFloats:
		st = d.d.SetFloatArray(path, val.Floats)
	}
	if err := d.setErr(path, st); err != nil {
		undo()
		return err
	}
	return shclw.VerifyPut(d, path, val)
}

// PutSyntax sets path to text read as shcl value syntax. Text that doesn't
// read back as typed, such as one with a trailing comment, is
// ErrNoReadBack, with the edit left in place.
func (d *Doc) PutSyntax(path, text string) error {
	if d.lay != nil {
		defer d.edited()
		return d.top().PutSyntax(path, text)
	}
	undo := d.fill(path)
	if err := d.setErr(path, d.d.SetLiteral(path, text)); err != nil {
		undo()
		return err
	}
	alone := New()
	// A failed set leaves alone empty, which the check reports.
	alone.d.SetLiteral("k", text)
	return shclw.VerifySyntax(d, path, alone, Parse("k: "+text+"\n"))
}

// Comment sets the comment above path.
func (d *Doc) Comment(path, text string) error {
	defer d.edited()
	t := d.top()
	return t.setErr(path, t.d.SetComment(path, text))
}

// Remove takes out path and everything under it, and says how many
// instances went.
func (d *Doc) Remove(path string) int {
	defer d.edited()
	return d.top().d.Remove(path)
}

func (d *Doc) setErr(path string, st shcl.SetStatus) error {
	switch st {
	case shcl.SetOk:
		return nil
	case shcl.SetBadPath, shcl.SetValueInPath, shcl.SetWildcard, shcl.SetNoSuchIndex, shcl.SetTooDeep,
		shcl.SetMultiple, shcl.SetUnderArray, shcl.SetHasChildren, shcl.SetRawBlock:
		return &shclw.Error{Kind: shclw.ErrBadPath, Path: path, Msg: st.String()}
	}
	return &shclw.Error{Kind: shclw.ErrBadValue, Path: path, Msg: st.String()}
}

var infoEdit = shclw.InfoEdit{Block: shcl.GenBanner, Major: FormatMajor, Values: values}

// values is every field's path and value and the load's error codes, for
// shclw.FinishInfo to tell a comment from a raw body's line.
func values(text string) string {
	d := shcl.Parse(text)
	var b strings.Builder
	for _, f := range d.Fields() {
		b.WriteString(f.Path + "\x00" + f.Value + "\x01")
	}
	for _, x := range d.Diagnostics() {
		if x.Severity == shcl.SeverityError {
			b.WriteString(x.Code + "\x02")
		}
	}
	return b.String()
}

var otherVersion = Diag{Severity: shclw.SevError, Code: "W001", Message: "schema is from another shcl version"}

func tooNew(path string, f int) error {
	return &shclw.Error{Kind: shclw.ErrTooNew, Path: path,
		Msg: fmt.Sprintf("format %d, and shcl %s reads up to %d", f, ShclVersion, FormatMajor)}
}

func strictness(s Strictness) shcl.Strictness {
	switch s {
	case shclw.Loose:
		return shcl.Loose
	case shclw.Strict:
		return shcl.Strict
	}
	return shcl.Standard
}

func status(s shcl.Status) Status {
	switch s {
	case shcl.Good:
		return shclw.Good
	case shcl.Empty:
		return shclw.Empty
	case shcl.NotFound:
		return shclw.NotFound
	case shcl.BadType:
		return shclw.BadType
	case shcl.Multiple:
		return shclw.Multiple
	}
	return shclw.BadPath
}

func got[T any](r shcl.Read[T]) shclw.Got[T] {
	g := shclw.Got[T]{Value: r.Value, Status: status(r.Status), Line: r.Line}
	if r.Raw != nil {
		g.Text = *r.Raw
	}
	return g
}

func diags(in []shcl.Diagnostic) []Diag {
	out := make([]Diag, len(in))
	for i, d := range in {
		sev := shclw.SevHint
		if d.Severity == shcl.SeverityError {
			sev = shclw.SevError
		}
		out[i] = Diag{Line: d.Line, Severity: sev, Code: d.Code, Message: d.Message}
	}
	return out
}

func undiags(in []Diag) []shcl.Diagnostic {
	out := make([]shcl.Diagnostic, len(in))
	for i, d := range in {
		sev := shcl.SeverityHint
		if d.Severity == shclw.SevError {
			sev = shcl.SeverityError
		}
		out[i] = shcl.Diagnostic{Line: d.Line, Severity: sev, Code: d.Code, Message: d.Message}
	}
	return out
}
