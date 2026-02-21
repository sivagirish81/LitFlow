"use client";

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import { CorpusNav } from "../../../../components/corpus-nav";
import { api } from "../../../../lib/api";

const EMBED_PROVIDER_KEY = "litflow.embedProvider";
const EMBED_VERSION_KEY = "litflow.embedVersion";

type Citation = {
  ref_id: string;
  paper_id: string;
  title: string;
  filename?: string;
  paper_url?: string;
  chunk_id: string;
  snippet: string;
  summary?: string;
  score: number;
};

export default function SearchPage({ params }: { params: { corpusId: string } }) {
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [citations, setCitations] = useState<Citation[]>([]);
  const [embedProvider, setEmbedProvider] = useState("mock");
  const [embedVersion, setEmbedVersion] = useState("v1");

  useEffect(() => {
    if (typeof window !== "undefined") {
      setEmbedProvider(localStorage.getItem(EMBED_PROVIDER_KEY) || "mock");
      setEmbedVersion(localStorage.getItem(EMBED_VERSION_KEY) || "mock-v1");
    }
  }, []);

  const ask = async () => {
    if (!question.trim()) {
      setError("LF-UI-2001: Enter a question first.");
      return;
    }
    setBusy(true);
    setError("");
    setStatus("Querying corpus and retrieving citations...");
    try {
      const res = await api.ask({
        corpus_id: params.corpusId,
        question,
        top_k: 8,
        embed_provider: embedProvider,
        embed_version: embedVersion,
      });
      setAnswer(linkifyCitationRefs(res.answer));
      setCitations(res.citations);
      if (res.citations.length === 0) {
        setStatus("LF-SEARCH-0001: No indexed chunks found. Upload PDFs and complete ingest for this corpus.");
      } else {
        setStatus(`Retrieved ${res.citations.length} citation(s).`);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "LF-UI-2002: Search failed.");
      setStatus("");
    } finally {
      setBusy(false);
    }
  };

  const presets = [
    "What are the central findings across these papers?",
    "Compare methods and reported metrics.",
    "What limitations are repeatedly mentioned?"
  ];

  return (
    <main className="mx-auto max-w-6xl p-8">
      <section className="rounded-3xl border border-black/10 bg-white/80 p-8 shadow-lg">
        <p className="text-xs uppercase tracking-[0.22em] text-zinc-500">Corpus Search</p>
        <h1 className="mt-2 text-5xl font-semibold tracking-tight">Ask Evidence-Grounded Questions</h1>
        <p className="mt-3 max-w-3xl text-zinc-700">Search works best after ingestion completes. Ask focused questions, then inspect citations to verify source evidence quickly.</p>
        <CorpusNav corpusId={params.corpusId} current="search" />
      </section>

      <section className="mt-6 grid gap-6 md:grid-cols-[1.15fr_0.85fr]">
        <div className="rounded-3xl border border-black/10 bg-white/85 p-6">
          <h2 className="text-xl font-semibold">Question</h2>
          <textarea
            className="mt-3 h-32 w-full rounded-2xl border border-black/20 bg-white p-4 text-sm outline-none transition focus:border-ink"
            placeholder="Ask a question about this corpus..."
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
          />
          <div className="mt-3 flex flex-wrap gap-2">
            {presets.map((p) => (
              <button key={p} onClick={() => setQuestion(p)} className="rounded-full border border-black/20 px-3 py-1 text-xs hover:bg-black/5">{p}</button>
            ))}
          </div>
          <div className="mt-4 flex items-center gap-2">
            <button disabled={busy} className="rounded-xl bg-ink px-6 py-2 text-white disabled:opacity-50" onClick={ask}>{busy ? "Searching..." : "Ask LitFlow"}</button>
            {status && <span className="text-sm text-zinc-600">{status}</span>}
          </div>
          <p className="mt-2 text-xs text-zinc-500">Embedding profile: <span className="font-medium text-zinc-700">{embedProvider}</span> / <span className="font-medium text-zinc-700">{embedVersion}</span></p>
          {error && <p className="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>}
        </div>

        <div className="rounded-3xl border border-black/10 bg-gradient-to-br from-teal-50 to-amber-50 p-6">
          <h2 className="text-lg font-semibold">How to use effectively</h2>
          <ol className="mt-3 space-y-2 text-sm text-zinc-700">
            <li>1. Create corpus by theme (single research area).</li>
            <li>2. Upload PDFs and run ingest until papers are processed.</li>
            <li>3. Ask specific questions, then verify citations.</li>
            <li>4. Move to Survey Builder for topic reports.</li>
          </ol>
        </div>
      </section>

      <section className="mt-6 rounded-3xl border border-black/10 bg-white/85 p-6">
        <h2 className="text-xl font-semibold">Answer</h2>
        <div className="mt-3 overflow-hidden rounded-2xl border border-black/10 bg-white px-4 py-3">
          {answer ? (
            <ReactMarkdown
              components={{
                h2: ({ children }) => <h3 className="mt-3 text-base font-semibold text-zinc-900 first:mt-0">{children}</h3>,
                ul: ({ children }) => <ul className="mt-2 list-disc space-y-1 pl-5 text-zinc-800">{children}</ul>,
                li: ({ children }) => <li className="leading-7 break-words">{children}</li>,
                p: ({ children }) => <p className="mt-2 leading-7 text-zinc-800 break-words">{children}</p>,
                a: ({ href, children }) => (
                  <a className="font-medium text-teal-800 underline underline-offset-2" href={href}>
                    {children}
                  </a>
                ),
              }}
            >
              {answer}
            </ReactMarkdown>
          ) : (
            <p className="text-zinc-600">No answer yet.</p>
          )}
        </div>
        {citations.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2">
            {citations.map((c) => (
              <a
                key={`chip-${c.ref_id}`}
                href={`#${c.ref_id}`}
                className="rounded-full border border-black/15 bg-white px-3 py-1 text-xs font-medium text-zinc-700 hover:bg-black/5"
              >
                [{c.ref_id}] {c.title}
              </a>
            ))}
          </div>
        )}
      </section>

      <section className="mt-6 rounded-3xl border border-black/10 bg-white/85 p-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Citations</h2>
          <span className="rounded-full border border-black/15 bg-white px-3 py-1 text-xs text-zinc-600">{citations.length} retrieved</span>
        </div>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          {citations.length === 0 && <p className="text-sm text-zinc-500">No citations yet.</p>}
          {citations.map((c) => (
            <article id={c.ref_id} className="overflow-hidden rounded-2xl border border-black/10 bg-white p-4 shadow-sm" key={`${c.ref_id}-${c.chunk_id}`}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-semibold break-words">[{c.ref_id}] {c.title}</div>
                  {c.paper_url && (
                    <a
                      className="mt-1 inline-block text-xs text-teal-700 underline underline-offset-2"
                      href={`${c.paper_url}`}
                      target="_blank"
                      rel="noreferrer"
                    >
                      Open PDF
                    </a>
                  )}
                </div>
                <span className="shrink-0 rounded-full bg-black/5 px-2 py-0.5 text-[11px]">{c.chunk_id.slice(0, 10)}...</span>
              </div>
              <p className="mt-3 break-words text-sm leading-6 text-zinc-800">{c.summary || c.snippet}</p>
              <details className="mt-2">
                <summary className="cursor-pointer text-xs text-zinc-500 hover:text-zinc-700">Evidence excerpt</summary>
                <p className="mt-2 max-h-32 overflow-y-auto break-words pr-1 text-xs leading-6 text-zinc-600">{c.snippet}</p>
              </details>
              <div className="mt-3 text-xs text-zinc-500">Similarity score: {c.score.toFixed(3)}</div>
            </article>
          ))}
        </div>
      </section>
    </main>
  );
}

function linkifyCitationRefs(answer: string): string {
  if (!answer) return answer;
  return answer
    .replace(/\[(C\d+)\]/g, "[$1](#$1)")
    .replace(/\[([^\]:\]]+):[a-f0-9]{12,}\]/gi, "[source]")
    .trim();
}
