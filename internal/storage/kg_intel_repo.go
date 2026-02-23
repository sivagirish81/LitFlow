package storage

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

type IntelOverview struct {
	TopMethodFamilies []IntelMethodFamily `json:"top_method_families"`
	TopDatasets       []IntelDatasetStat  `json:"top_datasets"`
	TopOutperformers  []IntelMethodStat   `json:"top_outperformers"`
}

type IntelMethodFamily struct {
	NodeID         string `json:"node_id"`
	Label          string `json:"label"`
	LinkedMethods  int    `json:"linked_methods"`
	SupportCount   int    `json:"support_count"`
	OutperformWins int    `json:"outperform_wins"`
}

type IntelMethodStat struct {
	NodeID string  `json:"node_id"`
	Label  string  `json:"label"`
	Score  float64 `json:"score"`
}

type IntelDatasetStat struct {
	NodeID      string `json:"node_id"`
	Label       string `json:"label"`
	UsageCount  int    `json:"usage_count"`
	MethodCount int    `json:"method_count"`
}

type IntelLineage struct {
	RootMethod     IntelLineageNode   `json:"root_method"`
	TreeNodes      []IntelLineageNode `json:"tree_nodes"`
	TreeEdges      []LineageEdge      `json:"tree_edges"`
	Timeline       []IntelYearCount   `json:"timeline"`
	Datasets       []IntelDatasetStat `json:"datasets"`
	TopDescendants []IntelMethodStat  `json:"top_descendants"`
}

type IntelLineageNode struct {
	NodeID        string `json:"node_id"`
	Label         string `json:"label"`
	Depth         int    `json:"depth"`
	YearFirstSeen int    `json:"year_first_seen"`
}

type IntelYearCount struct {
	Year  int `json:"year"`
	Count int `json:"count"`
}

type IntelPerformanceRow struct {
	Method          string   `json:"method"`
	BeatenMethods   int      `json:"beaten_methods"`
	Wins            int      `json:"wins"`
	Losses          int      `json:"losses"`
	WinRate         float64  `json:"win_rate"`
	DatasetCoverage int      `json:"dataset_coverage"`
	MetricCoverage  int      `json:"metric_coverage"`
	DominanceScore  float64  `json:"dominance_score"`
	TopDatasets     []string `json:"top_datasets"`
	TopMetrics      []string `json:"top_metrics"`
}

type IntelPerformance struct {
	Rows []IntelPerformanceRow `json:"rows"`
}

type IntelDatasetDominance struct {
	Datasets           []IntelDatasetStat  `json:"datasets"`
	UsageByYear        []IntelDatasetYear  `json:"usage_by_year"`
	MethodDistribution []IntelDatasetShare `json:"method_distribution"`
}

type IntelDatasetYear struct {
	Dataset string `json:"dataset"`
	Year    int    `json:"year"`
	Count   int    `json:"count"`
}

type IntelDatasetShare struct {
	Dataset string  `json:"dataset"`
	Method  string  `json:"method"`
	Share   float64 `json:"share"`
}

type IntelTrendTimeline struct {
	FamilySeries    []IntelFamilyYear `json:"family_series"`
	EmergingMethods []IntelMethodStat `json:"emerging_methods"`
}

type IntelFamilyYear struct {
	Family string `json:"family"`
	Year   int    `json:"year"`
	Count  int    `json:"count"`
}

