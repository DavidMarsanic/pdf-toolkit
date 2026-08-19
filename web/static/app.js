import * as pdfjsLib from "/vendor/pdfjs/pdf.mjs";

pdfjsLib.GlobalWorkerOptions.workerSrc = "/vendor/pdfjs/pdf.worker.mjs";

const THUMB_WIDTH = 180; // CSS px, at devicePixelRatio scale — see renderPageThumb

// ---- DOM refs -------------------------------------------------------------

const dropZone = document.querySelector("#dropZone");
const fileInput = document.querySelector("#fileInput");
const errorEl = document.querySelector("#error");

const lockedSection = document.querySelector("#lockedSection");
const unlockPassword = document.querySelector("#unlockPassword");
const unlockBtn = document.querySelector("#unlockBtn");

const workspace = document.querySelector("#workspace");
const workspaceFilename = document.querySelector("#workspaceFilename");
const pageGrid = document.querySelector("#pageGrid");
const startOverBtn = document.querySelector("#startOver");
const exportBtn = document.querySelector("#exportBtn");
const splitBtn = document.querySelector("#splitBtn");
const splitPanel = document.querySelector("#splitPanel");
const splitSpan = document.querySelector("#splitSpan");
const splitConfirm = document.querySelector("#splitConfirm");
const splitCancel = document.querySelector("#splitCancel");
const compressBtn = document.querySelector("#compressBtn");
const protectBtn = document.querySelector("#protectBtn");
const protectPanel = document.querySelector("#protectPanel");
const protectPassword = document.querySelector("#protectPassword");
const protectConfirm = document.querySelector("#protectConfirm");
const protectCancel = document.querySelector("#protectCancel");

const mergeSection = document.querySelector("#mergeSection");
const mergeCount = document.querySelector("#mergeCount");
const mergeList = document.querySelector("#mergeList");
const mergeStartOverBtn = document.querySelector("#mergeStartOver");
const mergeBtn = document.querySelector("#mergeBtn");

const progressEl = document.querySelector("#progress");
const progressLabel = document.querySelector("#progressLabel");
const doneActions = document.querySelector("#doneActions");
const doneMessage = document.querySelector("#doneMessage");
const openFileBtn = document.querySelector("#openFile");
const showFolderBtn = document.querySelector("#showFolder");

// ---- state ------------------------------------------------------------

let currentFile = null; // File, single-file workspace mode
let mergeFiles = []; // File[], indexed by data-file-index on merge-item elements
let lastResultPath = "";

// ---- generic helpers ----------------------------------------------------

function showError(message) {
  errorEl.textContent = message;
  errorEl.classList.remove("hidden");
}

function clearError() {
  errorEl.classList.add("hidden");
  errorEl.textContent = "";
}

function hideAllSections() {
  lockedSection.classList.add("hidden");
  workspace.classList.add("hidden");
  mergeSection.classList.add("hidden");
  progressEl.classList.add("hidden");
  doneActions.classList.add("hidden");
  splitPanel.classList.add("hidden");
  protectPanel.classList.add("hidden");
}

function resetToDropZone() {
  hideAllSections();
  clearError();
  currentFile = null;
  mergeFiles = [];
  pageGrid.innerHTML = "";
  mergeList.innerHTML = "";
  fileInput.value = "";
  dropZone.classList.remove("hidden");
}

// ---- drop zone ------------------------------------------------------------

dropZone.addEventListener("click", () => fileInput.click());
fileInput.addEventListener("change", () => handleFiles(fileInput.files));

["dragenter", "dragover"].forEach((evt) =>
  dropZone.addEventListener(evt, (e) => {
    e.preventDefault();
    dropZone.classList.add("dragover");
  })
);
["dragleave", "drop"].forEach((evt) =>
  dropZone.addEventListener(evt, (e) => {
    e.preventDefault();
    dropZone.classList.remove("dragover");
  })
);
dropZone.addEventListener("drop", (e) => {
  if (e.dataTransfer?.files?.length) handleFiles(e.dataTransfer.files);
});

