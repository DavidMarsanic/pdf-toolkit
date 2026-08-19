package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PageCount reports how many pages inFile has — used server-side only to
// validate an export's page list; the workspace UI itself gets page count
// and thumbnails straight from PDF.js in the browser, no round trip needed.
func PageCount(path string) (int, error) {
	n, err := api.PageCountFile(path)
	if err != nil {
		return 0, fmt.Errorf("reading page count: %w", err)
	}
	return n, nil
}

// Merge combines inFiles, in the given order, into one PDF at outFile.
func Merge(inFiles []string, outFile string) error {
	if len(inFiles) < 2 {
		return fmt.Errorf("merge needs at least two files")
	}
	if err := api.MergeCreateFile(inFiles, outFile, false, model.NewDefaultConfiguration()); err != nil {
		return fmt.Errorf("merging: %w", err)
	}
	return nil
}

// Split writes one PDF per `span`-page run into outDir (span=1 means one
// file per page), returning the files written, in page order.
func Split(inFile, outDir string, span int) ([]string, error) {
	if span < 1 {
		span = 1
	}
	if err := api.SplitFile(inFile, outDir, span, model.NewDefaultConfiguration()); err != nil {
		return nil, fmt.Errorf("splitting: %w", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("reading split output: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, filepath.Join(outDir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// Export writes a new PDF at outFile built from ops, in order: each op
// pulls one page from inFile (by its original 1-based page number) and
// optionally rotates it. This single call is what the workspace UI's
// reorder/delete/rotate/export screen drives — omitting a page number
// deletes it, repeating one duplicates it, and the slice order IS the
// output order, so drag-reorder in the browser maps directly onto ops.
func Export(inFile string, ops []PageOp, outFile string) error {
	if len(ops) == 0 {
		return ErrEmptyInput
	}

	selected := make([]string, len(ops))
	for i, op := range ops {
		selected[i] = strconv.Itoa(op.SourcePage)
	}
	conf := model.NewDefaultConfiguration()
	if err := api.CollectFile(inFile, outFile, selected, conf); err != nil {
		return fmt.Errorf("reordering pages: %w", err)
	}

	// Collect preserves the order we gave it, so ops[i] now lives at
	// 1-based output page i+1 — group by rotation angle since RotateFile
	// applies one angle per call, and chain through a temp file so
	// multiple angles in one export compose correctly.
	byAngle := map[int][]string{}
	for i, op := range ops {
		if op.Rotate%360 == 0 {
			continue
		}
		byAngle[op.Rotate] = append(byAngle[op.Rotate], strconv.Itoa(i+1))
	}
	for angle, pages := range byAngle {
		tmp := outFile + ".rotating"
		if err := api.RotateFile(outFile, tmp, angle, pages, conf); err != nil {
			return fmt.Errorf("rotating pages: %w", err)
		}
		if err := os.Rename(tmp, outFile); err != nil {
			return fmt.Errorf("finalizing rotation: %w", err)
		}
	}
	return nil
}

// Compress rewrites the PDF with pdfcpu's optimize pass (dedupes shared
// resources, drops redundant objects). Lossless — pdfcpu doesn't
// recompress embedded images, so this won't shrink an already-optimized
// or image-heavy scan much; that's a real limitation, not a bug.
func Compress(inFile, outFile string) error {
	if err := api.OptimizeFile(inFile, outFile, model.NewDefaultConfiguration()); err != nil {
		return fmt.Errorf("compressing: %w", err)
	}
	return nil
}

// Protect encrypts outFile (AES-256) so it requires password to open.
func Protect(inFile, outFile, password string) error {
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("password is required")
	}
	conf := model.NewAESConfiguration(password, password, 256)
	if err := api.EncryptFile(inFile, outFile, conf); err != nil {
		return fmt.Errorf("adding password: %w", err)
	}
	return nil
}

// Unprotect removes password protection, given the correct current password.
func Unprotect(inFile, outFile, password string) error {
	conf := model.NewDefaultConfiguration()
	conf.UserPW = password
	if err := api.DecryptFile(inFile, outFile, conf); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "password") {
			return ErrWrongPassword
		}
		return fmt.Errorf("removing password: %w", err)
	}
	return nil
}
