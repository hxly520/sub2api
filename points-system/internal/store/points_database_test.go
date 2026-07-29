package store

import "testing"

func TestPointsPoolConfigForcesIsolatedSchemaAndConnectionLimit(t *testing.T) {
	cfg, err := pointsPoolConfig(
		"postgres://points:secret@localhost/sub2api?sslmode=disable&pool_min_conns=10&pool_max_conns=20",
		"points", 8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ConnConfig.RuntimeParams["search_path"]; got != `"points",public` {
		t.Fatalf("search_path = %q", got)
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "sub2api-points" {
		t.Fatalf("application_name = %q", got)
	}
	if cfg.MinConns != 0 || cfg.MaxConns != 8 {
		t.Fatalf("pool bounds = %d/%d, want 0/8", cfg.MinConns, cfg.MaxConns)
	}
}

func TestPointsPoolConfigRejectsPublicSchemaAndUnsafeLimits(t *testing.T) {
	for _, test := range []struct {
		schema   string
		maxConns int
	}{
		{schema: "", maxConns: 8},
		{schema: "public", maxConns: 8},
		{schema: "points", maxConns: 0},
		{schema: "points", maxConns: 33},
	} {
		if _, err := pointsPoolConfig("postgres://localhost/sub2api", test.schema, test.maxConns); err == nil {
			t.Fatalf("schema=%q maxConns=%d was accepted", test.schema, test.maxConns)
		}
	}
}