function handleFiles(fileList) {
  const files = Array.from(fileList).filter((f) => f.type === "application/pdf" || f.name.toLowerCase().endsWith(".pdf"));
  if (files.length === 0) {
    showError("Only PDF files are supported.");
    return;
  }
  clearError();
  dropZone.classList.add("hidden");
  if (files.length === 1) {
    loadSingleFile(files[0]);
  } else {
    loadMergeFiles(files);
  }
}

// ---- single-file workspace ------------------------------------------------

async function loadSingleFile(file) {
  currentFile = file;
  hideAllSections();

  let pdfDoc;
  try {
    const buf = await file.arrayBuffer();
    pdfDoc = await pdfjsLib.getDocument({ data: buf }).promise;
  } catch (err) {
    if (err?.name === "PasswordException") {
      lockedSection.classList.remove("hidden");
      return;
    }
    showError("Couldn't open that PDF: " + (err?.message ?? err));
    dropZone.classList.remove("hidden");
    return;
  }

  workspaceFilename.textContent = file.name;
  pageGrid.innerHTML = "";
  workspace.classList.remove("hidden");

  const tiles = [];
  for (let i = 1; i <= pdfDoc.numPages; i++) {
    const tile = createPageTile(i);
    pageGrid.appendChild(tile);
    tiles.push(tile);
  }
  enableDragReorder(pageGrid, ".page-tile");

  // Render thumbnails after the grid is in the DOM so layout/scroll
  // position settles immediately — rendering itself happens page by page,
  // in the background, so a large document doesn't block the UI up front.
  for (let i = 1; i <= pdfDoc.numPages; i++) {
    renderPageThumb(pdfDoc, i, tiles[i - 1]).catch(() => {
      // A single page failing to render (rare, malformed content stream)
      // shouldn't block the rest of the grid — the tile just keeps its
      // blank placeholder and can still be exported.
    });
  }
}

function createPageTile(pageNum) {
  const tile = document.createElement("div");
  tile.className = "page-tile";
  tile.draggable = true;
  tile.dataset.sourcePage = String(pageNum);
  tile.dataset.rotate = "0";

  const canvas = document.createElement("canvas");
  tile.appendChild(canvas);

  const label = document.createElement("span");
  label.className = "page-number";
  label.textContent = String(pageNum);
  tile.appendChild(label);

  const controls = document.createElement("div");
  controls.className = "tile-controls";

  const rotateBtn = document.createElement("button");
  rotateBtn.type = "button";
  rotateBtn.className = "tile-btn rotate-btn";
  rotateBtn.textContent = "↻";
  rotateBtn.title = "Rotate 90°";
  // draggable=false alone doesn't reliably stop a draggable ancestor from
  // hijacking the gesture in every engine — bailing out of the tile's own
  // dragstart (see enableDragReorder) is what actually prevents a press-
  // and-slightly-move on this button from starting a tile drag instead of
  // registering as a click.
  rotateBtn.draggable = false;
  rotateBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    const next = (parseInt(tile.dataset.rotate, 10) + 90) % 360;
    tile.dataset.rotate = String(next);
    canvas.style.transform = `rotate(${next}deg)`;
  });
  controls.appendChild(rotateBtn);

  const removeBtn = document.createElement("button");
  removeBtn.type = "button";
  removeBtn.className = "tile-btn remove-btn";
  removeBtn.textContent = "✕";
  removeBtn.title = "Drop this page from the export";
  removeBtn.draggable = false;
  removeBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    tile.classList.toggle("removed");
    tile.dataset.removed = tile.classList.contains("removed") ? "1" : "";
    removeBtn.title = tile.classList.contains("removed") ? "Restore this page" : "Drop this page from the export";
  });
  controls.appendChild(removeBtn);

  tile.appendChild(controls);
  return tile;
}

