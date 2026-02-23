"use client";

import dynamic from "next/dynamic";
import { useEffect, useMemo, useState } from "react";
import { api } from "../../../../lib/api";

const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false });

type Tab = "overview" | "lineage" | "performance" | "datasets" | "trends" | "graph";
type Node = { id: string; node_type: string; label: string; year?: number };
type Link = { source: string; target: string; weight: number; edge_type: string };

export default function GraphPage({ params }: { params: { corpusId: string } }) {
  const [tab, setTab] = useState<Tab>("overview");
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");

  const [promptVersion, setPromptVersion] = useState("v1");
  const [modelVersion, setModelVersion] = useState("kg-llm-v1");
  const [maxConcurrent, setMaxConcurrent] = useState(4);
  const [paperId, setPaperId] = useState("");
  const [papers, setPapers] = useState<Array<{ paper_id: string; title?: string; filename: string }>>([]);
  const [wf, setWf] = useState<{ workflow_id: string; run_id: string; status: string } | null>(null);

  const [overview, setOverview] = useState<Awaited<ReturnType<typeof api.kgIntelOverview>> | null>(null);
  const [lineageMethod, setLineageMethod] = useState("transformer");
  const [lineage, setLineage] = useState<Awaited<ReturnType<typeof api.kgIntelLineage>> | null>(null);
  const [performance, setPerformance] = useState<Awaited<ReturnType<typeof api.kgIntelPerformance>> | null>(null);
  const [datasets, setDatasets] = useState<Awaited<ReturnType<typeof api.kgIntelDatasets>> | null>(null);
  const [trends, setTrends] = useState<Awaited<ReturnType<typeof api.kgIntelTrends>> | null>(null);

  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [showPaper, setShowPaper] = useState(true);
  const [showTopic, setShowTopic] = useState(true);
  const [showMethod, setShowMethod] = useState(true);
  const [showDataset, setShowDataset] = useState(true);

  const loadGraph = async () => {
    const res = await api.graph(params.corpusId);
    setNodes(res.nodes.map((n) => ({ id: n.node_id, node_type: n.node_type, label: n.label })));
    setLinks(res.edges.map((e) => ({ source: e.source_node_id, target: e.target_node_id, weight: e.weight, edge_type: e.edge_type })));
  };

  const loadIntel = async () => {
    const [o, p, d, t] = await Promise.all([
      api.kgIntelOverview(params.corpusId),
      api.kgIntelPerformance(params.corpusId, 20),
      api.kgIntelDatasets(params.corpusId, 10),
      api.kgIntelTrends(params.corpusId, 10)
    ]);
    setOverview(o);
    setPerformance(p);
    setDatasets(d);
    setTrends(t);
  };

  useEffect(() => {
    const run = async () => {
      try {
        const papersRes = await api.getPapers(params.corpusId);
        setPapers(papersRes.papers.map((p) => ({ paper_id: p.paper_id, title: p.title, filename: p.filename })));
        await Promise.all([loadGraph(), loadIntel()]);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load KG dashboard.");
      }
    };
    void run();
  }, [params.corpusId]);

  useEffect(() => {
    if (!wf?.workflow_id) return;
    if (["completed", "failed", "terminated", "canceled", "timed_out"].includes(wf.status)) return;
    const t = setInterval(async () => {
      try {
        const s = await api.workflowStatus(wf.workflow_id, wf.run_id);
        setWf((prev) => (prev ? { ...prev, status: s.status } : prev));
        if (s.status === "completed") {
          await Promise.all([loadGraph(), loadIntel()]);
        }
      } catch {
        // ignore transient status errors
      }
    }, 2500);
    return () => clearInterval(t);
  }, [wf?.workflow_id, wf?.run_id, wf?.status]);

  const startKGBackfill = async () => {
    setError("");
    setStatus("Starting KG backfill...");
    try {
      const run = await api.kgBackfill({
        corpus_id: params.corpusId,
        prompt_version: promptVersion,
        model_version: modelVersion,
        max_concurrent: maxConcurrent
      });
      setWf({ workflow_id: run.workflow_id, run_id: run.run_id, status: "running" });
      setStatus(`KG backfill started: ${run.workflow_id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start KG backfill.");
      setStatus("");
    }
  };

  const extractOnePaper = async () => {
    setError("");
    if (!paperId.trim()) return setError("Enter paper_id to extract.");
    try {
      const run = await api.kgExtractPaper(paperId.trim(), {
        corpus_id: params.corpusId,
        prompt_version: promptVersion,
        model_version: modelVersion
      });
      setWf({ workflow_id: run.workflow_id, run_id: run.run_id, status: "running" });
      setStatus(`Paper extraction started: ${run.workflow_id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start paper extraction.");
    }
  };

  const loadLineage = async () => {
    setError("");
    try {
      const out = await api.kgIntelLineage(params.corpusId, lineageMethod, 5);
      setLineage(out);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load lineage.");
    }
  };

  const filteredGraph = useMemo(() => {
    const allowed = new Set(
      nodes
        .filter((n) => (n.node_type !== "paper" || showPaper) && (n.node_type !== "topic" || showTopic) && (n.node_type !== "method" || showMethod) && (n.node_type !== "dataset" || showDataset))
        .map((n) => n.id)
    );
    return {
      nodes: nodes.filter((n) => allowed.has(n.id)),
      links: links.filter((l) => allowed.has(String(l.source)) && allowed.has(String(l.target)))
    };
  }, [nodes, links, showPaper, showTopic, showMethod, showDataset]);

  return (
    <main className="mx-auto max-w-7xl p-8">
      <h1 className="text-4xl font-semibold">Research Intelligence Engine</h1>
      <p className="mt-2 text-sm text-zinc-600">Decision-ready insights from your research graph, with full graph access when needed.</p>

      <section className="mt-6 rounded-3xl border border-black/10 bg-white/85 p-4">
        <h2 className="text-lg font-semibold">KG Extraction Workflows</h2>
        <div className="mt-3 grid gap-2 md:grid-cols-3">
          <label className="space-y-1">
            <span className="block text-xs font-semibold uppercase tracking-wide text-zinc-600">Prompt Version</span>
            <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={promptVersion} onChange={(e) => setPromptVersion(e.target.value)} placeholder="v1" />
          </label>
          <label className="space-y-1">
            <span className="block text-xs font-semibold uppercase tracking-wide text-zinc-600">Model Version Tag</span>
            <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={modelVersion} onChange={(e) => setModelVersion(e.target.value)} placeholder="kg-llm-v1" />
          </label>
          <label className="space-y-1">
            <span className="block text-xs font-semibold uppercase tracking-wide text-zinc-600">Max Concurrent Papers</span>
            <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" type="number" value={maxConcurrent} onChange={(e) => setMaxConcurrent(Number(e.target.value) || 4)} placeholder="4" />
          </label>
        </div>
        <p className="mt-2 text-xs text-zinc-600">
          `v1` = extraction prompt version, `kg-llm-v1` = model/profile label recorded with triples, `4` = papers processed in parallel in KG backfill.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <button className="rounded-xl bg-teal px-4 py-2 text-sm text-white" onClick={startKGBackfill}>Start KG Backfill</button>
          <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={() => void Promise.all([loadGraph(), loadIntel()])}>Refresh Insights</button>
        </div>
        <div className="mt-3 flex gap-2">
          <select className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={paperId} onChange={(e) => setPaperId(e.target.value)}>
            <option value="">Select a paper for one-off extraction</option>
            {papers.map((p) => (
              <option key={p.paper_id} value={p.paper_id}>
                {(p.title && p.title.trim()) ? p.title : p.filename}
              </option>
            ))}
          </select>
          <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={extractOnePaper}>Extract Paper</button>
        </div>
        {wf && <p className="mt-2 text-xs text-zinc-600">Workflow: {wf.workflow_id} ({wf.status})</p>}
      </section>

      <div className="mt-6 flex flex-wrap gap-2">
        {(["overview", "lineage", "performance", "datasets", "trends", "graph"] as Tab[]).map((t) => (
          <button key={t} className={`rounded-full border px-4 py-2 text-sm ${tab === t ? "bg-ink text-white" : "bg-white"}`} onClick={() => setTab(t)}>
            {t === "overview" ? "Overview" : t === "lineage" ? "Lineage Explorer" : t === "performance" ? "Performance Matrix" : t === "datasets" ? "Dataset Dominance" : t === "trends" ? "Trend Timeline" : "Full Graph"}
          </button>
        ))}
      </div>

      {status && <p className="mt-3 text-sm text-emerald-700">{status}</p>}
      {error && <p className="mt-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>}

      {tab === "overview" && (
        <section className="mt-4 grid gap-4 md:grid-cols-3">
          <Card title="Top Method Families">
            {overview?.top_method_families?.map((m) => <Row key={m.node_id} l={m.label} r={`links ${m.linked_methods}`} />) ?? <Empty />}
          </Card>
          <Card title="Top Outperformers">
            {overview?.top_outperformers?.map((m) => <Row key={m.node_id} l={m.label} r={`${m.score.toFixed(0)} wins`} />) ?? <Empty />}
          </Card>
          <Card title="Top Datasets">
            {overview?.top_datasets?.map((d) => <Row key={d.node_id} l={d.label} r={`${d.usage_count} uses`} />) ?? <Empty />}
          </Card>
        </section>
      )}

      {tab === "lineage" && (
        <section className="mt-4 grid gap-4 md:grid-cols-2">
          <Card title="Lineage Controls">
            <div className="flex gap-2">
              <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={lineageMethod} onChange={(e) => setLineageMethod(e.target.value)} placeholder="Method name (e.g., transformer)" />
              <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={loadLineage}>Load</button>
            </div>
            <p className="mt-2 text-xs text-zinc-600">
              Enter a method family name (examples: `transformer`, `bert`, `qphh`, `invar`). Use names from Overview {">"} Top Method Families for best results.
            </p>
            <div className="mt-3 text-sm text-zinc-700">Root: {lineage?.root_method?.label ?? "Not loaded"}</div>
          </Card>
          <Card title="Timeline">
            {lineage?.timeline?.length ? lineage.timeline.map((t, i) => <Row key={`${t.year}-${i}`} l={`${t.year}`} r={`${t.count} papers`} />) : <Empty />}
          </Card>
          <Card title="Descendant Tree">
            {lineage?.tree_edges?.length ? lineage.tree_edges.map((e, i) => <Row key={`${e.source_id}-${e.target_id}-${i}`} l={`${e.source_name}`} r={`${e.edge_type} -> ${e.target_name}`} />) : <Empty />}
          </Card>
          <Card title="Associated Datasets + Top Descendants">
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-500">Datasets</div>
            {lineage?.datasets?.length ? lineage.datasets.map((d, i) => <Row key={`${d.node_id}-${i}`} l={d.label} r={`${d.usage_count}`} />) : <Empty />}
            <div className="mt-3 text-xs font-semibold uppercase tracking-wide text-zinc-500">Top Descendants</div>
            {lineage?.top_descendants?.length ? lineage.top_descendants.map((m, i) => <Row key={`${m.node_id}-${i}`} l={m.label} r={`${m.score.toFixed(1)}`} />) : <Empty />}
          </Card>
        </section>
      )}

      {tab === "performance" && (
        <section className="mt-4 rounded-3xl border border-black/10 bg-white/85 p-4">
          <h3 className="text-lg font-semibold">Competitive Performance Matrix</h3>
          <div className="mt-3 overflow-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b text-left text-zinc-500">
                  <th className="px-2 py-2">Method</th><th className="px-2 py-2">Wins</th><th className="px-2 py-2">Losses</th><th className="px-2 py-2">Win Rate</th><th className="px-2 py-2">Datasets</th><th className="px-2 py-2">Metrics</th><th className="px-2 py-2">Dominance</th>
                </tr>
              </thead>
              <tbody>
                {performance?.rows?.map((r, i) => (
                  <tr key={`${r.method}-${i}`} className="border-b">
                    <td className="px-2 py-2 font-medium">{r.method}</td>
                    <td className="px-2 py-2">{r.wins}</td>
                    <td className="px-2 py-2">{r.losses}</td>
                    <td className="px-2 py-2">{(r.win_rate * 100).toFixed(0)}%</td>
                    <td className="px-2 py-2">{r.dataset_coverage}</td>
                    <td className="px-2 py-2">{r.metric_coverage}</td>
                    <td className="px-2 py-2">{r.dominance_score.toFixed(2)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {tab === "datasets" && (
        <section className="mt-4 grid gap-4 md:grid-cols-2">
          <Card title="Most Used Datasets">
            {datasets?.datasets?.length ? datasets.datasets.map((d, i) => <Row key={`${d.node_id}-${i}`} l={d.label} r={`${d.usage_count} uses`} />) : <Empty />}
          </Card>
          <Card title="Dataset-Method Distribution">
            {datasets?.method_distribution?.length ? datasets.method_distribution.slice(0, 20).map((d, i) => <Row key={`${d.dataset}-${d.method}-${i}`} l={`${d.dataset} -> ${d.method}`} r={`${(d.share * 100).toFixed(0)}%`} />) : <Empty />}
          </Card>
          <Card title="Usage By Year">
            {datasets?.usage_by_year?.length ? datasets.usage_by_year.slice(-30).map((y, i) => <Row key={`${y.dataset}-${y.year}-${i}`} l={`${y.year} ${y.dataset}`} r={`${y.count}`} />) : <Empty />}
          </Card>
        </section>
      )}

      {tab === "trends" && (
        <section className="mt-4 grid gap-4 md:grid-cols-2">
          <Card title="Research Trend Timeline">
            {trends?.family_series?.length ? trends.family_series.map((s, i) => <Row key={`${s.family}-${s.year}-${i}`} l={`${s.year} ${s.family}`} r={`${s.count} proposes`} />) : <Empty />}
          </Card>
          <Card title="Emerging Methods">
            {trends?.emerging_methods?.length ? trends.emerging_methods.map((m, i) => <Row key={`${m.node_id}-${i}`} l={m.label} r={`score ${m.score.toFixed(2)}`} />) : <Empty />}
          </Card>
        </section>
      )}

      {tab === "graph" && (
        <section className="mt-4">
          <div className="mb-2 flex gap-2 text-sm">
            <button className={`rounded-full border px-3 py-1 ${showPaper ? "bg-ink text-white" : ""}`} onClick={() => setShowPaper((v) => !v)}>Paper</button>
            <button className={`rounded-full border px-3 py-1 ${showTopic ? "bg-ink text-white" : ""}`} onClick={() => setShowTopic((v) => !v)}>Topic</button>
            <button className={`rounded-full border px-3 py-1 ${showMethod ? "bg-ink text-white" : ""}`} onClick={() => setShowMethod((v) => !v)}>Method</button>
            <button className={`rounded-full border px-3 py-1 ${showDataset ? "bg-ink text-white" : ""}`} onClick={() => setShowDataset((v) => !v)}>Dataset</button>
          </div>
          <div className="h-[680px] overflow-hidden rounded-3xl border border-black/10 bg-white/80">
            <ForceGraph2D
              graphData={filteredGraph}
              nodeCanvasObject={(node: object, ctx, globalScale) => {
                const n = node as Node;
                const color = n.node_type === "paper" ? "#2d6a6a" : n.node_type === "topic" ? "#b88746" : n.node_type === "method" ? "#7c3aed" : n.node_type === "dataset" ? "#c2410c" : "#334155";
                ctx.beginPath();
                ctx.fillStyle = color;
                ctx.arc((node as { x?: number }).x ?? 0, (node as { y?: number }).y ?? 0, 4, 0, 2 * Math.PI, false);
                ctx.fill();
                ctx.font = `${10 / globalScale}px IBM Plex Sans`;
                ctx.fillStyle = "#111318";
                ctx.fillText(n.label, ((node as { x?: number }).x ?? 0) + 6, ((node as { y?: number }).y ?? 0) + 3);
              }}
              linkDirectionalArrowLength={5}
              linkDirectionalArrowRelPos={1}
              linkWidth={(l: object) => Math.max(1, ((l as Link).weight ?? 0.1) * 2)}
            />
          </div>
        </section>
      )}
    </main>
  );
}

function Card({ title, children }: { title: string; children: any }) {
  return (
    <div className="rounded-3xl border border-black/10 bg-white/85 p-4">
      <h3 className="text-lg font-semibold">{title}</h3>
      <div className="mt-3 space-y-2 text-sm">{children}</div>
    </div>
  );
}

function Row({ l, r }: { l: string; r: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-black/10 bg-zinc-50 px-3 py-2">
      <div className="min-w-0 truncate">{l}</div>
      <div className="shrink-0 text-zinc-500">{r}</div>
    </div>
  );
}

function Empty() {
  return <div className="rounded-lg border border-dashed border-black/20 px-3 py-3 text-sm text-zinc-500">No data available yet.</div>;
}
