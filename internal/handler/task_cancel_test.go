package handler

import (
	"context"
	"testing"

	"light-ai/internal/eino"
)

func TestShouldForwardTaskStepAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if shouldForwardTaskStep(ctx, eino.TaskStep{Type: "content"}) {
		t.Fatal("content should not be forwarded after task cancellation")
	}
	if shouldForwardTaskStep(ctx, eino.TaskStep{Type: "done"}) {
		t.Fatal("done should not be forwarded after task cancellation")
	}
	if shouldForwardTaskStep(ctx, eino.TaskStep{Type: "error"}) {
		t.Fatal("error should not be forwarded after task cancellation")
	}
}
