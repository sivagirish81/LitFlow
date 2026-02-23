"use client";

import { useEffect, useState } from "react";
import { api } from "../../../../lib/api";

export default function SurveyPage({ params }: { params: { corpusId: string } }) {
  const [prompt, setPrompt] = useState("");
  const [runId, setRunId] = useState("");
  const [progress, setProgress] = useState<Record<string, string>>({});
  const [status, setStatus] = useState("");
  const [report, setReport] = useState("");
  const [outputFormat, setOutputFormat] = useState("latex");
  const [error, setError] = useState("");

  const start = async () => {
    setError("");
    const cleanPrompt = prompt.trim();
    if (!cleanPrompt) {
      setError("Please enter a topic prompt.");
      return;
    }
    try {
      const res = await api.createSurvey({
        corpus_id: params.corpusId,
        prompt: cleanPrompt,
        output_format: "latex",
        retrieval_top_k: 14
      });
      setRunId(res.survey_run_id);
      setReport("");
      setStatus("running");
      setOutputFormat("latex");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start survey.");
    }
  };

  const downloadCurrentText = () => {
    if (!report.trim()) return;
    const ext = outputFormat === "latex" ? "tex" : "txt";
    const blob = new Blob([report], { type: outputFormat === "latex" ? "text/x-tex;charset=utf-8" : "text/plain;charset=utf-8" });
    const href = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = href;
    a.download = `${runId || "survey-draft"}.${ext}`;
    a.click();
    URL.revokeObjectURL(href);
  };

  useEffect(() => {
    if (!runId) return;
    const t = setInterval(async () => {
      try {
        const p = await api.surveyProgress(runId);
        setProgress(p.topic_status ?? {});
      } catch {
        // Ignore progress polling failures while workflow starts.
      }
      try {
        const r = await api.surveyReport(runId);
        setStatus(r.status ?? "");
        if (r.output_format) setOutputFormat(r.output_format);
        if (r.report_text) setReport(r.report_text);
      } catch {
        // Keep polling on transient errors.
      }
    }, 2500);
    return () => clearInterval(t);
  }, [runId]);

  return (
    <main className="mx-auto max-w-6xl p-8">
      <h1 className="text-4xl font-semibold">Survey Builder</h1>
      <section className="mt-6 grid gap-6 md:grid-cols-2">
        <div className="rounded-3xl border border-black/10 bg-white/80 p-6">
          <label className="text-sm font-medium">Topic Prompt</label>
          <textarea
            className="mt-2 h-40 w-full rounded-xl border border-black/20 p-3"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Example: Transformer-based long-context retrieval techniques for scientific document QA."
          />
          <button className="mt-4 rounded-xl bg-teal px-5 py-2 text-white" onClick={start}>Start Survey</button>
          {error ? <p className="mt-3 text-sm text-rose-700">{error}</p> : null}
          <p className="mt-3 text-xs text-zinc-600">run: {runId || "-"}</p>
          <p className="mt-1 text-xs text-zinc-600">status: {status || "-"}</p>
          <div className="mt-4 space-y-2 text-sm">
            {Object.entries(progress).map(([topic, status]) => (
              <div className="flex items-center justify-between rounded-lg border border-black/10 px-3 py-2" key={topic}>
                <span>{topic}</span>
                <span className="rounded-full border border-black/20 px-2 py-0.5 text-xs">{status}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="rounded-3xl border border-black/10 bg-white/80 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold">LaTeX Draft</h2>
            <button className="rounded-lg border border-black/20 px-3 py-1.5 text-sm hover:bg-black/5 disabled:opacity-50" onClick={downloadCurrentText} disabled={!report.trim()}>
              Download .tex
            </button>
          </div>
          <textarea
            className="mt-3 h-[520px] w-full rounded-xl border border-black/20 p-3 font-mono text-sm"
            value={report}
            onChange={(e) => setReport(e.target.value)}
            placeholder="Generated LaTeX report will appear here."
          />
        </div>
      </section>
    </main>
  );
}
