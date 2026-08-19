package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidMarsanic/pdf-toolkit/internal/browser"
	"github.com/DavidMarsanic/pdf-toolkit/internal/engine"
	"github.com/DavidMarsanic/pdf-toolkit/internal/jobs"
	"github.com/DavidMarsanic/pdf-toolkit/internal/paths"
)

type exportParams struct {
	Ops []engine.PageOp `json:"ops"`
}

type splitParams struct {
	Span int `json:"span"`
}

type passwordParams struct {
	Password string `json:"password"`
}

// handleCreateJob accepts a multipart upload (one or more "file" parts,
// an "operation" field, and a "params" JSON field) and runs the matching
// pdfcpu operation in the background, reporting progress over the same
// job/SSE mechanism as every other Securexe applet.
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload", "code": "bad-request"})
		return
	}

	operation := r.FormValue("operation")
	rawParams := r.FormValue("params")

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file uploaded", "code": "bad-request"})
		return
	}

	job, ctx := s.Jobs.Create(s.ctx)
	scratch, err := paths.ScratchDir(job.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	staged, baseName, err := saveUploads(files, scratch)
	if err != nil {
		os.RemoveAll(scratch)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "bad-request"})
		return
	}

	go runJob(ctx, job, operation, rawParams, staged, baseName, scratch, s.DefaultOutputDir)

	writeJSON(w, http.StatusOK, map[string]string{"jobId": job.ID})
}

func saveUploads(files []*multipart.FileHeader, scratch string) (staged []string, baseName string, err error) {
	for i, fh := range files {
		src, err := fh.Open()
		if err != nil {
			return nil, "", fmt.Errorf("reading upload: %w", err)
		}

		name := sanitizeFilename(fh.Filename)
		if name == "" {
			name = fmt.Sprintf("input-%d.pdf", i)
		}
		dst := filepath.Join(scratch, fmt.Sprintf("%02d-%s", i, name))
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			return nil, "", fmt.Errorf("staging upload: %w", err)
		}
		_, copyErr := io.Copy(out, src)
		src.Close()
		out.Close()
		if copyErr != nil {
			return nil, "", fmt.Errorf("staging upload: %w", copyErr)
		}

		staged = append(staged, dst)
		if i == 0 {
			baseName = strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
	return staged, baseName, nil
}

// sanitizeFilename strips any directory component and rejects "." / ".."
// so a crafted multipart filename can never escape the scratch directory.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		return ""
	}
	return name
}

func runJob(ctx context.Context, job *jobs.Job, operation, rawParams string, inputs []string, baseName, scratch, outDir string) {
	defer os.RemoveAll(scratch)

	job.Publish(jobs.Event{Stage: "processing", Percent: 20})

	result, err := dispatch(operation, rawParams, inputs, baseName, scratch, outDir)

	if ctx.Err() != nil {
		job.Publish(jobs.Event{Stage: "canceled"})
		return
	}
	if err != nil {
		code := "error"
		if errors.Is(err, engine.ErrWrongPassword) {
			code = "wrong-password"
		}
		job.Publish(jobs.Event{Stage: "error", Message: err.Error(), Code: code})
		return
	}
	job.Publish(jobs.Event{Stage: "done", Percent: 100, Path: result.Path, Filename: result.Filename})
}

func dispatch(operation, rawParams string, inputs []string, baseName, scratch, outDir string) (*engine.Result, error) {
	switch operation {
	case "merge":
		return runMerge(inputs, outDir)
	case "split":
		var p splitParams
		if err := json.Unmarshal([]byte(rawParams), &p); err != nil {
			return nil, fmt.Errorf("invalid split parameters")
		}
		return runSplit(inputs[0], baseName, p, scratch, outDir)
	case "export":
		var p exportParams
		if err := json.Unmarshal([]byte(rawParams), &p); err != nil {
			return nil, fmt.Errorf("invalid export parameters")
		}
		return runExport(inputs[0], baseName, p, outDir)
	case "compress":
		return runCompress(inputs[0], baseName, outDir)
	case "protect":
		var p passwordParams
		if err := json.Unmarshal([]byte(rawParams), &p); err != nil {
			return nil, fmt.Errorf("invalid password parameters")
		}
		return runProtect(inputs[0], baseName, p, outDir)
	case "unprotect":
		var p passwordParams
		if err := json.Unmarshal([]byte(rawParams), &p); err != nil {
			return nil, fmt.Errorf("invalid password parameters")
		}
		return runUnprotect(inputs[0], baseName, p, outDir)
	default:
		return nil, fmt.Errorf("unknown operation %q", operation)
	}
}

func runMerge(inputs []string, outDir string) (*engine.Result, error) {
	out := uniqueOutputPath(outDir, "merged", "", "pdf")
	if err := engine.Merge(inputs, out); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

func runSplit(input, baseName string, p splitParams, scratch, outDir string) (*engine.Result, error) {
	splitDir := filepath.Join(scratch, "split")
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		return nil, err
	}
	files, err := engine.Split(input, splitDir, p.Span)
	if err != nil {
		return nil, err
	}
	out := uniqueOutputPath(outDir, baseName, "-split", "zip")
	if err := zipFiles(files, out); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

func runExport(input, baseName string, p exportParams, outDir string) (*engine.Result, error) {
	out := uniqueOutputPath(outDir, baseName, "-edited", "pdf")
	if err := engine.Export(input, p.Ops, out); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

func runCompress(input, baseName, outDir string) (*engine.Result, error) {
	out := uniqueOutputPath(outDir, baseName, "-compressed", "pdf")
	if err := engine.Compress(input, out); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

func runProtect(input, baseName string, p passwordParams, outDir string) (*engine.Result, error) {
	out := uniqueOutputPath(outDir, baseName, "-protected", "pdf")
	if err := engine.Protect(input, out, p.Password); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

func runUnprotect(input, baseName string, p passwordParams, outDir string) (*engine.Result, error) {
	out := uniqueOutputPath(outDir, baseName, "-unlocked", "pdf")
	if err := engine.Unprotect(input, out, p.Password); err != nil {
		return nil, err
	}
	return &engine.Result{Path: out, Filename: filepath.Base(out)}, nil
}

// uniqueOutputPath never silently overwrites something already sitting in
// Downloads — same collision-avoidance shape Finder itself uses.
func uniqueOutputPath(dir, stem, suffix, ext string) string {
	candidate := filepath.Join(dir, fmt.Sprintf("%s%s.%s", stem, suffix, ext))
	for i := 2; fileExists(candidate); i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s%s (%d).%s", stem, suffix, i, ext))
	}
	return candidate
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func zipFiles(files []string, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, f := range files {
		if err := addToZip(zw, f); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

func addToZip(zw *zip.Writer, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	defer src.Close()
	dst, err := zw.Create(filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	job, ok := s.Jobs.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := job.Subscribe()
	defer cancel()

	for {
		select {
		case e, open := <-ch:
			if !open {
				return
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if e.Stage == "done" || e.Stage == "error" || e.Stage == "canceled" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := browser.Reveal(req.Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := browser.Open(req.Path); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body", "code": "bad-request"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
