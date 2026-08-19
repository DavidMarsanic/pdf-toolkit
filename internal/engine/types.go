// Package engine wraps pdfcpu (a pure-Go PDF library — no external binary,
// no cgo) behind a small, UI-agnostic interface: Merge, Split, Reorder,
// Rotate, Compress, Protect, Unprotect. Unlike video-clipper/instaframe,
// this tool has no missing-dependency case to handle — pdfcpu is compiled
// directly into this binary.
package engine

import "errors"

// PageOp describes what happens to one page of the source document in an
// Export (Reorder) call: which original page to pull, and how much to
// rotate it by (0 if unchanged). The order of the slice IS the output
// page order — omitting a page number deletes it, repeating one
// duplicates it.
type PageOp struct {
	SourcePage int `json:"sourcePage"`
	Rotate     int `json:"rotate"` // degrees, must be a multiple of 90
}

// Result describes the file (or, for Split, the zip of files) written to
// disk.
type Result struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
}

var (
	// ErrWrongPassword is returned by Unprotect when the supplied password
	// doesn't open the document — classified from pdfcpu's own error text,
	// same heuristic style as video-clipper's classifyError.
	ErrWrongPassword = errors.New("wrong password")
	ErrEmptyInput    = errors.New("no pages selected")
)
