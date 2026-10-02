package handler

import (
	"context"
	"strings"
	"testing"
)

func TestUpdateProgressStores(t *testing.T) {
	for _, redisBacked := range []bool{false, true} {
		name := "memory"
		if redisBacked {
			name = "redis"
		}
		t.Run(name, func(t *testing.T) {
			var store UpdateStore = NewInMemoryUpdateStore()
			if redisBacked {
				store = NewRedisUpdateStore(newRedisTestClient(t))
			}
			ctx := context.Background()
			req, err := store.Create(ctx, "runtime-progress", "next", "user")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.PopPending(ctx, req.RuntimeID); err != nil {
				t.Fatal(err)
			}
			for _, line := range []string{"Downloading", "Verifying"} {
				if err = store.Progress(ctx, req.ID, line); err != nil {
					t.Fatal(err)
				}
			}
			got, err := store.Get(ctx, req.ID)
			if err != nil || got.Output != "Downloading\nVerifying" {
				t.Fatalf("progress=%+v err=%v", got, err)
			}
			if err = store.Fail(ctx, req.ID, "checksum mismatch"); err != nil {
				t.Fatal(err)
			}
			if err = store.Progress(ctx, req.ID, "late report"); err != nil {
				t.Fatal(err)
			}
			got, err = store.Get(ctx, req.ID)
			if err != nil || got.Status != UpdateFailed || got.Output != "Downloading\nVerifying" {
				t.Fatalf("terminal=%+v err=%v", got, err)
			}
		})
	}
	if got := appendUpdateOutput("earlier", strings.Repeat("x", 40000)); len(got) != 32768 {
		t.Fatalf("unbounded output %d", len(got))
	}
}
