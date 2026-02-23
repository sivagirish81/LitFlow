package graph

import (
	"fmt"
	"strings"
)

const KGExtractionPromptTemplate = `You are a research knowledge graph extractor.
Extract only explicit relationships from the input text.
Do not infer beyond the text.

Output STRICT JSON with this schema:
{
  "triples": [
    {
      "source_type": "paper|author|method|dataset|task|metric|organization",
      "source_name": "string",
      "relation_type": "CITES|PROPOSES|BASED_ON|EXTENDS|OUTPERFORMS|EVALUATED_ON|AUTHORED_BY|IMPLEMENTS|USES_DATASET",
      "target_type": "paper|author|method|dataset|task|metric|organization",
      "target_name": "string",
      "evidence": "short evidence span from text",
      "confidence": 0.0
    }
  ]
}

Rules:
- Emit at most 12 triples.
- Emit only if the relationship is directly supported by text.
- confidence must be in [0,1].
- Keep evidence short and verbatim-like.
- If no triples, return {"triples":[]}.

Few-shot examples:
Input: "BERT is based on Transformer and evaluated on GLUE."
Output: {"triples":[
{"source_type":"method","source_name":"BERT","relation_type":"BASED_ON","target_type":"method","target_name":"Transformer","evidence":"BERT is based on Transformer","confidence":0.95},
{"source_type":"method","source_name":"BERT","relation_type":"EVALUATED_ON","target_type":"task","target_name":"GLUE","evidence":"evaluated on GLUE","confidence":0.92}
]}

Input: "We compare to GPT-3."
Output: {"triples":[]}
`

func BuildChunkExtractionPrompt(paperTitle, chunkText string) string {
	title := strings.TrimSpace(paperTitle)
	if title == "" {
		title = "Unknown Paper"
	}
	return KGExtractionPromptTemplate + "\n\nPaper: " + title + "\n\nChunk:\n" + chunkText
}

func PromptHash(promptVersion string) string {
	return fmt.Sprintf("kg_prompt_%s", strings.TrimSpace(promptVersion))
}
