import Link from "next/link";

type Props = {
  corpusId: string;
  current: "search" | "survey" | "graph";
};

export function CorpusNav({ corpusId, current }: Props) {
  const tabs = [
    { key: "search", label: "Semantic Search", href: `/corpora/${corpusId}/search` },
    { key: "survey", label: "Survey Builder", href: `/corpora/${corpusId}/survey` },
    { key: "graph", label: "Knowledge Graph", href: `/corpora/${corpusId}/graph` }
  ] as const;

  return (
    <nav className="mt-4 flex flex-wrap gap-2 rounded-2xl border border-black/10 bg-white/70 p-2 shadow-sm">
      {tabs.map((tab) => {
        const active = tab.key === current;
        return (
          <Link
            key={tab.key}
            href={tab.href}
            className={`rounded-xl px-4 py-2 text-sm font-medium transition ${active ? "bg-ink text-white" : "text-zinc-700 hover:bg-black/5"}`}
          >
            {tab.label}
          </Link>
        );
      })}
    </nav>
  );
}
