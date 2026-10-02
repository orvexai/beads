package main

import (
	"encoding/json"
	"testing"
)

func TestVersionChangesStructure(t *testing.T) {
	// Verify versionChanges is properly structured
	if len(versionChanges) == 0 {
		t.Fatal("versionChanges should not be empty")
	}

	for i, vc := range versionChanges {
		if vc.Version == "" {
			t.Errorf("versionChanges[%d] has empty Version", i)
		}
		if vc.Date == "" {
			t.Errorf("versionChanges[%d] has empty Date", i)
		}
		if len(vc.Changes) == 0 {
			t.Errorf("versionChanges[%d] has no changes", i)
		}

		// Verify version format (should be like "0.22.1")
		if len(vc.Version) < 5 {
			t.Errorf("versionChanges[%d] has invalid Version format: %s", i, vc.Version)
		}

		// Verify date format (should be like "2025-11-06")
		if len(vc.Date) != 10 {
			t.Errorf("versionChanges[%d] has invalid Date format: %s", i, vc.Date)
		}

		// Verify each change is non-empty
		for j, change := range vc.Changes {
			if change == "" {
				t.Errorf("versionChanges[%d].Changes[%d] is empty", i, j)
			}
		}
	}
}

func TestVersionChangesJSON(t *testing.T) {
	// Test that versionChanges can be marshaled to JSON
	data, err := json.Marshal(versionChanges)
	if err != nil {
		t.Fatalf("Failed to marshal versionChanges to JSON: %v", err)
	}

	// Test that it can be unmarshaled back
	var unmarshaled []VersionChange
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal versionChanges from JSON: %v", err)
	}

	// Verify structure is preserved
	if len(unmarshaled) != len(versionChanges) {
		t.Errorf("Unmarshaled length %d != original length %d", len(unmarshaled), len(versionChanges))
	}

	// Spot check first entry
	if len(unmarshaled) > 0 && len(versionChanges) > 0 {
		if unmarshaled[0].Version != versionChanges[0].Version {
			t.Errorf("Version mismatch: %s != %s", unmarshaled[0].Version, versionChanges[0].Version)
		}
		if len(unmarshaled[0].Changes) != len(versionChanges[0].Changes) {
			t.Errorf("Changes count mismatch: %d != %d", len(unmarshaled[0].Changes), len(versionChanges[0].Changes))
		}
	}
}

func TestVersionChangesCoverage(t *testing.T) {
	// Ensure we have at least 3 recent versions documented
	if len(versionChanges) < 3 {
		t.Errorf("Should document at least 3 recent versions, found %d", len(versionChanges))
	}

	// Ensure each version has at least one change documented
	for i, vc := range versionChanges {
		if len(vc.Changes) < 1 {
			t.Errorf("versionChanges[%d] (v%s) should have at least 1 change, found %d", i, vc.Version, len(vc.Changes))
		}
	}
}

func TestMigrationVersionReportShowsClientSchemaCeiling(t *testing.T) {
	got := migrationVersionReport(t.Context(), nil)
	client, ok := got["client"].(map[string]interface{})
	if !ok {
		t.Fatalf("client migration report = %T, want map", got["client"])
	}
	if client["bd_version"] != Version {
		t.Errorf("client bd_version = %v, want %s", client["bd_version"], Version)
	}
	if client["main"] != 69 {
		t.Errorf("client main migration = %v, want 69", client["main"])
	}
	if client["clone_local"] != 27 {
		t.Errorf("client clone-local migration = %v, want 27", client["clone_local"])
	}
	if got["database"] != nil {
		t.Errorf("database migration report without a store = %v, want nil", got["database"])
	}

	infoSchema := buildInfoSchema("1.3.1", "bd", nil, got)
	if infoSchema["workspace_version"] != "1.3.1" || infoSchema["schema_version"] != "1.3.1" {
		t.Errorf("workspace version fields = (%v, %v), want compatibility-preserving 1.3.1 values",
			infoSchema["workspace_version"], infoSchema["schema_version"])
	}
}
