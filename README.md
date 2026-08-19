# PDF Toolkit

Merge, split, reorder, rotate, compress, and password-protect PDFs — entirely
on this machine. Opens as its own window.

No accounts, no upload, nothing sent anywhere: [pdfcpu](https://github.com/pdfcpu/pdfcpu)
is compiled directly into this binary, so there's no external tool to
install and no file ever leaves your computer.

## Requirements

**A Chromium-based browser already installed**: Google Chrome, Chromium,
Brave, Microsoft Edge, or Arc. PDF Toolkit renders your PDF's pages and its
own UI inside it; it doesn't install or bundle a browser itself. If none is
found, it tells you on launch instead of failing silently.

## Use

1. Open PDF Toolkit — it opens its own window.
2. Drop one PDF to work on its pages, or several at once to merge them.

**Working on one PDF** opens a page workspace: drag thumbnails to reorder,
click ↻ to rotate a page, click ✕ to drop it from the export, then
**Export**. **Split…**, **Compress**, and **Add password…** are separate,
one-click actions that each act on the original file, not your edits above.
A password-protected PDF can't be previewed — enter its password to remove
protection first.

**Dropping several PDFs** shows a reorderable file list and a single
**Merge** button.

Results are saved to your Downloads folder; "Open file" and "Show in
folder" appear as soon as a job finishes.

## License

MIT — see [LICENSE](LICENSE).