async function renderPageThumb(pdfDoc, pageNum, tile) {
  const page = await pdfDoc.getPage(pageNum);
  const unscaled = page.getViewport({ scale: 1 });
  const dpr = window.devicePixelRatio || 1;
  const scale = (THUMB_WIDTH * dpr) / unscaled.width;
  const viewport = page.getViewport({ scale });

  const canvas = tile.querySelector("canvas");
  canvas.width = viewport.width;
  canvas.height = viewport.height;
  canvas.style.width = THUMB_WIDTH + "px";
  canvas.style.height = (THUMB_WIDTH * viewport.height) / viewport.width + "px";

  const ctx = canvas.getContext("2d");
  await page.render({ canvasContext: ctx, viewport }).promise;
}

// ---- drag-and-drop reorder (shared by page grid + merge list) -------------

function enableDragReorder(container, itemSelector) {
  let dragged = null;

  container.addEventListener("dragstart", (e) => {
    // A press-and-slightly-move on a control button (rotate/remove) reads
    // to the browser as the start of a drag gesture unless explicitly
    // rejected here — without this, that button's own click handler never
    // fires and the tile silently starts dragging instead.
    if (e.target.closest(".tile-btn")) {
      e.preventDefault();
      return;
    }
    const item = e.target.closest(itemSelector);
    if (!item) return;
    dragged = item;
    item.classList.add("dragging");
    e.dataTransfer.effectAllowed = "move";
  });

  container.addEventListener("dragend", () => {
    dragged?.classList.remove("dragging");
    container.querySelectorAll(".drop-before, .drop-after").forEach((el) => {
      el.classList.remove("drop-before", "drop-after");
    });
    dragged = null;
  });

  container.addEventListener("dragover", (e) => {
    const over = e.target.closest(itemSelector);
    if (!over || over === dragged) return;
    e.preventDefault();
    container.querySelectorAll(".drop-before, .drop-after").forEach((el) => {
      el.classList.remove("drop-before", "drop-after");
    });
    const rect = over.getBoundingClientRect();
    const before = e.clientX - rect.left < rect.width / 2;
    over.classList.add(before ? "drop-before" : "drop-after");
  });

  container.addEventListener("drop", (e) => {
    const over = e.target.closest(itemSelector);
    if (!over || !dragged || over === dragged) return;
    e.preventDefault();
    const rect = over.getBoundingClientRect();
    const before = e.clientX - rect.left < rect.width / 2;
    container.insertBefore(dragged, before ? over : over.nextSibling);
  });
}

// ---- export (reorder/delete/rotate) ----------------------------------------

function collectExportOps() {
  const ops = [];
  for (const tile of pageGrid.querySelectorAll(".page-tile")) {
    if (tile.classList.contains("removed")) continue;
    ops.push({
      sourcePage: parseInt(tile.dataset.sourcePage, 10),
      rotate: parseInt(tile.dataset.rotate, 10),
    });
  }
  return ops;
}

exportBtn.addEventListener("click", () => {
  const ops = collectExportOps();
  if (ops.length === 0) {
    showError("At least one page must stay in the export.");
    return;
  }
  runJob("export", { ops }, [currentFile]);
});

// ---- split ------------------------------------------------------------

splitBtn.addEventListener("click", () => {
  protectPanel.classList.add("hidden");
  splitPanel.classList.toggle("hidden");
});
splitCancel.addEventListener("click", () => splitPanel.classList.add("hidden"));
splitConfirm.addEventListener("click", () => {
  const span = Math.max(1, parseInt(splitSpan.value, 10) || 1);
  runJob("split", { span }, [currentFile]);
});

// ---- compress ------------------------------------------------------------

compressBtn.addEventListener("click", () => {
  runJob("compress", {}, [currentFile]);
});

