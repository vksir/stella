// internal/memory/vector.go
package memory

import (
	"context"
	"math"
	"sort"
	"stella/ent"
	"stella/ent/memory"
	"stella/pkg/database"
)

type MemoryScore struct {
	Memory *ent.Memory
	Score  float64
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		va, vb := float64(a[i]), float64(b[i])
		dotProduct += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func RetrieveTopK(ctx context.Context, userID string, queryVector []float32, k int) ([]*ent.Memory, error) {
	memories, err := database.G.Memory.Query().
		Where(memory.UserID(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var scores []MemoryScore
	for _, m := range memories {
		score := CosineSimilarity(queryVector, m.Vector)
		scores = append(scores, MemoryScore{Memory: m, Score: score})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	if k > len(scores) {
		k = len(scores)
	}

	var topK []*ent.Memory
	for i := 0; i < k; i++ {
		topK = append(topK, scores[i].Memory)
	}

	return topK, nil
}
