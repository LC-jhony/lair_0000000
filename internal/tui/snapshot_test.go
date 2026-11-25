package tui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lair-sh/lair/internal/daemon"
	"github.com/lair-sh/lair/internal/engine"
)

type recordingCaller struct {
	ops map[string]string
}

func (r *recordingCaller) Call(_ context.Context, action string, payload, out any) error {
	if action == daemon.ActionMail || action == daemon.ActionDumps {
		raw, _ := json.Marshal(payload)
		var req struct {
			Op string `json:"op"`
		}
		_ = json.Unmarshal(raw, &req)
		if r.ops == nil {
			r.ops = map[string]string{}
		}
		r.ops[action] = req.Op
		switch action {
		case daemon.ActionMail:
			*(out.(*engine.MailResult)) = engine.MailResult{
				Messages: []engine.MailMessage{{ID: "m1", Subject: "hola"}}, Count: 1,
			}
		case daemon.ActionDumps:
			*(out.(*engine.DumpsResult)) = engine.DumpsResult{
				Events: []engine.DumpEvent{{ID: "d1", Message: "select 1"}}, Count: 1,
			}
		}
	}
	return nil
}

func TestSnapshotLoadsDebugLists(t *testing.T) {
	ctx := context.Background()
	c := &recordingCaller{}
	s := loadSnapshot(ctx, c)
	if c.ops[daemon.ActionMail] != "list" || c.ops[daemon.ActionDumps] != "list" {
		t.Fatalf("debug ops = %v, want list/list", c.ops)
	}
	if len(s.Mail.Messages) != 1 || len(s.Dumps.Events) != 1 {
		t.Fatalf("snapshot did not carry debug collections: mail=%d dumps=%d",
			len(s.Mail.Messages), len(s.Dumps.Events))
	}
}
