package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/utils"
)

// ProjectInfo holds detected project characteristics.
type ProjectInfo struct {
	Languages   []string
	Frameworks  []string
	BuildSystem string
	TestCommand string
	HasCI       bool
	HasDocker   bool
	Description string
}

// DetectProject scans a directory for project characteristics.
func DetectProject(dir string) *ProjectInfo {
	info := &ProjectInfo{}

	// Language detection
	if fileExists(filepath.Join(dir, "go.mod")) {
		info.Languages = append(info.Languages, "Go")
		info.BuildSystem = "go build"
		info.TestCommand = "go test ./..."
	}
	if fileExists(filepath.Join(dir, "package.json")) {
		info.Languages = append(info.Languages, "JavaScript/TypeScript")
		if fileExists(filepath.Join(dir, "bun.lock")) {
			info.BuildSystem = "bun"
		} else if fileExists(filepath.Join(dir, "yarn.lock")) {
			info.BuildSystem = "yarn"
		} else {
			info.BuildSystem = "npm"
		}
		info.TestCommand = info.BuildSystem + " test"
	}
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		info.Languages = append(info.Languages, "Rust")
		info.BuildSystem = "cargo"
		info.TestCommand = "cargo test"
	}
	if fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "setup.py")) {
		info.Languages = append(info.Languages, "Python")
		if fileExists(filepath.Join(dir, "pyproject.toml")) {
			info.BuildSystem = "uv/pip"
		} else {
			info.BuildSystem = "pip"
		}
		info.TestCommand = "pytest"
	}
	if fileExists(filepath.Join(dir, "pom.xml")) || fileExists(filepath.Join(dir, "build.gradle")) {
		info.Languages = append(info.Languages, "Java")
		if fileExists(filepath.Join(dir, "pom.xml")) {
			info.BuildSystem = "maven"
			info.TestCommand = "mvn test"
		} else {
			info.BuildSystem = "gradle"
			info.TestCommand = "gradle test"
		}
	}

	// Framework detection
	if fileExists(filepath.Join(dir, "package.json")) {
		pkgContent := utils.ReadFileIfExists(filepath.Join(dir, "package.json"))
		if strings.Contains(pkgContent, "\"react\"") {
			info.Frameworks = append(info.Frameworks, "React")
		}
		if strings.Contains(pkgContent, "\"vue\"") {
			info.Frameworks = append(info.Frameworks, "Vue")
		}
		if strings.Contains(pkgContent, "\"next\"") {
			info.Frameworks = append(info.Frameworks, "Next.js")
		}
		if strings.Contains(pkgContent, "\"express\"") {
			info.Frameworks = append(info.Frameworks, "Express")
		}
	}

	// CI detection
	if fileExists(filepath.Join(dir, ".github", "workflows")) ||
		fileExists(filepath.Join(dir, ".gitlab-ci.yml")) ||
		fileExists(filepath.Join(dir, ".circleci")) {
		info.HasCI = true
	}

	// Docker detection
	if fileExists(filepath.Join(dir, "Dockerfile")) ||
		fileExists(filepath.Join(dir, "docker-compose.yml")) ||
		fileExists(filepath.Join(dir, "docker-compose.yaml")) {
		info.HasDocker = true
	}

	// Project description from README
	for _, name := range []string{"README.md", "readme.md", "README.rst", "README"} {
		if content := utils.ReadFileIfExists(filepath.Join(dir, name)); content != "" {
			// Extract first paragraph as description
			lines := strings.SplitN(content, "\n\n", 2)
			if len(lines) > 0 {
				desc := strings.TrimSpace(lines[0])
				// Remove markdown header prefix
				desc = strings.TrimLeft(desc, "# ")
				if len(desc) > 200 {
					desc = desc[:200] + "..."
				}
				info.Description = desc
			}
			break
		}
	}

	// Fallback language detection from Makefile
	if info.BuildSystem == "" && fileExists(filepath.Join(dir, "Makefile")) {
		info.BuildSystem = "make"
	}

	return info
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
