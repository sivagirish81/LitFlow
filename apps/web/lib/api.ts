const API_BASE = process.env.NEXT_PUBLIC_LITFLOW_API_BASE ?? "http://localhost:8080";

type ApiErrPayload = {
  error?: {
    code?: string;
    message?: string;
  };
};

async function parseApiError(res: Response): Promise<string> {
  try {
    const data = (await res.json()) as ApiErrPayload;
    const code = data?.error?.code ?? `HTTP-${res.status}`;
    const message = data?.error?.message ?? "Request failed.";
    return `${code}: ${message}`;
  } catch {
    return `HTTP-${res.status}: Request failed.`;
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {})
    },
    cache: "no-store"
  });
  if (!res.ok) {
    throw new Error(await parseApiError(res));
  }
  return res.json() as Promise<T>;
}

export const api = {
  getCorpora: () => req<{ corpora: Array<{ corpus_id: string; name: string; created_at: string }> }>("/corpora"),
  createCorpus: (name: string) => req<{ corpus_id: string; name: string }>("/corpora", { method: "POST", body: JSON.stringify({ name }) }),
  uploadPDFs: async (corpusId: string, files: File[]) => {
    const fd = new FormData();
    files.forEach((f) => fd.append("files", f));
    const res = await fetch(`${API_BASE}/corpora/${corpusId}/upload`, { method: "POST", body: fd });
    if (!res.ok) throw new Error(await parseApiError(res));
    return res.json() as Promise<{ uploaded: Array<{ filename: string; paper_id: string }> }>;
  },
  startIngest: (corpusId: string) => req<{ workflow_id: string; run_id: string }>(`/corpora/${corpusId}/ingest`, { method: "POST" }),
  getProgress: (corpusId: string) => req<{ total: number; done: number; failed: number; per_paper_status: Record<string, string> }>(`/corpora/${corpusId}/progress`),
  getPapers: (corpusId: string) => req<{ papers: Array<{ paper_id: string; filename: string; status: string; fail_reason?: string }> }>(`/corpora/${corpusId}/papers`),
  ask: (payload: { corpus_id: string; question: string; top_k?: number }) => req<{ answer: string; citations: Array<{ ref_id: string; paper_id: string; title: string; filename?: string; paper_url?: string; chunk_id: string; snippet: string; summary?: string; score: number }> }>("/ask", { method: "POST", body: JSON.stringify(payload) }),
  createSurvey: (payload: { corpus_id: string; topics: string[]; questions: string[] }) => req<{ survey_run_id: string }>("/survey", { method: "POST", body: JSON.stringify(payload) }),
  surveyProgress: (id: string) => req<{ total_topics: number; done_topics: number; topic_status: Record<string, string> }>(`/survey/${id}/progress`),
  surveyReport: (id: string) => req<{ status: string; report_markdown: string }>(`/survey/${id}/report`),
  graph: (corpusId: string) => req<{ nodes: Array<{ node_id: string; node_type: string; label: string }>; edges: Array<{ source_node_id: string; target_node_id: string; weight: number; edge_type: string }> }>(`/corpora/${corpusId}/graph`)
};
