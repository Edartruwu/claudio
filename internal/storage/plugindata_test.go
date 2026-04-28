package storage

import "testing"

func TestGetPluginData_NotFound(t *testing.T) {
	db := openTestDB(t)
	val, err := db.GetPluginData("my-plugin", "missing-key")
	if err != nil {
		t.Fatalf("GetPluginData: %v", err)
	}
	if val != "" {
		t.Errorf("want empty string, got %q", val)
	}
}

func TestSetPluginData_Upsert(t *testing.T) {
	db := openTestDB(t)

	// Insert
	if err := db.SetPluginData("my-plugin", "color", "blue"); err != nil {
		t.Fatalf("SetPluginData insert: %v", err)
	}
	val, err := db.GetPluginData("my-plugin", "color")
	if err != nil {
		t.Fatalf("GetPluginData after insert: %v", err)
	}
	if val != "blue" {
		t.Errorf("want %q, got %q", "blue", val)
	}

	// Upsert (update)
	if err := db.SetPluginData("my-plugin", "color", "red"); err != nil {
		t.Fatalf("SetPluginData upsert: %v", err)
	}
	val, err = db.GetPluginData("my-plugin", "color")
	if err != nil {
		t.Fatalf("GetPluginData after upsert: %v", err)
	}
	if val != "red" {
		t.Errorf("want %q after upsert, got %q", "red", val)
	}
}

func TestDeletePluginData(t *testing.T) {
	db := openTestDB(t)

	if err := db.SetPluginData("my-plugin", "temp", "value"); err != nil {
		t.Fatalf("SetPluginData: %v", err)
	}
	if err := db.DeletePluginData("my-plugin", "temp"); err != nil {
		t.Fatalf("DeletePluginData: %v", err)
	}
	val, err := db.GetPluginData("my-plugin", "temp")
	if err != nil {
		t.Fatalf("GetPluginData after delete: %v", err)
	}
	if val != "" {
		t.Errorf("want empty after delete, got %q", val)
	}
}

func TestListPluginData(t *testing.T) {
	db := openTestDB(t)

	// Empty list for unknown plugin
	result, err := db.ListPluginData("unknown-plugin")
	if err != nil {
		t.Fatalf("ListPluginData empty: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("want empty map, got %v", result)
	}

	// Populate and list
	entries := map[string]string{
		"alpha": "1",
		"beta":  "2",
		"gamma": "3",
	}
	for k, v := range entries {
		if err := db.SetPluginData("list-plugin", k, v); err != nil {
			t.Fatalf("SetPluginData %q: %v", k, err)
		}
	}

	result, err = db.ListPluginData("list-plugin")
	if err != nil {
		t.Fatalf("ListPluginData: %v", err)
	}
	if len(result) != len(entries) {
		t.Errorf("want %d entries, got %d", len(entries), len(result))
	}
	for k, want := range entries {
		if got := result[k]; got != want {
			t.Errorf("key %q: want %q, got %q", k, want, got)
		}
	}

	// Keys from other plugins not included
	if err := db.SetPluginData("other-plugin", "x", "y"); err != nil {
		t.Fatalf("SetPluginData other-plugin: %v", err)
	}
	result, err = db.ListPluginData("list-plugin")
	if err != nil {
		t.Fatalf("ListPluginData after other-plugin insert: %v", err)
	}
	if len(result) != len(entries) {
		t.Errorf("cross-plugin leak: want %d entries, got %d", len(entries), len(result))
	}
}
