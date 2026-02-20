"use client";

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import { api } from "../../../../lib/api";

export default function SurveyPage({ params }: { params: { corpusId: string } }) {
  const [topicsRaw, setTopicsRaw] = useState("transformers, retrieval augmented generation");
  const [questionsRaw, setQuestionsRaw] = useState("");
  const [runId, setRunId] = useState("");
  const [progress, setProgress] = useState<Record<string, string>>({});
  const [report, setReport] = useState("");

  const start = async () => {
    const topics = topicsRaw.split(",").map((x) => x.trim()).filter(Boolean);
    const questions = questionsRaw.split("\n").map((x) => x.trim()).filter(Boolean);
    const res = await api.createSurvey({ corpus_id: params.corpusId, topics, questions });
    setRunId(res.survey_run_id);
    setReport("");
  };

  useEffect(() => {
    if (!runId) return;
    const t = setInterval(async () => {
      const p = await api.surveyProgress(runId);
      setProgress(p.topic_status ?? {});
      const r = await api.surveyReport(runId);
      if (r.report_markdown) setReport(r.report_markdown);
    }, 2500);
    return () => clearInterval(t);
  }, [runId]);

  return (
    <main className="mx-auto max-w-6xl p-8">
      <h1 className="text-4xl font-semibold">Survey Builder</h1>
      <section className="mt-6 grid gap-6 md:grid-cols-2">
        <div className="rounded-3xl border border-black/10 bg-white/80 p-6">
          <label className="text-sm font-medium">Topics (comma-separated)</label>
          <input className="mt-2 w-full rounded-xl border border-black/20 px-3 py-2" value={topicsRaw} onChange={(e) => setTopicsRaw(e.target.value)} />
          <label className="mt-4 block text-sm font-medium">Questions (optional, newline-separated)</label>
          <textarea className="mt-2 h-24 w-full rounded-xl border border-black/20 p-3" value={questionsRaw} onChange={(e) => setQuestionsRaw(e.target.value)} />
          <button className="mt-4 rounded-xl bg-teal px-5 py-2 text-white" onClick={start}>Start Survey</button>
          <p className="mt-3 text-xs text-zinc-600">run: {runId || "-"}</p>
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
          <h2 className="text-xl font-semibold">Report</h2>
          <article className="prose mt-3 max-w-none prose-headings:font-semibold">
            <ReactMarkdown>{report || "Survey report will appear here."}</ReactMarkdown>
          </article>
        </div>
      </section>
    </main>
  );
}
