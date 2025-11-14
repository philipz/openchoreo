package clickstack

import (
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/pkg/telemetry/query"
)

func TestComponentLogsQuery(t *testing.T) {
	cfg := config.ClickStackConfig{
		LogsTable:   "telemetry.logs_mv",
		TracesTable: "telemetry.traces_mv",
	}
	builder := NewQueryBuilder(cfg)
	start := time.Now().Add(-time.Hour)
	end := time.Now()

	sqlStmt, args, err := builder.ComponentLogs(query.ComponentLogQuery{
		BaseLogQuery: query.BaseLogQuery{
			TimeRange: query.TimeRange{
				Start: start,
				End:   end,
			},
			Limit:     50,
			SortOrder: query.SortDesc,
		},
		ComponentID:   "comp-1",
		EnvironmentID: "dev",
	})
	if err != nil {
		t.Fatalf("ComponentLogs returned error: %v", err)
	}

	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d", len(args))
	}

	if args[0] != start.UTC() || args[1] != end.UTC() {
		t.Fatalf("unexpected time args: %#v", args[:2])
	}

	if args[2] != "comp-1" {
		t.Fatalf("expected component arg")
	}

	if args[3] != "dev" {
		t.Fatalf("expected environment arg")
	}

	if args[4] != 50 {
		t.Fatalf("expected limit appended")
	}

	if want := "FROM telemetry.logs_mv"; !strings.Contains(sqlStmt, want) {
		t.Fatalf("query does not contain %q: %s", want, sqlStmt)
	}
	if want := "component_id = ?"; !strings.Contains(sqlStmt, want) {
		t.Fatalf("query missing component filter: %s", sqlStmt)
	}
	if want := "ORDER BY timestamp DESC"; !strings.Contains(sqlStmt, want) {
		t.Fatalf("query missing sort direction: %s", sqlStmt)
	}
}

func TestComponentLogsMissingComponent(t *testing.T) {
	cfg := config.ClickStackConfig{
		LogsTable:   "telemetry.logs_mv",
		TracesTable: "telemetry.traces_mv",
	}
	builder := NewQueryBuilder(cfg)
	_, _, err := builder.ComponentLogs(query.ComponentLogQuery{
		BaseLogQuery: query.BaseLogQuery{
			TimeRange: query.TimeRange{
				Start: time.Now().Add(-time.Minute),
				End:   time.Now(),
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for missing component ID")
	}
}
