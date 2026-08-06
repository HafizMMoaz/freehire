package sources

import (
	"slices"
	"testing"
)

func TestAijobsProvider(t *testing.T) {
	if got := NewAijobs(nil).Provider(); got != "aijobs" {
		t.Errorf("Provider() = %q, want %q", got, "aijobs")
	}
}

func TestAijobsIsBoardlessAggregator(t *testing.T) {
	s := NewAijobs(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("aijobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("aijobs should implement the aggregator marker")
	}
}

func TestAijobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["aijobs"]; !ok {
		t.Error("All() should register provider aijobs")
	}
	if !slices.Contains(FilterableProviders(), "aijobs") {
		t.Error("FilterableProviders() should include aijobs")
	}
}

func TestAijobsBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/aijobs.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/aijobs.yml fails validation: %v", err)
	}
}
