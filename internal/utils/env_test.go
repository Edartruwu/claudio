package utils

import (
	"os"
	"testing"
)

func TestGetEnv_Set(t *testing.T) {
	key := "TEST_GETENV_SET"
	t.Setenv(key, "hello")
	if got := GetEnv(key, "default"); got != "hello" {
		t.Errorf("GetEnv = %q, want %q", got, "hello")
	}
}

func TestGetEnv_Unset(t *testing.T) {
	key := "TEST_GETENV_UNSET_XYZ"
	os.Unsetenv(key)
	if got := GetEnv(key, "fallback"); got != "fallback" {
		t.Errorf("GetEnv = %q, want %q", got, "fallback")
	}
}

func TestGetEnv_Empty(t *testing.T) {
	key := "TEST_GETENV_EMPTY"
	t.Setenv(key, "")
	// empty string should use default
	if got := GetEnv(key, "default"); got != "default" {
		t.Errorf("GetEnv with empty value = %q, want %q", got, "default")
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"random", false},
	}

	key := "TEST_GETENVBOOL"
	for _, tc := range tests {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv(key, tc.val)
			if got := GetEnvBool(key, false); got != tc.want {
				t.Errorf("GetEnvBool(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestGetEnvBool_Default(t *testing.T) {
	key := "TEST_GETENVBOOL_DEFAULT_XYZ"
	os.Unsetenv(key)
	if got := GetEnvBool(key, true); got != true {
		t.Error("GetEnvBool unset: expected default true")
	}
	if got := GetEnvBool(key, false); got != false {
		t.Error("GetEnvBool unset: expected default false")
	}
}

func TestGetEnvInt(t *testing.T) {
	key := "TEST_GETENVINT"

	t.Run("valid int", func(t *testing.T) {
		t.Setenv(key, "42")
		if got := GetEnvInt(key, 0); got != 42 {
			t.Errorf("GetEnvInt = %d, want 42", got)
		}
	})

	t.Run("negative int", func(t *testing.T) {
		t.Setenv(key, "-7")
		if got := GetEnvInt(key, 0); got != -7 {
			t.Errorf("GetEnvInt = %d, want -7", got)
		}
	})

	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv(key, "notanumber")
		if got := GetEnvInt(key, 99); got != 99 {
			t.Errorf("GetEnvInt invalid = %d, want 99", got)
		}
	})

	t.Run("unset uses default", func(t *testing.T) {
		os.Unsetenv(key)
		if got := GetEnvInt(key, 5); got != 5 {
			t.Errorf("GetEnvInt unset = %d, want 5", got)
		}
	})
}

func TestGetEnvFloat(t *testing.T) {
	key := "TEST_GETENVFLOAT"

	t.Run("valid float", func(t *testing.T) {
		t.Setenv(key, "3.14")
		if got := GetEnvFloat(key, 0.0); got != 3.14 {
			t.Errorf("GetEnvFloat = %f, want 3.14", got)
		}
	})

	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv(key, "nan_value")
		if got := GetEnvFloat(key, 1.5); got != 1.5 {
			t.Errorf("GetEnvFloat invalid = %f, want 1.5", got)
		}
	})

	t.Run("unset uses default", func(t *testing.T) {
		os.Unsetenv(key)
		if got := GetEnvFloat(key, 2.0); got != 2.0 {
			t.Errorf("GetEnvFloat unset = %f, want 2.0", got)
		}
	})
}

func TestRequireEnv_Set(t *testing.T) {
	key := "TEST_REQUIREENV_SET"
	t.Setenv(key, "myvalue")
	if got := RequireEnv(key); got != "myvalue" {
		t.Errorf("RequireEnv = %q, want %q", got, "myvalue")
	}
}

func TestRequireEnv_Unset(t *testing.T) {
	key := "TEST_REQUIREENV_UNSET_XYZ"
	os.Unsetenv(key)
	defer func() {
		if r := recover(); r == nil {
			t.Error("RequireEnv with unset var: expected panic, got none")
		}
	}()
	RequireEnv(key)
}

func TestValidateEnv(t *testing.T) {
	presentKey := "TEST_VALIDATEENV_PRESENT"
	missingKey := "TEST_VALIDATEENV_MISSING_XYZ"

	t.Setenv(presentKey, "set")
	os.Unsetenv(missingKey)

	t.Run("all present", func(t *testing.T) {
		missing := ValidateEnv(presentKey)
		if len(missing) != 0 {
			t.Errorf("expected no missing vars, got %v", missing)
		}
	})

	t.Run("one missing", func(t *testing.T) {
		missing := ValidateEnv(presentKey, missingKey)
		if len(missing) != 1 || missing[0] != missingKey {
			t.Errorf("expected [%s], got %v", missingKey, missing)
		}
	})

	t.Run("all missing", func(t *testing.T) {
		missing := ValidateEnv(missingKey)
		if len(missing) != 1 {
			t.Errorf("expected 1 missing, got %v", missing)
		}
	})

	t.Run("no args", func(t *testing.T) {
		missing := ValidateEnv()
		if len(missing) != 0 {
			t.Errorf("expected no missing with no args, got %v", missing)
		}
	})
}

func TestExpandEnvVars(t *testing.T) {
	key := "TEST_EXPANDENVVARS"
	t.Setenv(key, "world")

	got := ExpandEnvVars("hello $" + key)
	if got != "hello world" {
		t.Errorf("ExpandEnvVars = %q, want %q", got, "hello world")
	}

	got = ExpandEnvVars("hello ${" + key + "}")
	if got != "hello world" {
		t.Errorf("ExpandEnvVars (braces) = %q, want %q", got, "hello world")
	}

	// No variables — unchanged
	got = ExpandEnvVars("no vars here")
	if got != "no vars here" {
		t.Errorf("ExpandEnvVars no vars = %q, want %q", got, "no vars here")
	}
}

func TestClaudioEnvVars(t *testing.T) {
	t.Setenv("CLAUDIO_TEST_FOO", "bar")
	t.Setenv("CLAUDIO_TEST_BAZ", "qux")
	t.Setenv("OTHER_VAR", "ignored")

	got := ClaudioEnvVars()
	if got["CLAUDIO_TEST_FOO"] != "bar" {
		t.Errorf("CLAUDIO_TEST_FOO = %q, want %q", got["CLAUDIO_TEST_FOO"], "bar")
	}
	if got["CLAUDIO_TEST_BAZ"] != "qux" {
		t.Errorf("CLAUDIO_TEST_BAZ = %q, want %q", got["CLAUDIO_TEST_BAZ"], "qux")
	}
	if _, ok := got["OTHER_VAR"]; ok {
		t.Error("expected OTHER_VAR to be excluded")
	}
}
