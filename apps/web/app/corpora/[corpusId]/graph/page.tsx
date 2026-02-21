"use client";

import dynamic from "next/dynamic";
import { useEffect, useMemo, useState } from "react";
import { api } from "../../../../lib/api";

const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false });

type Node = { id: string; node_type: string; label: string; year?: number };
type Link = { source: string; target: string; weight: number; edge_type: string };
type LineageEdge = { source_id: string; source_name: string; target_id: string; target_name: string; edge_type: string; depth: number };

export default function GraphPage({ params }: { params: { corpusId: string } }) {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [showPaper, setShowPaper] = useState(true);
  const [showTopic, setShowTopic] = useState(true);
  const [showMethod, setShowMethod] = useState(true);
  const [showDataset, setShowDataset] = useState(true);
  const [minYear, setMinYear] = useState(0);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");

  const [promptVersion, setPromptVersion] = useState("v1");
  const [modelVersion, setModelVersion] = useState("kg-llm-v1");
  const [maxConcurrent, setMaxConcurrent] = useState(4);
  const [paperId, setPaperId] = useState("");
  const [methodName, setMethodName] = useState("");
  const [lineageEdges, setLineageEdges] = useState<LineageEdge[]>([]);
  const [cypher, setCypher] = useState("MATCH (n)-[r]->(m) RETURN n,r,m LIMIT 25");
  const [queryOut, setQueryOut] = useState("");
  const [wf, setWf] = useState<{ workflow_id: string; run_id: string; status: string } | null>(null);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const res = await api.graph(params.corpusId);
        setNodes(res.nodes.map((n) => ({ id: n.node_id, node_type: n.node_type, label: n.label })));
        setLinks(res.edges.map((e) => ({ source: e.source_node_id, target: e.target_node_id, weight: e.weight, edge_type: e.edge_type })));
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load graph.");
      } finally {
        setLoading(false);
      }
    };
    void load();
  }, [params.corpusId]);

  useEffect(() => {
    if (!wf?.workflow_id) return;
    const done = ["completed", "failed", "terminated", "canceled", "timed_out"].includes(wf.status);
    if (done) return;
    const t = setInterval(async () => {
      try {
        const s = await api.workflowStatus(wf.workflow_id, wf.run_id);
        setWf((prev) => (prev ? { ...prev, status: s.status } : prev));
        if (s.status === "completed") {
          await refreshGraph();
        }
      } catch {
        // ignore transient status errors
      }
    }, 2500);
    return () => clearInterval(t);
  }, [wf?.workflow_id, wf?.run_id, wf?.status]);

  const filtered = useMemo(() => {
    const allowed = new Set(
      nodes
        .filter((n) => {
          if (n.node_type === "paper") return showPaper;
          if (n.node_type === "topic") return showTopic;
          if (n.node_type === "method") return showMethod;
          if (n.node_type === "dataset") return showDataset;
          return true;
        })
        .filter((n) => (n.node_type !== "paper" || !n.year || n.year >= minYear))
        .map((n) => n.id)
    );
    return {
      nodes: nodes.filter((n) => allowed.has(n.id)),
      links: links.filter((l) => allowed.has(String(l.source)) && allowed.has(String(l.target)))
    };
  }, [nodes, links, showPaper, showTopic, showMethod, showDataset, minYear]);

  const selected = useMemo(() => nodes.find((n) => n.id === selectedNodeId) ?? null, [nodes, selectedNodeId]);
  const selectedEdges = useMemo(() => {
    if (!selectedNodeId) return [] as Link[];
    return links.filter((l) => String(l.source) === selectedNodeId || String(l.target) === selectedNodeId);
  }, [links, selectedNodeId]);

  const refreshGraph = async () => {
    const res = await api.graph(params.corpusId);
    setNodes(res.nodes.map((n) => ({ id: n.node_id, node_type: n.node_type, label: n.label })));
    setLinks(res.edges.map((e) => ({ source: e.source_node_id, target: e.target_node_id, weight: e.weight, edge_type: e.edge_type })));
  };

  const startKGBackfill = async () => {
    setError("");
    setStatus("Starting KG backfill...");
    try {
      const run = await api.kgBackfill({ corpus_id: params.corpusId, prompt_version: promptVersion, model_version: modelVersion, max_concurrent: maxConcurrent });
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
    setStatus("Starting paper extraction...");
    try {
      const run = await api.kgExtractPaper(paperId.trim(), { corpus_id: params.corpusId, prompt_version: promptVersion, model_version: modelVersion });
      setWf({ workflow_id: run.workflow_id, run_id: run.run_id, status: "running" });
      setStatus(`Paper extraction started: ${run.workflow_id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start paper extraction.");
      setStatus("");
    }
  };

  const runLineage = async () => {
    setError("");
    if (!methodName.trim()) return setError("Enter method name.");
    try {
      const res = await api.kgLineage(params.corpusId, methodName.trim());
      setLineageEdges(res.edges ?? []);
      setStatus(`Lineage loaded for "${methodName.trim()}".`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Lineage query failed.");
      setStatus("");
    }
  };

  const runCypher = async () => {
    setError("");
    if (!cypher.trim()) return setError("Enter Cypher query.");
    try {
      const out = await api.kgQuery({ corpus_id: params.corpusId, cypher });
      setQueryOut(JSON.stringify(out, null, 2));
      setStatus("Cypher query executed.");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Cypher query failed.");
      setStatus("");
    }
  };

  return (
    <main className="mx-auto max-w-6xl p-8">
      <h1 className="text-4xl font-semibold">Knowledge Graph Intelligence</h1>
      <p className="mt-2 text-sm text-zinc-600">Run KG extraction workflows, query lineage, execute graph queries, and inspect the graph interactively.</p>

      <section className="mt-6 grid gap-4 rounded-3xl border border-black/10 bg-white/85 p-4 md:grid-cols-2">
        <div className="space-y-3 rounded-2xl border border-black/10 bg-white p-4">
          <h2 className="text-lg font-semibold">KG Extraction Workflows</h2>
          <div className="grid gap-2 md:grid-cols-3">
            <label className="space-y-1">
              <span className="block text-xs font-medium uppercase tracking-wide text-zinc-600">Prompt Version</span>
              <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={promptVersion} onChange={(e) => setPromptVersion(e.target.value)} placeholder="v1" />
            </label>
            <label className="space-y-1">
              <span className="block text-xs font-medium uppercase tracking-wide text-zinc-600">Model Version Tag</span>
              <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={modelVersion} onChange={(e) => setModelVersion(e.target.value)} placeholder="kg-llm-v1" />
            </label>
            <label className="space-y-1">
              <span className="block text-xs font-medium uppercase tracking-wide text-zinc-600">Max Concurrent Papers</span>
              <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" type="number" value={maxConcurrent} onChange={(e) => setMaxConcurrent(Number(e.target.value) || 4)} placeholder="4" />
            </label>
          </div>
          <p className="text-xs text-zinc-600">
            <span className="font-medium">Prompt Version:</span> tracks extraction prompt changes (for reproducible re-runs).{" "}
            <span className="font-medium">Model Version Tag:</span> your label for which LLM/profile produced triples.{" "}
            <span className="font-medium">Max Concurrent Papers:</span> how many papers KG backfill processes in parallel.
          </p>
          <div className="flex flex-wrap gap-2">
            <button className="rounded-xl bg-teal px-4 py-2 text-sm text-white" onClick={startKGBackfill}>Start KG Backfill</button>
            <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={refreshGraph}>Refresh Graph</button>
          </div>
          <div className="flex gap-2">
            <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={paperId} onChange={(e) => setPaperId(e.target.value)} placeholder="paper_id for one-off extract" />
            <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={extractOnePaper}>Extract Paper</button>
          </div>
          {wf && (
            <div className="rounded-lg border border-black/10 bg-zinc-50 p-3 text-xs text-zinc-700">
              <div><span className="font-semibold">Workflow:</span> {wf.workflow_id}</div>
              <div><span className="font-semibold">Run:</span> {wf.run_id}</div>
              <div><span className="font-semibold">Status:</span> {wf.status}</div>
            </div>
          )}
        </div>

        <div className="space-y-3 rounded-2xl border border-black/10 bg-white p-4">
          <h2 className="text-lg font-semibold">Lineage + Cypher</h2>
          <div className="flex gap-2">
            <input className="w-full rounded-lg border border-black/15 px-3 py-2 text-sm" value={methodName} onChange={(e) => setMethodName(e.target.value)} placeholder='method name (e.g., "transformer")' />
            <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={runLineage}>Get Lineage</button>
          </div>
          <div className="max-h-28 overflow-auto rounded-lg border border-black/10 bg-zinc-50 p-2 text-xs text-zinc-700">
            {lineageEdges.length === 0 ? "No lineage loaded." : lineageEdges.map((e, i) => <div key={`${e.source_id}-${e.target_id}-${i}`}>[{e.depth}] {e.source_name} -{e.edge_type}→ {e.target_name}</div>)}
          </div>
          <textarea className="h-24 w-full rounded-lg border border-black/15 p-2 font-mono text-xs" value={cypher} onChange={(e) => setCypher(e.target.value)} />
          <button className="rounded-xl border border-black/20 px-4 py-2 text-sm" onClick={runCypher}>Run Cypher</button>
          <pre className="max-h-28 overflow-auto rounded-lg border border-black/10 bg-zinc-50 p-2 text-xs text-zinc-700">{queryOut || "Cypher results will appear here."}</pre>
        </div>
      </section>

      {status && <p className="mt-3 text-sm text-emerald-700">{status}</p>}
      {error && <p className="mt-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>}

      <div className="mt-4 flex gap-2 text-sm">
        <button className={`rounded-full border px-3 py-1 ${showPaper ? "bg-ink text-white" : ""}`} onClick={() => setShowPaper((v) => !v)}>Paper</button>
        <button className={`rounded-full border px-3 py-1 ${showTopic ? "bg-ink text-white" : ""}`} onClick={() => setShowTopic((v) => !v)}>Topic</button>
        <button className={`rounded-full border px-3 py-1 ${showMethod ? "bg-ink text-white" : ""}`} onClick={() => setShowMethod((v) => !v)}>Method</button>
        <button className={`rounded-full border px-3 py-1 ${showDataset ? "bg-ink text-white" : ""}`} onClick={() => setShowDataset((v) => !v)}>Dataset</button>
        <input className="rounded-full border px-3 py-1" type="number" placeholder="Min year" value={minYear || ""} onChange={(e) => setMinYear(Number(e.target.value) || 0)} />
      </div>
      <section className="mt-4 h-[620px] overflow-hidden rounded-3xl border border-black/10 bg-white/80">
        {loading && <div className="p-4 text-sm text-zinc-600">Loading graph...</div>}
        {!loading && filtered.links.length === 0 && (
          <div className="p-4 text-sm text-zinc-600">No graph edges yet. Run KG Backfill, then click Refresh Graph.</div>
        )}
        <ForceGraph2D
          graphData={filtered}
          onNodeClick={(node: object) => setSelectedNodeId((node as Node).id)}
          nodeCanvasObject={(node: object, ctx, globalScale) => {
            const n = node as Node;
            const color =
              n.node_type === "paper" ? "#2d6a6a" :
              n.node_type === "topic" ? "#b88746" :
              n.node_type === "method" ? "#7c3aed" :
              n.node_type === "dataset" ? "#c2410c" : "#334155";
            const r = 5;
            ctx.beginPath();
            ctx.fillStyle = color;
            ctx.arc((node as { x?: number }).x ?? 0, (node as { y?: number }).y ?? 0, r, 0, 2 * Math.PI, false);
            ctx.fill();
            const fontSize = 11 / globalScale;
            ctx.font = `${fontSize}px IBM Plex Sans`;
            ctx.fillStyle = "#111318";
            ctx.fillText(n.label, ((node as { x?: number }).x ?? 0) + 8, ((node as { y?: number }).y ?? 0) + 4);
          }}
          linkColor={(l: object) => {
            const t = String((l as Link).edge_type || "").toUpperCase();
            if (t === "EXTENDS") return "#0f766e";
            if (t === "BASED_ON") return "#2563eb";
            if (t === "OUTPERFORMS") return "#b91c1c";
            if (t === "USES_DATASET") return "#7c3aed";
            if (t === "EVALUATED_ON") return "#c2410c";
            return "#64748b";
          }}
          linkDirectionalArrowLength={6}
          linkDirectionalArrowRelPos={1}
          linkDirectionalParticles={1}
          linkWidth={(l: object) => Math.max(1, ((l as Link).weight ?? 0.1) * 3)}
          cooldownTicks={120}
        />
      </section>
      {selected && (
        <section className="mt-4 rounded-2xl border border-black/10 bg-white/80 p-4">
          <h3 className="text-lg font-semibold">Selected Node</h3>
          <p className="text-sm text-zinc-700">{selected.label} <span className="text-zinc-500">({selected.node_type})</span></p>
          <div className="mt-2 max-h-40 overflow-auto rounded-lg border border-black/10 bg-zinc-50 p-2 text-xs text-zinc-700">
            {selectedEdges.length === 0
              ? "No connected edges."
                : selectedEdges.map((e, i) => (
                  <div key={`${String(e.source)}-${String(e.target)}-${i}`}>
                    {String(e.source)} -{e.edge_type}{">"} {String(e.target)}
                  </div>
                ))}
          </div>
        </section>
      )}
    </main>
  );
}
