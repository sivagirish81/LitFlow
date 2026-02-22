package storage

import (
	"context"
	"fmt"
	"sync"
)

type GraphNode struct {
	NodeID   string         `json:"node_id"`
	NodeType string         `json:"node_type"`
	Label    string         `json:"label"`
	Payload  map[string]any `json:"payload"`
}

type GraphEdge struct {
	EdgeID       string         `json:"edge_id"`
	SourceNodeID string         `json:"source_node_id"`
	TargetNodeID string         `json:"target_node_id"`
	EdgeType     string         `json:"edge_type"`
	Weight       float64        `json:"weight"`
	Payload      map[string]any `json:"payload"`
}

type GraphRepo struct {
	db *DB

	kgSchemaMu       sync.Mutex
	kgSchemaPrepared bool
}

func NewGraphRepo(db *DB) *GraphRepo {
	return &GraphRepo{db: db}
}

func (r *GraphRepo) UpsertTopicRetrieval(ctx context.Context, corpusID, topic, paperID, title string, score float64, chunkID string) error {
	topicNodeID := "topic:" + corpusID + ":" + topic
	paperNodeID := "paper:" + paperID
	edgeID := "edge:" + corpusID + ":" + topicNodeID + ":" + paperNodeID + ":retrieved_for_topic"
	_, err := r.db.Pool.Exec(ctx, `
INSERT INTO graph_nodes(node_id, corpus_id, node_type, label, payload)
VALUES ($1, $2::uuid, 'topic', $3, '{}'::jsonb)
ON CONFLICT (node_id) DO UPDATE SET label = EXCLUDED.label`, topicNodeID, corpusID, topic)
	if err != nil {
		return fmt.Errorf("upsert topic node: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
INSERT INTO graph_nodes(node_id, corpus_id, node_type, label, payload)
VALUES ($1, $2::uuid, 'paper', $3, jsonb_build_object('paper_id', $4))
ON CONFLICT (node_id) DO UPDATE SET label = EXCLUDED.label`, paperNodeID, corpusID, title, paperID)
	if err != nil {
		return fmt.Errorf("upsert paper node: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
INSERT INTO graph_edges(edge_id, corpus_id, source_node_id, target_node_id, edge_type, weight, payload)
VALUES ($1, $2::uuid, $3, $4, 'retrieved_for_topic', $5, jsonb_build_object('chunk_id', $6))
ON CONFLICT (corpus_id, source_node_id, target_node_id, edge_type)
DO UPDATE SET weight = GREATEST(graph_edges.weight, EXCLUDED.weight), payload = EXCLUDED.payload`,
		edgeID, corpusID, paperNodeID, topicNodeID, score, chunkID)
	if err != nil {
		return fmt.Errorf("upsert graph edge: %w", err)
	}
	return nil
}

func (r *GraphRepo) GetGraph(ctx context.Context, corpusID string) ([]GraphNode, []GraphEdge, error) {
	nodesRows, err := r.db.Pool.Query(ctx, `SELECT node_id, node_type, label FROM graph_nodes WHERE corpus_id=$1::uuid`, corpusID)
	if err != nil {
		return nil, nil, fmt.Errorf("query graph nodes: %w", err)
	}
	defer nodesRows.Close()
	nodes := make([]GraphNode, 0)
	for nodesRows.Next() {
		var n GraphNode
		if err := nodesRows.Scan(&n.NodeID, &n.NodeType, &n.Label); err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, n)
	}
	edgesRows, err := r.db.Pool.Query(ctx, `SELECT edge_id, source_node_id, target_node_id, edge_type, weight FROM graph_edges WHERE corpus_id=$1::uuid`, corpusID)
	if err != nil {
		return nil, nil, fmt.Errorf("query graph edges: %w", err)
	}
	defer edgesRows.Close()
	edges := make([]GraphEdge, 0)
	for edgesRows.Next() {
		var e GraphEdge
		if err := edgesRows.Scan(&e.EdgeID, &e.SourceNodeID, &e.TargetNodeID, &e.EdgeType, &e.Weight); err != nil {
			return nil, nil, err
		}
		edges = append(edges, e)
	}
	return nodes, edges, nil
}
