"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { api } from "../../lib/api";

type Corpus = { corpus_id: string; name: string; created_at: string };

export default function CorporaPage() {
  const [corpora, setCorpora] = useState<Corpus[]>([]);
  const [name, setName] = useState("");
  const [activeCorpus, setActiveCorpus] = useState<string>("");
  const [files, setFiles] = useState<File[]>([]);
  const [progress, setProgress] = useState<Record<string, string>>({});
  const [papers, setPapers] = useState<Array<{ paper_id: string; filename: string; status: string; fail_reason?: string }>>([]);
  const [uploaded, setUploaded] = useState<Array<{ filename: string; paper_id: string }>>([]);
  const [message, setMessage] = useState<string>("");
  const [error, setError] = useState<string>("");
  const [busy, setBusy] = useState<"" | "create" | "upload" | "ingest">("");

  const selected = useMemo(() => corpora.find((c) => c.corpus_id === activeCorpus), [corpora, activeCorpus]);

  const load = async () => {
    try {
      const res = await api.getCorpora();
      setCorpora(res.corpora);
      if (!activeCorpus && res.corpora[0]) setActiveCorpus(res.corpora[0].corpus_id);
    } catch (e) {
      setError(`Load failed: ${e instanceof Error ? e.message : "unknown error"}`);
    }
  };

  const loadPapers = async (corpusId: string) => {
    try {
      const res = await api.getPapers(corpusId);
      setPapers(res.papers ?? []);
    } catch {
      setPapers([]);
    }
  };

  useEffect(() => { void load(); }, []);

  useEffect(() => {
    if (!activeCorpus) return;
    void loadPapers(activeCorpus);
    const t = setInterval(async () => {
      await loadPapers(activeCorpus);
      try {
        const p = await api.getProgress(activeCorpus);
        setProgress(p.per_paper_status ?? {});
      } catch {
        // ingestion may not be running yet
      }
    }, 2500);
    return () => clearInterval(t);
  }, [activeCorpus]);

  const createCorpus = async () => {
    if (!name.trim()) return;
    setError("");
    setMessage("");
    setBusy("create");
    try {
      const created = await api.createCorpus(name.trim());
      setName("");
      setMessage(`Corpus ready: ${created.name}`);
      await load();
      setActiveCorpus(created.corpus_id);
      await loadPapers(created.corpus_id);
    } catch (e) {
      setError(`Create failed: ${e instanceof Error ? e.message : "unknown error"}`);
    } finally {
      setBusy("");
    }
  };

  const upload = async () => {
    setError("");
    setMessage("");
    if (!activeCorpus) return setError("LF-UI-1001: Select a corpus before uploading.");
    if (files.length === 0) return setError("LF-UI-1002: Pick one or more PDF files.");
    const pdfFiles = files.filter((f) => f.name.toLowerCase().endsWith(".pdf"));
    if (pdfFiles.length === 0) return setError("LF-UI-1004: Only PDF files are accepted.");
    setBusy("upload");
    try {
      const res = await api.uploadPDFs(activeCorpus, pdfFiles);
      setUploaded(res.uploaded);
      setFiles([]);
      setMessage(`Upload complete: ${res.uploaded.length} paper(s) added.`);
      await loadPapers(activeCorpus);
    } catch (e) {
      setError(`Upload failed: ${e instanceof Error ? e.message : "unknown error"}`);
    } finally {
      setBusy("");
    }
  };

  const ingest = async () => {
    setError("");
    setMessage("");
    if (!activeCorpus) return setError("LF-UI-1003: Select a corpus before starting ingest.");
    if (papers.length === 0) return setError("LF-INGEST-1001: Upload at least one PDF before starting ingest.");
    setBusy("ingest");
    try {
      const run = await api.startIngest(activeCorpus);
      setMessage(`Ingest started: ${run.workflow_id}`);
    } catch (e) {
      setError(`Ingest failed: ${e instanceof Error ? e.message : "unknown error"}`);
    } finally {
      setBusy("");
    }
  };

  const doneCount = papers.filter((p) => p.status === "processed").length;
  const failCount = papers.filter((p) => p.status === "failed").length;
  const queueCount = papers.filter((p) => p.status === "pending" || p.status === "processing").length;

  const addFiles = (incoming: File[]) => {
    const merged = [...files, ...incoming];
    const dedup = new Map<string, File>();
    for (const f of merged) {
      dedup.set(`${f.name}:${f.size}:${f.lastModified}`, f);
    }
    setFiles(Array.from(dedup.values()));
  };

  return (
    <main className="mx-auto max-w-6xl p-8">
      <section className="rounded-3xl border border-black/10 bg-white/70 p-8 shadow-lg">
        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Corpus Workspace</p>
        <h1 className="mt-2 text-5xl font-semibold tracking-tight">Build a Focused Corpus</h1>
        <p className="mt-3 max-w-3xl text-zinc-700">Create a corpus per topic, upload PDFs to that corpus, run ingest, then use Search/Survey/Graph to explore evidence. Each corpus is isolated so results stay relevant.</p>
        <div className="mt-5 grid gap-3 text-sm md:grid-cols-3">
          <div className="rounded-2xl border border-black/10 bg-white p-4"><span className="font-semibold">1. Create Corpus</span><p className="mt-1 text-zinc-600">Name by theme, e.g. "RAG evaluation papers".</p></div>
          <div className="rounded-2xl border border-black/10 bg-white p-4"><span className="font-semibold">2. Upload PDFs</span><p className="mt-1 text-zinc-600">Upload one or many PDFs into that corpus.</p></div>
          <div className="rounded-2xl border border-black/10 bg-white p-4"><span className="font-semibold">3. Start Ingest</span><p className="mt-1 text-zinc-600">Temporal processes files and enables search.</p></div>
        </div>
      </section>

      <div className="mt-8 grid gap-8 md:grid-cols-2">
        <section className="rounded-3xl border border-black/10 bg-white/85 p-6">
          <h2 className="text-xl font-semibold">Create & Select Corpus</h2>
          <div className="mt-4 flex gap-2">
            <input className="w-full rounded-xl border border-black/20 bg-white px-4 py-2" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g., Graph Neural Networks" />
            <button disabled={busy !== ""} className="rounded-xl bg-ink px-4 py-2 text-white disabled:opacity-50" onClick={createCorpus}>{busy === "create" ? "Creating..." : "Create"}</button>
          </div>

          <div className="mt-5 flex items-center justify-between">
            <h3 className="text-xs uppercase tracking-[0.2em] text-zinc-500">Available Corpora</h3>
            <span className="text-xs text-zinc-500">{corpora.length} total</span>
          </div>

          <div className="mt-3 max-h-72 space-y-2 overflow-auto">
            {corpora.map((c) => (
              <button key={c.corpus_id} onClick={() => setActiveCorpus(c.corpus_id)} className={`w-full rounded-xl border px-3 py-3 text-left transition ${activeCorpus === c.corpus_id ? "border-ink bg-ink/5" : "border-black/10 hover:bg-black/5"}`}>
                <div className="font-medium">{c.name}</div>
                <div className="mt-1 text-xs text-zinc-500">{c.corpus_id}</div>
              </button>
            ))}
          </div>
        </section>

        <section className="rounded-3xl border border-black/10 bg-white/85 p-6">
          <h2 className="text-xl font-semibold">Upload & Ingest</h2>
          <p className="mt-1 text-sm text-zinc-600">Selected corpus: <span className="font-medium text-zinc-900">{selected?.name ?? "none"}</span></p>

          <label
            className="mt-4 block rounded-2xl border border-dashed border-black/25 bg-gradient-to-br from-white to-zinc-50 p-4 transition hover:border-black/40"
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => {
              e.preventDefault();
              addFiles(Array.from(e.dataTransfer.files ?? []));
            }}
          >
            <input type="file" accept="application/pdf" multiple className="hidden" onChange={(e) => addFiles(Array.from(e.target.files ?? []))} />
            <div className="text-sm font-medium">Drop PDFs here or click to pick files</div>
            <div className="mt-2 text-xs text-zinc-500">{files.length} selected (multi-file supported)</div>
          </label>

          <div className="mt-4 flex gap-2">
            <button disabled={busy !== "" || !activeCorpus || files.length === 0} className="rounded-xl border border-black/20 px-4 py-2 disabled:opacity-50" onClick={upload}>{busy === "upload" ? "Uploading..." : "Upload"}</button>
            <button disabled={busy !== "" || !activeCorpus || papers.length === 0} className="rounded-xl bg-teal px-4 py-2 text-white disabled:opacity-50" onClick={ingest}>{busy === "ingest" ? "Starting..." : "Start Ingest"}</button>
          </div>

          {message && <p className="mt-3 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{message}</p>}
          {error && <p className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>}

          {files.length > 0 && (
            <div className="mt-3 rounded-xl border border-black/10 bg-white p-3 text-sm">
              <p className="font-medium">Selected Files</p>
              <div className="mt-2 max-h-24 space-y-1 overflow-auto">
                {files.map((f) => (
                  <div className="flex items-center justify-between" key={`${f.name}-${f.size}-${f.lastModified}`}>
                    <span className="truncate pr-3">{f.name}</span>
                    <span className="text-xs text-zinc-500">{Math.ceil(f.size / 1024)} KB</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {uploaded.length > 0 && (
            <div className="mt-3 rounded-xl border border-black/10 bg-white p-3 text-sm">
              <p className="font-medium">Uploaded Files</p>
              <div className="mt-2 max-h-28 space-y-1 overflow-auto">
                {uploaded.map((u) => (
                  <div className="flex items-center justify-between" key={u.paper_id}>
                    <span>{u.filename}</span>
                    <span className="text-xs text-zinc-500">{u.paper_id.slice(0, 12)}...</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="mt-4 grid grid-cols-3 gap-2 text-center text-xs">
            <div className="rounded-xl border border-black/10 bg-white p-2"><div className="text-zinc-500">Processed</div><div className="text-lg font-semibold">{doneCount}</div></div>
            <div className="rounded-xl border border-black/10 bg-white p-2"><div className="text-zinc-500">Failed</div><div className="text-lg font-semibold">{failCount}</div></div>
            <div className="rounded-xl border border-black/10 bg-white p-2"><div className="text-zinc-500">Queued</div><div className="text-lg font-semibold">{queueCount}</div></div>
          </div>

          <div className="mt-4 max-h-56 overflow-auto rounded-xl border border-black/10 bg-white p-3 text-sm">
            {papers.length === 0 && <p className="text-zinc-500">No papers uploaded yet.</p>}
            {papers.map((paper) => (
              <div className="flex items-center justify-between border-b border-black/5 py-2" key={paper.paper_id}>
                <span className="truncate pr-3">{paper.filename}</span>
                <span className="rounded-full border border-black/20 px-2 py-0.5 text-xs">{paper.status}</span>
              </div>
            ))}
            {Object.entries(progress).map(([paper, status]) => (
              <div className="hidden" key={`${paper}-${status}`}>
                {paper}:{status}
              </div>
            ))}
          </div>

          {activeCorpus && (
            <div className="mt-4 flex flex-wrap gap-2 text-sm">
              <Link className="rounded-full border border-black/20 bg-white px-4 py-2 hover:bg-black/5" href={`/corpora/${activeCorpus}/search`}>Open Semantic Search</Link>
              <Link className="rounded-full border border-black/20 bg-white px-4 py-2 hover:bg-black/5" href={`/corpora/${activeCorpus}/survey`}>Open Survey Builder</Link>
              <Link className="rounded-full border border-black/20 bg-white px-4 py-2 hover:bg-black/5" href={`/corpora/${activeCorpus}/graph`}>Open Knowledge Graph</Link>
            </div>
          )}
        </section>
      </div>
    </main>
  );
}
