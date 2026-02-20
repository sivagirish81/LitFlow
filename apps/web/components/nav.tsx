import Link from "next/link";

export function TopNav() {
  return (
    <header className="sticky top-0 z-20 border-b border-black/10 bg-gradient-to-r from-white/90 via-amber-50/70 to-teal-50/70 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3">
        <div className="flex items-center gap-3">
          <span className="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-ink text-sm font-semibold text-white">LF</span>
          <Link className="text-xl font-semibold tracking-tight" href="/">LitFlow</Link>
        </div>
        <div className="flex items-center gap-2">
          <Link className="rounded-full px-4 py-2 text-sm text-zinc-700 hover:bg-black/5" href="/">Overview</Link>
          <Link className="rounded-full border border-black/20 bg-white px-4 py-2 text-sm font-medium" href="/corpora">Corpora</Link>
        </div>
      </div>
    </header>
  );
}