func (r *GraphRepo) GetIntelOverview(ctx context.Context, corpusID string) (IntelOverview, error) {
	if err := r.ensureKGSchema(ctx); err != nil {
		return IntelOverview{}, err
	}

	out := IntelOverview{}
	rows, err := r.db.Pool.Query(ctx, `
SELECT n.node_id, n.label,
  COUNT(DISTINCT CASE WHEN e.edge_type IN ('EXTENDS','BASED_ON') THEN CASE WHEN e.source_node_id=n.node_id THEN e.target_node_id ELSE e.source_node_id END END) AS linked_methods,
  COALESCE(SUM(COALESCE((e.payload->>'support_count')::int,1)), 0) AS support_count,
  COUNT(DISTINCT CASE WHEN e.edge_type='OUTPERFORMS' AND e.source_node_id=n.node_id THEN e.target_node_id END) AS outperform_wins
FROM graph_nodes n
LEFT JOIN graph_edges e ON e.corpus_id=n.corpus_id AND (e.source_node_id=n.node_id OR e.target_node_id=n.node_id)
WHERE n.corpus_id=$1::uuid AND n.node_type='method'
GROUP BY n.node_id, n.label
ORDER BY linked_methods DESC, support_count DESC
LIMIT 300`, corpusID)
	if err != nil {
		return IntelOverview{}, fmt.Errorf("overview methods query: %w", err)
	}
	defer rows.Close()
	methodAgg := map[string]IntelMethodFamily{}
	for rows.Next() {
		var m IntelMethodFamily
		if err := rows.Scan(&m.NodeID, &m.Label, &m.LinkedMethods, &m.SupportCount, &m.OutperformWins); err != nil {
			return IntelOverview{}, fmt.Errorf("overview methods scan: %w", err)
		}
		key := canonicalMethodLabel(m.Label)
		if key == "" || isGenericMethodLabel(key) {
			continue
		}
		cur := methodAgg[key]
		if cur.NodeID == "" {
			cur = IntelMethodFamily{NodeID: m.NodeID, Label: key}
		}
		cur.LinkedMethods += m.LinkedMethods
		cur.SupportCount += m.SupportCount
		cur.OutperformWins += m.OutperformWins
		methodAgg[key] = cur
	}
	if err := rows.Err(); err != nil {
		return IntelOverview{}, err
	}
	out.TopMethodFamilies = topMethodFamilies(methodAgg, 5)

	dsRows, err := r.db.Pool.Query(ctx, `
SELECT d.node_id, d.label,
  COUNT(*) AS usage_count,
  COUNT(DISTINCT m.node_id) AS method_count
FROM graph_nodes d
JOIN graph_edges e ON e.corpus_id=d.corpus_id AND (
  (e.target_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')) OR
  (e.source_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET'))
)
JOIN graph_nodes m ON m.corpus_id=d.corpus_id AND m.node_type='method' AND (m.node_id=e.source_node_id OR m.node_id=e.target_node_id)
WHERE d.corpus_id=$1::uuid AND d.node_type='dataset'
GROUP BY d.node_id, d.label
ORDER BY usage_count DESC
LIMIT 500`, corpusID)
	if err != nil {
		return IntelOverview{}, fmt.Errorf("overview datasets query: %w", err)
	}
	defer dsRows.Close()
	datasetAgg := map[string]IntelDatasetStat{}
	for dsRows.Next() {
		var d IntelDatasetStat
		if err := dsRows.Scan(&d.NodeID, &d.Label, &d.UsageCount, &d.MethodCount); err != nil {
			return IntelOverview{}, fmt.Errorf("overview datasets scan: %w", err)
		}
		key := canonicalDatasetLabel(d.Label)
		if key == "" || isLowQualityDatasetLabel(key) {
			continue
		}
		cur := datasetAgg[key]
		if cur.NodeID == "" {
			cur = IntelDatasetStat{NodeID: d.NodeID, Label: key}
		}
		cur.UsageCount += d.UsageCount
		if d.MethodCount > cur.MethodCount {
			cur.MethodCount = d.MethodCount
		}
		datasetAgg[key] = cur
	}
	if err := dsRows.Err(); err != nil {
		return IntelOverview{}, err
	}
	out.TopDatasets = topDatasets(datasetAgg, 5)

	winRows, err := r.db.Pool.Query(ctx, `
SELECT n.node_id, n.label, COUNT(*)::float AS score
FROM graph_nodes n
JOIN graph_edges e ON e.corpus_id=n.corpus_id AND e.source_node_id=n.node_id AND e.edge_type='OUTPERFORMS'
WHERE n.corpus_id=$1::uuid AND n.node_type='method'
GROUP BY n.node_id, n.label
ORDER BY score DESC
LIMIT 300`, corpusID)
	if err != nil {
		return IntelOverview{}, fmt.Errorf("overview outperformers query: %w", err)
	}
	defer winRows.Close()
	winAgg := map[string]IntelMethodStat{}
	for winRows.Next() {
		var s IntelMethodStat
		if err := winRows.Scan(&s.NodeID, &s.Label, &s.Score); err != nil {
			return IntelOverview{}, fmt.Errorf("overview outperformers scan: %w", err)
		}
		key := canonicalMethodLabel(s.Label)
		if key == "" || isGenericMethodLabel(key) {
			continue
		}
		cur := winAgg[key]
		if cur.NodeID == "" {
			cur = IntelMethodStat{NodeID: s.NodeID, Label: key}
		}
		cur.Score += s.Score
		winAgg[key] = cur
	}
	if err := winRows.Err(); err != nil {
		return IntelOverview{}, err
	}
	out.TopOutperformers = topMethodStats(winAgg, 5)
	return out, nil
}

