package tools

import "testing"

func TestIsCatFileCommand_SedLineRange(t *testing.T) {
	cases := []string{
		"sed -n '10,20p' somefile.go",
		"sed -n '1,50p' /abs/path/to/file",
	}
	for _, cmd := range cases {
		if !isCatFileCommand(cmd) {
			t.Errorf("expected isCatFileCommand(%q) = true, got false", cmd)
		}
	}
}

func TestIsCatFileCommand_SedInPipeline(t *testing.T) {
	cases := []string{
		"cat file | sed -n '10,20p'",
		"grep foo file | sed -n '1,5p'",
	}
	for _, cmd := range cases {
		if isCatFileCommand(cmd) {
			t.Errorf("expected isCatFileCommand(%q) = false, got true", cmd)
		}
	}
}
