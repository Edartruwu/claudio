package teams

import (
	"testing"
)

func TestCreateTeam_WithModel(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")

	team, err := mgr.CreateTeam("my-team", "desc", "sess-1", "haiku")
	if err != nil {
		t.Fatal(err)
	}

	if team.Model != "haiku" {
		t.Errorf("expected model %q, got %q", "haiku", team.Model)
	}
	if team.Name != "my-team" {
		t.Errorf("expected name %q, got %q", "my-team", team.Name)
	}
}

func TestCreateTeam_WithoutModel(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")

	team, err := mgr.CreateTeam("no-model", "desc", "sess-1", "")
	if err != nil {
		t.Fatal(err)
	}

	if team.Model != "" {
		t.Errorf("expected empty model, got %q", team.Model)
	}
}

func TestCreateTeam_ModelPersistsInGetTeam(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")

	_, err := mgr.CreateTeam("persist-team", "desc", "sess-1", "deepseek-r1-70b")
	if err != nil {
		t.Fatal(err)
	}

	team, ok := mgr.GetTeam("persist-team")
	if !ok {
		t.Fatal("team not found")
	}

	if team.Model != "deepseek-r1-70b" {
		t.Errorf("expected model %q after GetTeam, got %q", "deepseek-r1-70b", team.Model)
	}
}

func TestCreateTeam_ModelInListTeams(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")

	mgr.CreateTeam("team-a", "desc", "s1", "opus")
	mgr.CreateTeam("team-b", "desc", "s1", "haiku")

	teams := mgr.ListTeams()
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}

	models := map[string]string{}
	for _, team := range teams {
		models[team.Name] = team.Model
	}

	if models["team-a"] != "opus" {
		t.Errorf("team-a: expected model %q, got %q", "opus", models["team-a"])
	}
	if models["team-b"] != "haiku" {
		t.Errorf("team-b: expected model %q, got %q", "haiku", models["team-b"])
	}
}

func TestCreateTeam_Duplicate(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, "")

	_, err := mgr.CreateTeam("dup-team", "desc", "s1", "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.CreateTeam("dup-team", "desc", "s1", "")
	if err == nil {
		t.Error("expected error creating duplicate team")
	}
}