func (r *GraphRepo) GetIntelLineage(ctx context.Context, corpusID, method string, depth int) (IntelLineage, error) {
	if err := r.ensureKGSchema(ctx); err != nil {
		return IntelLineage{}, err
	}
	if depth <= 0 {
		depth = 4
	}
	if depth > 8 {
		depth = 8
	}
	var root IntelLineageNode
	err := r.db.Pool.QueryRow(ctx, `
SELECT node_id, label
FROM graph_nodes
WHERE corpus_id=$1::uuid AND node_type='method' AND (LOWER(label)=LOWER($2) OR LOWER(label) LIKE LOWER($2 || '%'))
ORDER BY CASE WHEN LOWER(label)=LOWER($2) THEN 0 ELSE 1 END, LENGTH(label)
LIMIT 1`, corpusID, method).Scan(&root.NodeID, &root.Label)
	if err != nil {
		return IntelLineage{}, fmt.Errorf("lineage root not found for method=%s", method)
	}
	root.Depth = 0
	out := IntelLineage{RootMethod: root}

	rows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE walk AS (
  SELECT $2::text AS node_id, 0::int AS depth
  UNION
  SELECT e.source_node_id, w.depth + 1
  FROM graph_edges e
  JOIN walk w ON e.target_node_id = w.node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND w.depth < $3
),
uniq AS (
  SELECT node_id, MIN(depth) AS depth
  FROM walk
  GROUP BY node_id
)
SELECT u.node_id, n.label, u.depth
FROM uniq u
JOIN graph_nodes n ON n.node_id=u.node_id
ORDER BY u.depth, n.label`, corpusID, root.NodeID, depth)
	if err != nil {
		return IntelLineage{}, fmt.Errorf("lineage nodes query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n IntelLineageNode
		if err := rows.Scan(&n.NodeID, &n.Label, &n.Depth); err != nil {
			return IntelLineage{}, fmt.Errorf("lineage node scan: %w", err)
		}
		out.TreeNodes = append(out.TreeNodes, n)
	}
	if err := rows.Err(); err != nil {
		return IntelLineage{}, err
	}

	edgeRows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE walk AS (
  SELECT $2::text AS node_id, 0::int AS depth
  UNION
  SELECT e.source_node_id, w.depth + 1
  FROM graph_edges e
  JOIN walk w ON e.target_node_id = w.node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND w.depth < $3
),
uniq AS (SELECT DISTINCT node_id FROM walk)
SELECT e.source_node_id, ns.label, e.target_node_id, nt.label, e.edge_type
FROM graph_edges e
JOIN uniq s ON s.node_id=e.source_node_id
JOIN uniq t ON t.node_id=e.target_node_id
JOIN graph_nodes ns ON ns.node_id=e.source_node_id
JOIN graph_nodes nt ON nt.node_id=e.target_node_id
WHERE e.corpus_id=$1::uuid AND e.edge_type IN ('EXTENDS','BASED_ON')`, corpusID, root.NodeID, depth)
	if err != nil {
		return IntelLineage{}, fmt.Errorf("lineage edges query: %w", err)
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var e LineageEdge
		e.Depth = 0
		if err := edgeRows.Scan(&e.SourceID, &e.SourceName, &e.TargetID, &e.TargetName, &e.EdgeType); err != nil {
			return IntelLineage{}, fmt.Errorf("lineage edge scan: %w", err)
		}
		out.TreeEdges = append(out.TreeEdges, e)
	}
	if err := edgeRows.Err(); err != nil {
		return IntelLineage{}, err
	}

	yearRows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE walk AS (
  SELECT $2::text AS node_id, 0::int AS depth
  UNION
  SELECT e.source_node_id, w.depth + 1
  FROM graph_edges e
  JOIN walk w ON e.target_node_id = w.node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND w.depth < $3
),
uniq AS (SELECT DISTINCT node_id FROM walk),
prov AS (
  SELECT DISTINCT (pr->>'paper_id') AS paper_id
  FROM graph_edges e
  JOIN uniq u ON u.node_id=e.source_node_id OR u.node_id=e.target_node_id
  JOIN LATERAL jsonb_array_elements(COALESCE(e.payload->'provenance','[]'::jsonb)) pr ON true
  WHERE e.corpus_id=$1::uuid
)
SELECT p.year, COUNT(*)::int
FROM prov
JOIN papers p ON p.corpus_id=$1::uuid AND p.paper_id=prov.paper_id
WHERE p.year IS NOT NULL
GROUP BY p.year
ORDER BY p.year`, corpusID, root.NodeID, depth)
	if err == nil {
		defer yearRows.Close()
		for yearRows.Next() {
			var y IntelYearCount
			if err := yearRows.Scan(&y.Year, &y.Count); err != nil {
				return IntelLineage{}, fmt.Errorf("lineage timeline scan: %w", err)
			}
			out.Timeline = append(out.Timeline, y)
		}
	}

	dsRows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE walk AS (
  SELECT $2::text AS node_id, 0::int AS depth
  UNION
  SELECT e.source_node_id, w.depth + 1
  FROM graph_edges e
  JOIN walk w ON e.target_node_id = w.node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND w.depth < $3
),
methods AS (SELECT DISTINCT node_id FROM walk),
ds AS (
  SELECT d.node_id, d.label, COUNT(*)::int usage_count
  FROM graph_nodes d
  JOIN graph_edges e ON e.corpus_id=d.corpus_id
  JOIN methods m ON m.node_id=e.source_node_id OR m.node_id=e.target_node_id
  WHERE d.corpus_id=$1::uuid AND d.node_type='dataset'
    AND ((e.source_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')) OR (e.target_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')))
  GROUP BY d.node_id, d.label
)
SELECT node_id, label, usage_count, 0::int method_count
FROM ds
ORDER BY usage_count DESC
LIMIT 10`, corpusID, root.NodeID, depth)
	if err == nil {
		defer dsRows.Close()
		for dsRows.Next() {
			var d IntelDatasetStat
			if err := dsRows.Scan(&d.NodeID, &d.Label, &d.UsageCount, &d.MethodCount); err != nil {
				return IntelLineage{}, fmt.Errorf("lineage datasets scan: %w", err)
			}
			out.Datasets = append(out.Datasets, d)
		}
	}

	tdRows, err := r.db.Pool.Query(ctx, `
WITH RECURSIVE walk AS (
  SELECT $2::text AS node_id, 0::int AS depth
  UNION
  SELECT e.source_node_id, w.depth + 1
  FROM graph_edges e
  JOIN walk w ON e.target_node_id = w.node_id
  WHERE e.corpus_id = $1::uuid
    AND e.edge_type IN ('EXTENDS','BASED_ON')
    AND w.depth < $3
),
methods AS (SELECT DISTINCT node_id FROM walk WHERE node_id<>$2::text)
SELECT n.node_id, n.label, COUNT(*)::float AS score
FROM methods m
JOIN graph_nodes n ON n.node_id=m.node_id
LEFT JOIN graph_edges e ON e.corpus_id=$1::uuid AND e.source_node_id=n.node_id AND e.edge_type='OUTPERFORMS'
GROUP BY n.node_id, n.label
ORDER BY score DESC, n.label
LIMIT 10`, corpusID, root.NodeID, depth)
	if err == nil {
		defer tdRows.Close()
		for tdRows.Next() {
			var s IntelMethodStat
			if err := tdRows.Scan(&s.NodeID, &s.Label, &s.Score); err != nil {
				return IntelLineage{}, fmt.Errorf("lineage descendants scan: %w", err)
			}
			out.TopDescendants = append(out.TopDescendants, s)
		}
	}
	return out, nil
}

