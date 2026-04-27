package tools

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// huashuDesignDir returns the path to the huashu-design directory.
// Checks HUASHU_DESIGN_DIR env var first, then falls back to ~/Personal/huashu-design.
func huashuDesignDir() string {
	if dir := os.Getenv("HUASHU_DESIGN_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Personal", "huashu-design")
}

// checkNodeAvailable verifies that node is on PATH.
func checkNodeAvailable() error {
	if err := exec.Command("node", "--version").Run(); err != nil {
		return fmt.Errorf(
			"Node.js not found.\n" +
				"Install Node.js >= 18: https://nodejs.org")
	}
	return nil
}

// checkFfmpegAvailable verifies that ffmpeg is on PATH.
func checkFfmpegAvailable() error {
	if err := exec.Command("ffmpeg", "-version").Run(); err != nil {
		return fmt.Errorf(
			"ffmpeg required for video export. Install via: brew install ffmpeg")
	}
	return nil
}

// checkPython3Available verifies that python3 is on PATH.
func checkPython3Available() error {
	if err := exec.Command("python3", "--version").Run(); err != nil {
		return fmt.Errorf(
			"python3 not found.\n" +
				"Install Python 3: https://www.python.org/downloads/")
	}
	return nil
}

// checkPlaywrightAvailable verifies that the playwright Python package is installed.
func checkPlaywrightAvailable() error {
	if err := exec.Command("python3", "-c", "import playwright").Run(); err != nil {
		return fmt.Errorf(
			"Playwright required. Install via: pip install playwright && playwright install chromium")
	}
	return nil
}

// nodeGlobalModulesDir returns the global node_modules path so scripts can
// find globally-installed packages (e.g. playwright). Uses `npm root -g`.
func nodeGlobalModulesDir() string {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		log.Printf("[tools] could not determine Node.js global modules dir (npm not found or failed); module resolution may fail: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}
