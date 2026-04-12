package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Abraxas-365/claudio-plugin-caido/cmd"
)

var version = "dev"

const (
	description = "Caido proxy control — query HTTP history, replay requests, manage findings, intercept traffic"

	schema = `{"type":"object","required":["args"],"properties":{"args":{"type":"string","description":"Subcommand and flags. Examples: 'history -f req.host.eq:\"target.com\"', 'replay --host target.com', 'finding-create --request-id abc --title IDOR'"},"input":{"type":"string","description":"Raw HTTP request bytes for replay/intercept-forward commands"}}}`

	instructions = `## Caido Pentest Plugin

You have access to a live Caido proxy instance. Use it to analyze captured traffic and actively test for vulnerabilities.

### Pentest loop
1. ` + "`history -f <HTTPQL>`" + ` to find interesting endpoints
2. ` + "`request <id>`" + ` to inspect full request/response
3. ` + "`replay`" + ` with crafted payload via stdin to test
4. Analyze response — status code, body, headers, timing
5. ` + "`finding-create`" + ` when vulnerability confirmed

### HTTPQL quick reference
- ` + "`req.host.eq:\"target.com\"`" + ` — filter by host
- ` + "`req.method.eq:\"POST\"`" + ` — filter by method
- ` + "`res.status.eq:500`" + ` — filter by response code
- ` + "`req.path.cont:\"/api/admin\"`" + ` — path contains string

### Vulnerability patterns to test
- **IDOR**: Replay request, swap ` + "`id`" + ` param to another user's ID
- **SQLi**: Inject ` + "`'`" + `, ` + "`1'--`" + `, ` + "`1 OR 1=1`" + ` into string params
- **Path traversal**: Try ` + "`../../etc/passwd`" + ` in file path params
- **Auth bypass**: Replay with missing/empty Authorization header
- **Mass assignment**: Add extra fields like ` + "`{\"role\":\"admin\"}`" + ` to POST bodies
- **JWT alg:none**: Replace JWT, set alg to none, strip signature

### Security
Never store credentials found in traffic. Report with ` + "`finding-create`" + `.
`
)

func main() {
	// Parse top-level flags before cobra
	describeFlag := flag.Bool("describe", false, "")
	schemaFlag := flag.Bool("schema", false, "")
	instructionsFlag := flag.Bool("instructions", false, "")

	// Parse only top-level flags, allow remainder
	flag.CommandLine.Parse(os.Args[1:])

	if *describeFlag {
		fmt.Println(description)
		os.Exit(0)
	}

	if *schemaFlag {
		fmt.Print(schema)
		os.Exit(0)
	}

	if *instructionsFlag {
		fmt.Print(instructions)
		os.Exit(0)
	}

	// Execute cobra root command
	// Reconstruct argv from remaining args + potential args input
	argv := []string{os.Args[0]}

	// Check for --args flag in remaining args
	remaining := flag.Args()
	var argsValue string
	var inputValue string

	// Simple parsing: look for --args and --input
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--args" && i+1 < len(remaining) {
			argsValue = remaining[i+1]
			i++ // Skip the value
		} else if remaining[i] == "--input" && i+1 < len(remaining) {
			inputValue = remaining[i+1]
			i++ // Skip the value
		} else {
			// Pass through other flags/args
			argv = append(argv, remaining[i])
		}
	}

	// If args provided, split and append to argv
	if argsValue != "" {
		// Shell-like split
		parts := strings.Fields(argsValue)
		argv = append(argv, parts...)
	}

	// If input provided, write to stdin temp mechanism
	// For now, it's read from os.Stdin directly in commands
	_ = inputValue

	// Set os.Args to reconstructed argv
	os.Args = argv

	// Execute root command
	if err := cmd.Root.Execute(); err != nil {
		os.Exit(1)
	}
}