func (r *GraphRepo) GetIntelPerformance(ctx context.Context, corpusID string, topN int) (IntelPerformance, error) {
	if err := r.ensureKGSchema(ctx); err != nil {
		return IntelPerformance{}, err
	}
	if topN <= 0 {
		topN = 20
	}
	rows, err := r.db.Pool.Query(ctx, `
WITH methods AS (
  SELECT node_id, label
  FROM graph_nodes
  WHERE corpus_id=$1::uuid AND node_type='method'
),
win_loss AS (
  SELECT m.node_id,
    COUNT(DISTINCT CASE WHEN e.edge_type='OUTPERFORMS' AND e.source_node_id=m.node_id THEN e.target_node_id END) AS beaten_methods,
    COUNT(CASE WHEN e.edge_type='OUTPERFORMS' AND e.source_node_id=m.node_id THEN 1 END) AS wins,
    COUNT(CASE WHEN e.edge_type='OUTPERFORMS' AND e.target_node_id=m.node_id THEN 1 END) AS losses
  FROM methods m
  LEFT JOIN graph_edges e ON e.corpus_id=$1::uuid AND (e.source_node_id=m.node_id OR e.target_node_id=m.node_id)
  GROUP BY m.node_id
),
coverage AS (
  SELECT m.node_id,
    COUNT(DISTINCT CASE WHEN d.node_type='dataset' THEN d.node_id END) AS dataset_coverage,
    COUNT(DISTINCT CASE WHEN d.node_type='metric' THEN d.node_id END) AS metric_coverage
  FROM methods m
  LEFT JOIN graph_edges e ON e.corpus_id=$1::uuid AND (e.source_node_id=m.node_id OR e.target_node_id=m.node_id)
  LEFT JOIN graph_nodes d ON d.corpus_id=$1::uuid AND (d.node_id=e.source_node_id OR d.node_id=e.target_node_id) AND d.node_id<>m.node_id
  GROUP BY m.node_id
)
SELECT m.label, w.beaten_methods::int, w.wins::int, w.losses::int,
  CASE WHEN (w.wins+w.losses)=0 THEN 0 ELSE (w.wins::float / (w.wins+w.losses)::float) END AS win_rate,
  c.dataset_coverage::int, c.metric_coverage::int,
  ((w.wins - w.losses) * LN(1 + GREATEST(1, c.dataset_coverage + c.metric_coverage)))::float AS dominance_score
FROM methods m
JOIN win_loss w ON w.node_id=m.node_id
JOIN coverage c ON c.node_id=m.node_id
ORDER BY dominance_score DESC, w.wins DESC
LIMIT 500`, corpusID)
	if err != nil {
		return IntelPerformance{}, fmt.Errorf("performance matrix query: %w", err)
	}
	defer rows.Close()
	type agg struct {
		beatenMethods   int
		wins            int
		losses          int
		datasetCoverage int
		metricCoverage  int
	}
	perfAgg := map[string]agg{}
	for rows.Next() {
		var rrow IntelPerformanceRow
		if err := rows.Scan(&rrow.Method, &rrow.BeatenMethods, &rrow.Wins, &rrow.Losses, &rrow.WinRate, &rrow.DatasetCoverage, &rrow.MetricCoverage, &rrow.DominanceScore); err != nil {
			return IntelPerformance{}, fmt.Errorf("performance matrix scan: %w", err)
		}
		key := canonicalMethodLabel(rrow.Method)
		if key == "" || isGenericMethodLabel(key) {
			continue
		}
		cur := perfAgg[key]
		cur.beatenMethods += rrow.BeatenMethods
		cur.wins += rrow.Wins
		cur.losses += rrow.Losses
		if rrow.DatasetCoverage > cur.datasetCoverage {
			cur.datasetCoverage = rrow.DatasetCoverage
		}
		if rrow.MetricCoverage > cur.metricCoverage {
			cur.metricCoverage = rrow.MetricCoverage
		}
		perfAgg[key] = cur
	}
	if err := rows.Err(); err != nil {
		return IntelPerformance{}, err
	}
	out := IntelPerformance{Rows: make([]IntelPerformanceRow, 0, len(perfAgg))}
	for method, a := range perfAgg {
		winRate := 0.0
		if a.wins+a.losses > 0 {
			winRate = float64(a.wins) / float64(a.wins+a.losses)
		}
		dominance := float64(a.wins-a.losses) * math.Log1p(float64(maxInt(1, a.datasetCoverage+a.metricCoverage)))
		out.Rows = append(out.Rows, IntelPerformanceRow{
			Method:          method,
			BeatenMethods:   a.beatenMethods,
			Wins:            a.wins,
			Losses:          a.losses,
			WinRate:         winRate,
			DatasetCoverage: a.datasetCoverage,
			MetricCoverage:  a.metricCoverage,
			DominanceScore:  dominance,
		})
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].DominanceScore != out.Rows[j].DominanceScore {
			return out.Rows[i].DominanceScore > out.Rows[j].DominanceScore
		}
		if out.Rows[i].Wins != out.Rows[j].Wins {
			return out.Rows[i].Wins > out.Rows[j].Wins
		}
		return out.Rows[i].Method < out.Rows[j].Method
	})
	if len(out.Rows) > topN {
		out.Rows = out.Rows[:topN]
	}
	return out, nil
}

