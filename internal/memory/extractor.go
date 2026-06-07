package memory

import (
	"context"
	"fmt"
	"stella/pkg/database"
)

func AsyncExtractAndStore(userID string, userMessage, assistantResponse string) {
	// TODO: Use LLM to extract memory from conversation and embed the result
	// ...
	ctx := context.Background()
	extractedFact := ""
	var embedding []float32

	if extractedFact != "" {
		_, err := database.G.Memory.Create().
			SetUserID(userID).
			SetContent(extractedFact).
			SetVector(embedding).
			Save(ctx)
		if err != nil {
			fmt.Printf("Error storing memory: %v\n", err)
		}
	}
}
