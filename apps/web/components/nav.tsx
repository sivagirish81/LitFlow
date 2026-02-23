"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export function TopNav() {
  const pathname = usePathname();
  const m = pathname.match(/^\/corpora\/([^/]+)(?:\/(search|survey|graph))?/);
  const corpusId = m?.[1] ?? "";
  const current = (m?.[2] as "search" | "survey" | "graph" | undefined) ?? "";
  const onCorpora = pathname === "/corpora";

  return (
    <header className="sticky top-0 z-20 border-b border-black/10 bg-gradient-to-r from-white/90 via-amber-50/70 to-teal-50/70 backdrop-blur">
      <div className="mx-auto max-w-6xl px-4 py-3">
        <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-ink text-sm font-semibold text-white">LF</span>
          <Link className="text-xl font-semibold tracking-tight" href="/">LitFlow</Link>
        </div>
        <div className="flex items-center gap-2">
          <Link className="rounded-full px-4 py-2 text-sm text-zinc-700 hover:bg-black/5" href="/">Overview</Link>
          <Link className={`rounded-full border px-4 py-2 text-sm font-medium transition ${onCorpora ? "border-ink bg-ink text-white shadow-sm" : "border-black/20 bg-white text-zinc-700 hover:bg-black/5"}`} href="/corpora">Corpora</Link>
        </div>
        </div>
        {corpusId && (
          <nav className="mt-3 flex flex-wrap gap-2 rounded-2xl border border-black/10 bg-white/80 p-2">
            <Link
              href={`/corpora/${corpusId}/search`}
              className={`rounded-xl border px-4 py-2 text-sm font-medium transition ${current === "search" ? "border-ink bg-ink text-white shadow-sm" : "border-black/15 bg-white text-zinc-700 hover:bg-black/5"}`}
            >
              Semantic Search
            </Link>
            <Link
              href={`/corpora/${corpusId}/survey`}
              className={`rounded-xl border px-4 py-2 text-sm font-medium transition ${current === "survey" ? "border-ink bg-ink text-white shadow-sm" : "border-black/15 bg-white text-zinc-700 hover:bg-black/5"}`}
            >
              Survey Builder
            </Link>
            <Link
              href={`/corpora/${corpusId}/graph`}
              className={`rounded-xl border px-4 py-2 text-sm font-medium transition ${current === "graph" ? "border-ink bg-ink text-white shadow-sm" : "border-black/15 bg-white text-zinc-700 hover:bg-black/5"}`}
            >
              Knowledge Graph
            </Link>
          </nav>
        )}
      </div>
    </header>
  );
}