func (r *GraphRepo) GetIntelDatasets(ctx context.Context, corpusID string, topN int) (IntelDatasetDominance, error) {
	if err := r.ensureKGSchema(ctx); err != nil {
		return IntelDatasetDominance{}, err
	}
	if topN <= 0 {
		topN = 10
	}
	out := IntelDatasetDominance{}
	rows, err := r.db.Pool.Query(ctx, `
SELECT d.node_id, d.label,
  COUNT(*)::int AS usage_count,
  COUNT(DISTINCT m.node_id)::int AS method_count
FROM graph_nodes d
JOIN graph_edges e ON e.corpus_id=d.corpus_id
JOIN graph_nodes m ON m.corpus_id=d.corpus_id AND m.node_type='method' AND (m.node_id=e.source_node_id OR m.node_id=e.target_node_id)
WHERE d.corpus_id=$1::uuid AND d.node_type='dataset'
  AND ((e.target_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')) OR (e.source_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')))
GROUP BY d.node_id, d.label
ORDER BY usage_count DESC
LIMIT $2`, corpusID, topN)
	if err != nil {
		return IntelDatasetDominance{}, fmt.Errorf("dataset rankings query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d IntelDatasetStat
		if err := rows.Scan(&d.NodeID, &d.Label, &d.UsageCount, &d.MethodCount); err != nil {
			return IntelDatasetDominance{}, fmt.Errorf("dataset rankings scan: %w", err)
		}
		out.Datasets = append(out.Datasets, d)
	}
	if err := rows.Err(); err != nil {
		return IntelDatasetDominance{}, err
	}

	seriesRows, err := r.db.Pool.Query(ctx, `
WITH prov AS (
  SELECT d.label AS dataset, p.year AS year
  FROM graph_nodes d
  JOIN graph_edges e ON e.corpus_id=d.corpus_id
  JOIN LATERAL jsonb_array_elements(COALESCE(e.payload->'provenance','[]'::jsonb)) pr ON true
  JOIN papers p ON p.corpus_id=d.corpus_id AND p.paper_id=(pr->>'paper_id')
  WHERE d.corpus_id=$1::uuid AND d.node_type='dataset' AND p.year IS NOT NULL
    AND ((e.target_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')) OR (e.source_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')))
)
SELECT dataset, year, COUNT(*)::int
FROM prov
GROUP BY dataset, year
ORDER BY year, dataset
LIMIT 500`, corpusID)
	if err == nil {
		defer seriesRows.Close()
		for seriesRows.Next() {
			var y IntelDatasetYear
			if err := seriesRows.Scan(&y.Dataset, &y.Year, &y.Count); err != nil {
				return IntelDatasetDominance{}, fmt.Errorf("dataset year scan: %w", err)
			}
			out.UsageByYear = append(out.UsageByYear, y)
		}
	}

	shareRows, err := r.db.Pool.Query(ctx, `
WITH base AS (
  SELECT d.label AS dataset, m.label AS method, COUNT(*)::float AS c
  FROM graph_nodes d
  JOIN graph_edges e ON e.corpus_id=d.corpus_id
  JOIN graph_nodes m ON m.corpus_id=d.corpus_id AND m.node_type='method' AND (m.node_id=e.source_node_id OR m.node_id=e.target_node_id)
  WHERE d.corpus_id=$1::uuid AND d.node_type='dataset'
    AND ((e.target_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')) OR (e.source_node_id=d.node_id AND e.edge_type IN ('EVALUATED_ON','USES_DATASET')))
  GROUP BY d.label, m.label
),
tot AS (
  SELECT dataset, SUM(c) AS total
  FROM base
  GROUP BY dataset
)
SELECT b.dataset, b.method, CASE WHEN t.total=0 THEN 0 ELSE b.c/t.total END AS share
FROM base b
JOIN tot t ON t.dataset=b.dataset
ORDER BY b.dataset, share DESC
LIMIT 300`, corpusID)
	if err == nil {
		defer shareRows.Close()
		for shareRows.Next() {
			var s IntelDatasetShare
			if err := shareRows.Scan(&s.Dataset, &s.Method, &s.Share); err != nil {
				return IntelDatasetDominance{}, fmt.Errorf("dataset share scan: %w", err)
			}
			out.MethodDistribution = append(out.MethodDistribution, s)
		}
	}
	return out, nil
}

func (r *GraphRepo) GetIntelTrends(ctx context.Context, corpusID string, topN int) (IntelTrendTimeline, error) {
	if err := r.ensureKGSchema(ctx); err != nil {
		return IntelTrendTimeline{}, err
	}
	if topN <= 0 {
		topN = 10
	}
	out := IntelTrendTimeline{}
	rows, err := r.db.Pool.Query(ctx, `
WITH series AS (
  SELECT m.label AS family, p.year AS year, COUNT(*)::int AS c
  FROM graph_edges e
  JOIN graph_nodes m ON m.corpus_id=e.corpus_id AND m.node_type='method' AND (m.node_id=e.source_node_id OR m.node_id=e.target_node_id)
  JOIN LATERAL jsonb_array_elements(COALESCE(e.payload->'provenance','[]'::jsonb)) pr ON true
  JOIN papers p ON p.corpus_id=e.corpus_id AND p.paper_id=(pr->>'paper_id')
  WHERE e.corpus_id=$1::uuid AND e.edge_type='PROPOSES' AND p.year IS NOT NULL
  GROUP BY m.label, p.year
),
ranked AS (
  SELECT family, SUM(c) AS total
  FROM series
  GROUP BY family
  ORDER BY total DESC
  LIMIT $2
)
SELECT s.family, s.year, s.c
FROM series s
JOIN ranked r ON r.family=s.family
ORDER BY s.family, s.year`, corpusID, topN)
	if err != nil {
		return IntelTrendTimeline{}, fmt.Errorf("trend series query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f IntelFamilyYear
		if err := rows.Scan(&f.Family, &f.Year, &f.Count); err != nil {
			return IntelTrendTimeline{}, fmt.Errorf("trend series scan: %w", err)
		}
		out.FamilySeries = append(out.FamilySeries, f)
	}
	if err := rows.Err(); err != nil {
		return IntelTrendTimeline{}, err
	}

	emerRows, err := r.db.Pool.Query(ctx, `
WITH series AS (
  SELECT m.node_id, m.label, p.year, COUNT(*)::float AS c
  FROM graph_edges e
  JOIN graph_nodes m ON m.corpus_id=e.corpus_id AND m.node_type='method' AND (m.node_id=e.source_node_id OR m.node_id=e.target_node_id)
  JOIN LATERAL jsonb_array_elements(COALESCE(e.payload->'provenance','[]'::jsonb)) pr ON true
  JOIN papers p ON p.corpus_id=e.corpus_id AND p.paper_id=(pr->>'paper_id')
  WHERE e.corpus_id=$1::uuid AND e.edge_type='PROPOSES' AND p.year IS NOT NULL
  GROUP BY m.node_id, m.label, p.year
),
latest AS (
  SELECT node_id, label, MAX(year) AS y
  FROM series
  GROUP BY node_id, label
),
calc AS (
  SELECT l.node_id, l.label,
    COALESCE(s1.c,0) AS c_now,
    COALESCE(s0.c,0) AS c_prev,
    CASE WHEN COALESCE(s0.c,0)=0 THEN COALESCE(s1.c,0) ELSE (COALESCE(s1.c,0)-s0.c)/s0.c END AS growth
  FROM latest l
  LEFT JOIN series s1 ON s1.node_id=l.node_id AND s1.year=l.y
  LEFT JOIN series s0 ON s0.node_id=l.node_id AND s0.year=l.y-1
)
SELECT node_id, label, (growth * LN(1 + GREATEST(c_now,1)))::float AS score
FROM calc
ORDER BY score DESC
LIMIT 10`, corpusID)
	if err == nil {
		defer emerRows.Close()
		for emerRows.Next() {
			var m IntelMethodStat
			if err := emerRows.Scan(&m.NodeID, &m.Label, &m.Score); err != nil {
				return IntelTrendTimeline{}, fmt.Errorf("emerging methods scan: %w", err)
			}
			out.EmergingMethods = append(out.EmergingMethods, m)
		}
	}
	return out, nil
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9\s]`)
var multiSpace = regexp.MustCompile(`\s+`)

func normalizeLabel(s string) string {
	x := strings.TrimSpace(strings.ToLower(s))
	x = strings.ReplaceAll(x, "_", " ")
	x = strings.ReplaceAll(x, "-", " ")
	x = nonAlphaNum.ReplaceAllString(x, " ")
	x = multiSpace.ReplaceAllString(x, " ")
	return strings.TrimSpace(x)
}

func canonicalMethodLabel(s string) string {
	x := normalizeLabel(s)
	if x == "" {
		return ""
	}
	// Merge common variant suffixes: "qphh expanded", "qphh plain" -> "qphh"
	parts := strings.Fields(x)
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if last == "base" || last == "variant" {
			x = strings.Join(parts[:len(parts)-1], " ")
		}
	}
	// Drop generic trailing qualifiers.
	x = strings.TrimSuffix(x, " method")
	x = strings.TrimSuffix(x, " algorithm")
	x = strings.TrimSuffix(x, " model")
	return strings.TrimSpace(x)
}

func canonicalDatasetLabel(s string) string {
	x := normalizeLabel(s)
	if x == "" {
		return ""
	}
	x = strings.TrimSuffix(x, " dataset")
	x = strings.TrimSuffix(x, " datasets")
	return strings.TrimSpace(x)
}

func isGenericMethodLabel(x string) bool {
	if x == "" {
		return true
	}
	blacklist := map[string]bool{
		"proposed": true, "proposed method": true, "proposed algorithm": true,
		"proposed approach": true, "existing studies": true, "this paper": true,
		"method": true, "algorithm": true, "model": true, "baseline": true,
		"approach": true,
	}
	if blacklist[x] {
		return true
	}
	if strings.HasPrefix(x, "proposed ") || strings.HasPrefix(x, "existing ") {
		return true
	}
	return false
}

func isLowQualityDatasetLabel(x string) bool {
	if x == "" {
		return true
	}
	// Filter quantity-like or generic fragments masquerading as datasets.
	badContains := []string{
		"users", "locations", "resources", "given resources", "latency measurements",
		"cloud computing environments", "cost", "energy consumption", "servers",
	}
	for _, b := range badContains {
		if strings.Contains(x, b) {
			return true
		}
	}
	// Tiny generic nouns are usually extraction noise.
	if x == "dataset" || x == "data" || x == "evaluation" {
		return true
	}
	return false
}

func topMethodFamilies(m map[string]IntelMethodFamily, n int) []IntelMethodFamily {
	out := make([]IntelMethodFamily, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LinkedMethods != out[j].LinkedMethods {
			return out[i].LinkedMethods > out[j].LinkedMethods
		}
		if out[i].SupportCount != out[j].SupportCount {
			return out[i].SupportCount > out[j].SupportCount
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func topMethodStats(m map[string]IntelMethodStat, n int) []IntelMethodStat {
	out := make([]IntelMethodStat, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func topDatasets(m map[string]IntelDatasetStat, n int) []IntelDatasetStat {
	out := make([]IntelDatasetStat, 0, len(m))
	for _, v := range m {
		// Require some cross-method support for quality.
		if v.MethodCount < 1 {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UsageCount != out[j].UsageCount {
			return out[i].UsageCount > out[j].UsageCount
		}
		if out[i].MethodCount != out[j].MethodCount {
			return out[i].MethodCount > out[j].MethodCount
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
