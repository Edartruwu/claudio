package skills

import "testing"

// ---------------------------------------------------------------------------
// bundledSkills — new design skills
// ---------------------------------------------------------------------------

func TestBundledSkills_ContainsDesignSystem(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'design-system'")
}

func TestBundledSkills_ContainsMockup(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'mockup'")
}

func TestBundledSkills_ContainsHandoff(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			return
		}
	}
	t.Error("bundledSkills() should contain skill named 'handoff'")
}

func TestBundledSkills_DesignSystemContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			if s.Content == "" {
				t.Error("design-system skill Content should not be empty")
			}
			return
		}
	}
	t.Error("design-system skill not found")
}

func TestBundledSkills_MockupContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			if s.Content == "" {
				t.Error("mockup skill Content should not be empty")
			}
			return
		}
	}
	t.Error("mockup skill not found")
}

func TestBundledSkills_HandoffContentNonEmpty(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			if s.Content == "" {
				t.Error("handoff skill Content should not be empty")
			}
			return
		}
	}
	t.Error("handoff skill not found")
}

func TestBundledSkills_DesignSystemSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "design-system" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

func TestBundledSkills_MockupSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "mockup" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

func TestBundledSkills_HandoffSource(t *testing.T) {
	skills := bundledSkills()
	for _, s := range skills {
		if s.Name == "handoff" {
			if s.Source != "bundled" {
				t.Errorf("expected Source=%q, got %q", "bundled", s.Source)
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// LoadAll — design skills reachable via registry
// ---------------------------------------------------------------------------

func TestLoadAll_DesignSkillsInRegistry(t *testing.T) {
	r := LoadAll("", "")
	for _, name := range []string{"design-system", "mockup", "handoff"} {
		s, ok := r.Get(name)
		if !ok {
			t.Errorf("expected skill %q in registry", name)
			continue
		}
		if s.Content == "" {
			t.Errorf("skill %q should have non-empty Content", name)
		}
	}
}