// ---- protect / unprotect ----------------------------------------------

protectBtn.addEventListener("click", () => {
  splitPanel.classList.add("hidden");
  protectPanel.classList.toggle("hidden");
});
protectCancel.addEventListener("click", () => protectPanel.classList.add("hidden"));
protectConfirm.addEventListener("click", () => {
  if (!protectPassword.value) {
    showError("Enter a password first.");
    return;
  }
  runJob("protect", { password: protectPassword.value }, [currentFile]);
});

unlockBtn.addEventListener("click", () => {
  if (!unlockPassword.value) {
    showError("Enter the document's password first.");
    return;
  }
  runJob("unprotect", { password: unlockPassword.value }, [currentFile]);
});

// ---- merge ------------------------------------------------------------

function loadMergeFiles(files) {
  mergeFiles = files;
  mergeList.innerHTML = "";
  mergeCount.textContent = String(files.length);

  files.forEach((file, i) => {
    const li = document.createElement("li");
    li.className = "merge-item";
    li.draggable = true;
    li.dataset.fileIndex = String(i);

    const handle = document.createElement("span");
    handle.className = "drag-handle";
    handle.textContent = "⠿";
    li.appendChild(handle);

    const name = document.createElement("span");
    name.className = "merge-name";
    name.textContent = file.name;
    li.appendChild(name);

    mergeList.appendChild(li);
  });

  enableDragReorder(mergeList, ".merge-item");
  mergeSection.classList.remove("hidden");
}

mergeBtn.addEventListener("click", () => {
  const ordered = Array.from(mergeList.querySelectorAll(".merge-item")).map(
    (li) => mergeFiles[parseInt(li.dataset.fileIndex, 10)]
  );
  runJob("merge", {}, ordered);
});

// ---- start over ------------------------------------------------------------

startOverBtn.addEventListener("click", resetToDropZone);
mergeStartOverBtn.addEventListener("click", resetToDropZone);

// ---- job submission + SSE progress ----------------------------------------

async function runJob(operation, params, files) {
  hideProgressAndDone();
  clearError();

  const form = new FormData();
  form.set("operation", operation);
  form.set("params", JSON.stringify(params));
  for (const file of files) form.append("file", file, file.name);

  progressEl.classList.remove("hidden");
  progressLabel.textContent = "Uploading…";

  let jobId;
  try {
    const res = await fetch("/api/jobs", { method: "POST", body: form });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || "request failed");
    jobId = data.jobId;
  } catch (err) {
    progressEl.classList.add("hidden");
    showError(String(err.message || err));
    return;
  }

  const source = new EventSource(`/api/jobs/${jobId}/events`);
  source.onmessage = (event) => {
    const payload = JSON.parse(event.data);
    if (payload.stage === "processing") {
      progressLabel.textContent = "Processing…";
    } else if (payload.stage === "done") {
      source.close();
      progressEl.classList.add("hidden");
      showDone(payload.path, payload.filename);
    } else if (payload.stage === "error") {
      source.close();
      progressEl.classList.add("hidden");
      showError(payload.code === "wrong-password" ? "That password didn't work." : payload.message);
    } else if (payload.stage === "canceled") {
      source.close();
      progressEl.classList.add("hidden");
    }
  };
  source.onerror = () => {
    source.close();
    progressEl.classList.add("hidden");
    showError("Lost connection to the local server.");
  };
}

function hideProgressAndDone() {
  progressEl.classList.add("hidden");
  doneActions.classList.add("hidden");
}

function showDone(path, filename) {
  lastResultPath = path;
  doneMessage.textContent = `Saved ${filename} to Downloads.`;
  doneActions.classList.remove("hidden");
}

openFileBtn.addEventListener("click", () => {
  fetch("/api/open", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path: lastResultPath }) });
});
showFolderBtn.addEventListener("click", () => {
  fetch("/api/reveal", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path: lastResultPath }) });
});
