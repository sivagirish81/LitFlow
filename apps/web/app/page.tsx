import Link from "next/link";

export default function HomePage() {
  return (
    <main className="mx-auto grid min-h-[85vh] max-w-6xl place-items-center p-8">
      <section className="w-full rounded-3xl border border-black/10 bg-white/70 p-10 shadow-xl">
        <p className="text-sm uppercase tracking-[0.24em] text-zinc-500">Temporal-native research stack</p>
        <h1 className="mt-4 text-6xl font-semibold leading-tight">LitFlow</h1>
        <p className="mt-5 max-w-2xl text-lg text-zinc-700">Upload real PDFs, ingest with Temporal workflows, ask citation-backed questions, build surveys, and inspect your topic graph.</p>
        <Link className="mt-8 inline-flex rounded-full bg-ink px-6 py-3 text-white" href="/corpora">Start a Corpus</Link>
      </section>
    </main>
  );
}
