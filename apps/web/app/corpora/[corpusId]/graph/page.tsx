"use client";

import dynamic from "next/dynamic";
import { useEffect, useMemo, useState } from "react";
import { api } from "../../../../lib/api";

const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false });

type Node = { id: string; node_type: string; label: string; year?: number };
type Link = { source: string; target: string; weight: number; edge_type: string };

export default function GraphPage({ params }: { params: { corpusId: string } }) {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [showPaper, setShowPaper] = useState(true);
  const [showTopic, setShowTopic] = useState(true);
  const [minYear, setMinYear] = useState(0);

  useEffect(() => {
    void (async () => {
      const res = await api.graph(params.corpusId);
      setNodes(res.nodes.map((n) => ({ id: n.node_id, node_type: n.node_type, label: n.label })));
      setLinks(res.edges.map((e) => ({ source: e.source_node_id, target: e.target_node_id, weight: e.weight, edge_type: e.edge_type })));
    })();
  }, [params.corpusId]);

  const filtered = useMemo(() => {
    const allowed = new Set(
      nodes
        .filter((n) => (n.node_type === "paper" ? showPaper : n.node_type === "topic" ? showTopic : true))
        .filter((n) => (n.node_type !== "paper" || !n.year || n.year >= minYear))
        .map((n) => n.id)
    );
    return {
      nodes: nodes.filter((n) => allowed.has(n.id)),
      links: links.filter((l) => allowed.has(String(l.source)) && allowed.has(String(l.target)))
    };
  }, [nodes, links, showPaper, showTopic, minYear]);

  return (
    <main className="mx-auto max-w-6xl p-8">
      <h1 className="text-4xl font-semibold">Knowledge Graph</h1>
      <div className="mt-4 flex gap-2 text-sm">
        <button className={`rounded-full border px-3 py-1 ${showPaper ? "bg-ink text-white" : ""}`} onClick={() => setShowPaper((v) => !v)}>Paper</button>
        <button className={`rounded-full border px-3 py-1 ${showTopic ? "bg-ink text-white" : ""}`} onClick={() => setShowTopic((v) => !v)}>Topic</button>
        <input className="rounded-full border px-3 py-1" type="number" placeholder="Min year" value={minYear || ""} onChange={(e) => setMinYear(Number(e.target.value) || 0)} />
      </div>
      <section className="mt-4 h-[620px] overflow-hidden rounded-3xl border border-black/10 bg-white/80">
        <ForceGraph2D
          graphData={filtered}
          nodeCanvasObject={(node: object, ctx, globalScale) => {
            const n = node as Node;
            const color = n.node_type === "paper" ? "#2d6a6a" : "#b88746";
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
          linkDirectionalParticles={1}
          linkWidth={(l: object) => Math.max(0.5, ((l as Link).weight ?? 0.2) * 2)}
          cooldownTicks={120}
        />
      </section>
    </main>
  );
}
