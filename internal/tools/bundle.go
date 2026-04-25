package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)


// BundleMockupTool reads an HTML entry file, inlines local <script src="...">
// references, optionally fetches and embeds CDN script tags, then writes a
// single self-contained HTML file. Pure Go — no CGO, no exec.Command.
type BundleMockupTool struct {
	designsDir string
	pusher     ScreenshotPusher // optional: push bundle link to CC chat after writing
	sessionID  string
	publicURL  string // optional: base URL prefix for bundle links (e.g. https://host.ts.net)
}

// WithPublicURL sets a base URL prefix for bundle links pushed to CC chat.
// If set, bundleURL becomes absolute: publicURL + "/designs/project/...".
// Returns receiver for fluent chaining.
func (t *BundleMockupTool) WithPublicURL(url string) *BundleMockupTool {
	t.publicURL = url
	return t
}

// WithPusher wires a ScreenshotPusher so BundleMockup pushes a clickable link
// to CC chat after the bundle file is written. sessionID is forwarded as context.
// Returns receiver for fluent chaining.
func (t *BundleMockupTool) WithPusher(pusher ScreenshotPusher, sessionID string) *BundleMockupTool {
	t.pusher = pusher
	t.sessionID = sessionID
	return t
}

// NewBundleMockupTool creates a BundleMockupTool that defaults output under designsDir.
func NewBundleMockupTool(designsDir string) *BundleMockupTool {
	return &BundleMockupTool{designsDir: designsDir}
}

// BundleMockupInput is the JSON input schema for this tool.
type BundleMockupInput struct {
	EntryHTML  string            `json:"entry_html"`
	OutputPath string            `json:"output_path"`
	SessionDir string            `json:"session_dir"` // optional: reuse existing session dir instead of creating new timestamp
	Files      map[string]string `json:"files"`
	EmbedCDN   *bool             `json:"embed_cdn"` // pointer so we can detect omission
}

// BundleMockupOutput is the JSON result returned by this tool.
type BundleMockupOutput struct {
	OutputPath     string   `json:"output_path"`
	SizeBytes      int64    `json:"size_bytes"`
	EmbeddedDeps   []string `json:"embedded_deps"`
	OfflineCapable bool     `json:"offline_capable"`
}

func (t *BundleMockupTool) Name() string { return "BundleMockup" }

func (t *BundleMockupTool) Description() string {
	return `Bundle an HTML mockup into a single self-contained file.

Reads an HTML entry file, inlines all local <script src="..."> references
(JSX, JS, etc.), optionally fetches CDN script tags (React, ReactDOM, Babel)
via HTTP and embeds them inline, then writes the result to output_path.
Returns the output path, file size, list of embedded CDN deps, and whether
the result is offline-capable.

Pure Go — works without Node.js, Playwright, or any external dependencies.`
}

func (t *BundleMockupTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"entry_html": {
				"type": "string",
				"description": "Absolute or relative path to the HTML entry file."
			},
			"output_path": {
				"type": "string",
				"description": "Exact output file path. Takes precedence over session_dir."
			},
			"session_dir": {
				"type": "string",
				"description": "Session directory to write bundle into ({session_dir}/bundle/mockup.html). Pass the same session_dir used for RenderMockup to keep all outputs together. Defaults to a new {designsDir}/{timestamp} dir."
			},
			"files": {
				"type": "object",
				"description": "Optional explicit file map: {\"tokens.jsx\": \"/path/to/tokens.jsx\", ...}. Overrides automatic resolution.",
				"additionalProperties": {"type": "string"}
			},
			"embed_cdn": {
				"type": "boolean",
				"description": "Fetch CDN <script> URLs and embed inline. Default: true."
			}
		},
		"required": ["entry_html"]
	}`)
}

func (t *BundleMockupTool) IsReadOnly() bool { return false }

func (t *BundleMockupTool) RequiresApproval(_ json.RawMessage) bool { return false }

// localScriptRe matches any <script src="...">; local vs CDN distinguished in code.
var localScriptRe = regexp.MustCompile(`(?i)<script([^>]*)\bsrc="([^"]+)"([^>]*)>(\s*</script>)?`)

// cdnScriptRe matches <script src="https://..."> or <script src="http://...">.
var cdnScriptRe = regexp.MustCompile(`(?i)<script([^>]*)\bsrc="(https?://[^"]+)"([^>]*)>(\s*</script>)?`)

func (t *BundleMockupTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var in BundleMockupInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}

	if in.EntryHTML == "" {
		return &Result{Content: "entry_html is required", IsError: true}, nil
	}

	// Default embed_cdn = true when omitted.
	embedCDN := true
	if in.EmbedCDN != nil {
		embedCDN = *in.EmbedCDN
	}

	// Read entry HTML.
	htmlBytes, err := os.ReadFile(in.EntryHTML)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to read entry_html %q: %v", in.EntryHTML, err), IsError: true}, nil
	}
	html := string(htmlBytes)
	htmlDir := filepath.Dir(in.EntryHTML)

	var warnings []string

	// --- 1. Inline local <script src="..."> tags ---
	html = localScriptRe.ReplaceAllStringFunc(html, func(match string) string {
		groups := localScriptRe.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		srcRef := groups[2] // e.g. "./tokens.jsx" or "tokens.jsx"
		// Skip CDN URLs — handled by cdnScriptRe below.
		if strings.HasPrefix(srcRef, "http://") || strings.HasPrefix(srcRef, "https://") {
			return match
		}
		srcName := filepath.Base(srcRef)

		// Determine actual file path: explicit map takes priority.
		var srcPath string
		if in.Files != nil {
			if mapped, ok := in.Files[srcName]; ok {
				srcPath = mapped
			} else if mapped, ok := in.Files[srcRef]; ok {
				srcPath = mapped
			}
		}
		if srcPath == "" {
			// Resolve relative to the HTML file's directory.
			srcPath = filepath.Join(htmlDir, filepath.FromSlash(srcRef))
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read local script %q (%s): %v", srcRef, srcPath, err))
			return match // leave original tag unchanged
		}

		// Preserve any other attributes (e.g. type="text/babel") but strip src.
		attrs := strings.TrimSpace(groups[1] + " " + groups[3])
		attrs = removeSrcAttr(attrs)
		if attrs != "" {
			return fmt.Sprintf("<script %s>\n%s\n</script>", strings.TrimSpace(attrs), string(content))
		}
		return fmt.Sprintf("<script>\n%s\n</script>", string(content))
	})

	// --- 2. Optionally embed CDN <script src="https://..."> tags ---
	var embeddedDeps []string
	remainingCDN := 0

	if embedCDN {
		client := &http.Client{Timeout: 30 * time.Second}

		html = cdnScriptRe.ReplaceAllStringFunc(html, func(match string) string {
			groups := cdnScriptRe.FindStringSubmatch(match)
			if groups == nil {
				return match
			}
			url := groups[2]

			resp, err := client.Get(url) //nolint:noctx
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("CDN fetch failed for %q: %v — leaving as-is", url, err))
				remainingCDN++
				return match
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				warnings = append(warnings, fmt.Sprintf("CDN fetch %q returned HTTP %d — leaving as-is", url, resp.StatusCode))
				remainingCDN++
				return match
			}

			body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB cap
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("CDN read failed for %q: %v — leaving as-is", url, err))
				remainingCDN++
				return match
			}

			dep := extractDepName(url)
			if dep != "" {
				embeddedDeps = append(embeddedDeps, dep)
			}

			attrs := strings.TrimSpace(groups[1] + " " + groups[3])
			attrs = removeSrcAttr(attrs)
			if attrs != "" {
				return fmt.Sprintf("<script %s>\n%s\n</script>", strings.TrimSpace(attrs), string(body))
			}
			return fmt.Sprintf("<script>\n%s\n</script>", string(body))
		})
	}

	// --- 3. Resolve output path ---
	outPath := in.OutputPath
	if outPath == "" {
		sessionDir := in.SessionDir
		if sessionDir == "" {
			sessionDir = filepath.Join(t.designsDir, "session")
		}
		outPath = filepath.Join(sessionDir, "bundle", "mockup.html")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to create output dir: %v", err), IsError: true}, nil
	}

	// Inject infinite canvas shell (pan + zoom) around the artboard content.
	html = InjectInfiniteCanvas(html)

	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return &Result{Content: fmt.Sprintf("Failed to write output: %v", err), IsError: true}, nil
	}

	info, err := os.Stat(outPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Failed to stat output: %v", err), IsError: true}, nil
	}

	offlineCapable := embedCDN && remainingCDN == 0

	out := BundleMockupOutput{
		OutputPath:     outPath,
		SizeBytes:      info.Size(),
		EmbeddedDeps:   embeddedDeps,
		OfflineCapable: offlineCapable,
	}

	outJSON, _ := json.MarshalIndent(out, "", "  ")

	// Compute the bundle URL. Use t.designsDir (project-scoped) as the anchor so
	// the URL is correct even when outPath lives in a legacy global designs dir.
	//
	// Two cases:
	//   1. outPath is inside t.designsDir (project-scoped):
	//      → /designs/project/{slug}/{session}/bundle/mockup.html
	//   2. outPath is outside t.designsDir (legacy ~/.claudio/designs/ dir):
	//      → /designs/static/{session}/bundle/mockup.html
	var bundleURL, sessionDirName string
	if relPath, err := filepath.Rel(t.designsDir, outPath); err == nil && !strings.HasPrefix(relPath, "..") {
		// Project-scoped: relPath = "{session}/bundle/mockup.html"
		sessionDirName = strings.SplitN(relPath, string(filepath.Separator), 2)[0]
		slug := filepath.Base(filepath.Dir(t.designsDir)) // parent of "designs/" = slug
		bundleURL = "/designs/project/" + slug + "/" + sessionDirName + "/bundle/mockup.html"
	} else {
		// Legacy global designs dir: fall back to /designs/static/ route.
		sessionDir := filepath.Dir(filepath.Dir(outPath)) // .../designs/{session}
		sessionDirName = filepath.Base(sessionDir)
		bundleURL = "/designs/static/" + sessionDirName + "/bundle/mockup.html"
	}
	if t.publicURL != "" {
		bundleURL = strings.TrimRight(t.publicURL, "/") + bundleURL
	}

	if t.pusher != nil {
		// Best-effort — ignore errors, bundle result already returned.
		_ = t.pusher.PushBundleLink(t.sessionID, bundleURL, sessionDirName, outPath)
	}

	_ = outJSON // suppress unused warning
	// Return warnings if any; otherwise empty — the bundle card is pushed via
	// PushBundleLink above. Returning the URL caused the AI to echo
	// [Open bundle](url) markdown in its next response, which rendered as a
	// plain link instead of the card widget.
	var content string
	if len(warnings) > 0 {
		content = "Warnings:\n" + strings.Join(warnings, "\n")
	}
	return &Result{Content: content}, nil
}

// injectInfiniteCanvas wraps the HTML body with a pan/zoom infinite canvas shell.
// It modifies <body> content in-place — no external dependencies, pure vanilla JS/CSS.
// InjectInfiniteCanvas is exported so the web server can inject the canvas
// at serve time, keeping old bundles on disk up-to-date without re-bundling.
func InjectInfiniteCanvas(html string) string {
	canvasCSS := `<style id="cc-canvas-style">
*,*::before,*::after{box-sizing:border-box}
html,body{margin:0;padding:0;width:100%;height:100%;overflow:hidden;background:#0B0E0F}
#cc-canvas-root{position:fixed;inset:0;overflow:hidden;background:#383c3f;background-image:radial-gradient(circle,rgba(255,255,255,0.12) 1px,transparent 1px);background-size:24px 24px;cursor:grab;user-select:none;-webkit-user-select:none}
#cc-canvas-root.cc-grabbing{cursor:grabbing}
#cc-canvas-content{position:absolute;top:0;left:0;transform-origin:0 0;will-change:transform}
#cc-toolbar{position:fixed;bottom:24px;left:50%;transform:translateX(-50%);z-index:99999;display:flex;gap:6px;align-items:center;background:rgba(15,18,19,0.92);backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);border:1px solid rgba(255,255,255,0.1);border-radius:14px;padding:8px 14px;color:#D4DDE0;font-family:'JetBrains Mono',monospace,-apple-system,sans-serif;font-size:13px;font-weight:500;box-shadow:0 8px 32px rgba(0,0,0,0.6);white-space:nowrap}
#cc-toolbar button{background:rgba(255,255,255,0.06);border:1px solid rgba(255,255,255,0.1);color:#D4DDE0;border-radius:8px;padding:5px 12px;font-size:13px;cursor:pointer;transition:background 0.15s;font-family:inherit;font-weight:500;display:inline-flex;align-items:center;gap:5px}
#cc-toolbar button:hover{background:rgba(0,196,140,0.15);border-color:rgba(0,196,140,0.4);color:#00C48C}
#cc-btn-back{background:rgba(0,196,140,0.1)!important;border-color:rgba(0,196,140,0.3)!important;color:#00C48C!important}
#cc-toolbar .cc-zoom-label{min-width:46px;text-align:center;color:#6B7E82;font-size:12px;font-family:inherit}
#cc-toolbar .cc-sep{width:1px;height:18px;background:rgba(255,255,255,0.1);margin:0 2px}
#cc-canvas-content [data-artboard]{border-radius:12px;box-shadow:0 8px 40px rgba(0,0,0,0.7),0 0 0 1px rgba(255,255,255,0.08);overflow:hidden}
#root>div>*:not(:has([data-artboard])):not([data-artboard]){display:none!important}#root div:has([data-artboard])>*:not([data-artboard]):not(:has([data-artboard])){display:none!important}#root>div{background:transparent!important}
</style>`

	canvasJS := `<script id="cc-canvas-js">(function(){
var scale=1,tx=0,ty=0,dragging=false,startX=0,startY=0,startTx=0,startTy=0;
var root=document.getElementById('cc-canvas-root');
var content=document.getElementById('cc-canvas-content');
var label=document.getElementById('cc-zoom-label');
var MIN=0.05,MAX=8;
var laid=false;
function clamp(v,lo,hi){return Math.min(Math.max(v,lo),hi)}
function applyTransform(){
  content.style.transform='translate('+tx+'px,'+ty+'px) scale('+scale+')';
  if(label)label.textContent=Math.round(scale*100)+'%';
}
function layoutHorizontal(artboards){
  if(laid||artboards.length===0)return;
  // Only re-layout if artboards have non-zero size
  if(artboards[0].offsetWidth===0)return;
  laid=true;
  var GAP=80,PAD=80;
  var x=PAD,maxH=0;
  content.style.position='relative';
  for(var i=0;i<artboards.length;i++){
    var el=artboards[i];
    var w=el.offsetWidth,h=el.offsetHeight;
    el.style.position='absolute';
    el.style.left=x+'px';
    el.style.top=PAD+'px';
    el.style.margin='0';
    x+=w+GAP;
    if(h>maxH)maxH=h;
  }
  content.style.width=(x-GAP+PAD)+'px';
  content.style.height=(maxH+PAD*2)+'px';
}
function fitToScreen(){
  var rr=root.getBoundingClientRect();
  var cw=content.offsetWidth,ch=content.offsetHeight;
  if(cw===0||ch===0)return;
  var s=Math.min(rr.width/cw,rr.height/ch)*0.88;
  s=clamp(s,MIN,MAX);
  scale=s;
  tx=(rr.width-cw*scale)/2;
  ty=(rr.height-ch*scale)/2;
  applyTransform();
}
function zoomAt(cx,cy,factor){
  var ns=clamp(scale*factor,MIN,MAX);
  var f=ns/scale;
  tx=cx-(cx-tx)*f;ty=cy-(cy-ty)*f;scale=ns;
  applyTransform();
}
root.addEventListener('wheel',function(e){
  e.preventDefault();
  var factor=e.deltaY<0?1.1:0.909;
  if(e.ctrlKey||e.metaKey)factor=e.deltaY<0?1.25:0.8;
  var rr=root.getBoundingClientRect();
  zoomAt(e.clientX-rr.left,e.clientY-rr.top,factor);
},{passive:false});
root.addEventListener('mousedown',function(e){
  if(e.button!==0)return;
  dragging=true;startX=e.clientX;startY=e.clientY;startTx=tx;startTy=ty;
  root.classList.add('cc-grabbing');
});
window.addEventListener('mousemove',function(e){
  if(!dragging)return;
  tx=startTx+(e.clientX-startX);ty=startTy+(e.clientY-startY);
  applyTransform();
});
window.addEventListener('mouseup',function(){dragging=false;root.classList.remove('cc-grabbing');});
var touches={},lastPinchDist=0;
root.addEventListener('touchstart',function(e){
  for(var i=0;i<e.changedTouches.length;i++){var t=e.changedTouches[i];touches[t.identifier]={x:t.clientX,y:t.clientY};}
  if(Object.keys(touches).length===1){var k=Object.keys(touches)[0];startX=touches[k].x;startY=touches[k].y;startTx=tx;startTy=ty;}
  if(Object.keys(touches).length===2){var ks=Object.keys(touches);var a=touches[ks[0]],b=touches[ks[1]];lastPinchDist=Math.hypot(b.x-a.x,b.y-a.y);}
},{passive:true});
root.addEventListener('touchmove',function(e){
  e.preventDefault();
  for(var i=0;i<e.changedTouches.length;i++){var t=e.changedTouches[i];if(touches[t.identifier])touches[t.identifier]={x:t.clientX,y:t.clientY};}
  var ks=Object.keys(touches);
  if(ks.length===1){tx=startTx+(touches[ks[0]].x-startX);ty=startTy+(touches[ks[0]].y-startY);applyTransform();}
  if(ks.length===2){var a=touches[ks[0]],b=touches[ks[1]];var d=Math.hypot(b.x-a.x,b.y-a.y);var factor=d/lastPinchDist;var cx=(a.x+b.x)/2,cy=(a.y+b.y)/2;var rr=root.getBoundingClientRect();zoomAt(cx-rr.left,cy-rr.top,factor);lastPinchDist=d;}
},{passive:false});
root.addEventListener('touchend',function(e){
  for(var i=0;i<e.changedTouches.length;i++)delete touches[e.changedTouches[i].identifier];
  if(Object.keys(touches).length===1){var k=Object.keys(touches)[0];startX=touches[k].x;startY=touches[k].y;startTx=tx;startTy=ty;}
},{passive:true});
window.addEventListener('keydown',function(e){
  if(e.key==='0'&&(e.metaKey||e.ctrlKey)){e.preventDefault();fitToScreen();}
  if((e.key==='='||e.key==='+')&&(e.metaKey||e.ctrlKey)){e.preventDefault();var rr=root.getBoundingClientRect();zoomAt(rr.width/2,rr.height/2,1.2);}
  if(e.key==='-'&&(e.metaKey||e.ctrlKey)){e.preventDefault();var rr=root.getBoundingClientRect();zoomAt(rr.width/2,rr.height/2,0.833);}
});
document.getElementById('cc-btn-fit').addEventListener('click',fitToScreen);
document.getElementById('cc-btn-in').addEventListener('click',function(){var rr=root.getBoundingClientRect();zoomAt(rr.width/2,rr.height/2,1.25);});
document.getElementById('cc-btn-out').addEventListener('click',function(){var rr=root.getBoundingClientRect();zoomAt(rr.width/2,rr.height/2,0.8);});
// Wait for React artboards to mount, then layout + fit
function tryFit(attempts){
  var artboards=content.querySelectorAll('[data-artboard]');
  if(artboards.length>0&&artboards[0].offsetWidth>0){
    layoutHorizontal(artboards);
    // Start zoomed in on first artboard at readable scale, not fit-all
    var vw=window.innerWidth,vh=window.innerHeight;
    var first=artboards[0];
    var fw=first.offsetWidth||375,fh=first.offsetHeight||812;
    scale=clamp(Math.min(vw/fw,vh/fh)*0.8,MIN,MAX);
    tx=(vw-fw*scale)/2;
    ty=(vh-fh*scale)/2;
    applyTransform();
    return;
  }
  if(attempts>0)setTimeout(function(){tryFit(attempts-1);},300);
  else fitToScreen();
}
if(document.readyState==='complete'){tryFit(30);}
else{window.addEventListener('load',function(){tryFit(30);});}
})();</script>`

	toolbar := `<div id="cc-toolbar">
<button id="cc-btn-back" title="Back" onclick="history.length>1?history.back():window.close()"><svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7"/></svg>Back</button>
<div class="cc-sep"></div>
<button id="cc-btn-out" title="Zoom out (⌘-)">−</button>
<span class="cc-zoom-label" id="cc-zoom-label">100%</span>
<button id="cc-btn-in" title="Zoom in (⌘+)">+</button>
<div class="cc-sep"></div>
<button id="cc-btn-fit" title="Fit to screen (⌘0)">Fit</button>
</div>`

	// Inject CSS into <head> (before </head>)
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx != -1 {
		html = html[:idx] + canvasCSS + "\n" + html[idx:]
	} else {
		html = canvasCSS + "\n" + html
	}

	// Wrap <body> content in canvas divs + append toolbar and JS before </body>
	bodyOpen := strings.Index(strings.ToLower(html), "<body")
	if bodyOpen == -1 {
		// No <body> tag — wrap everything
		return html
	}
	// Find end of opening <body...> tag
	bodyTagEnd := strings.Index(html[bodyOpen:], ">")
	if bodyTagEnd == -1 {
		return html
	}
	bodyTagEnd += bodyOpen + 1 // absolute index after '>'

	// Find </body>
	bodyClose := strings.LastIndex(strings.ToLower(html), "</body>")
	if bodyClose == -1 {
		return html
	}

	before := html[:bodyTagEnd]
	inner := html[bodyTagEnd:bodyClose]
	after := html[bodyClose:]

	return before +
		"\n<div id=\"cc-canvas-root\"><div id=\"cc-canvas-content\">\n" +
		inner +
		"\n</div></div>\n" + toolbar + "\n" + canvasJS + "\n" +
		after
}

// removeSrcAttr strips src="..." from an attribute string.
var srcAttrRe = regexp.MustCompile(`(?i)\s*\bsrc="[^"]*"`)

func removeSrcAttr(attrs string) string {
	return strings.TrimSpace(srcAttrRe.ReplaceAllString(attrs, ""))
}

// extractDepName attempts to extract "pkg@version" from a CDN URL.
// Examples:
//
//	https://unpkg.com/react@18.3.1/umd/react.development.js → react@18.3.1
//	https://cdn.jsdelivr.net/npm/react-dom@18.3.1/+esm       → react-dom@18.3.1
func extractDepName(rawURL string) string {
	// Strip protocol + host to get path segment
	path := rawURL
	if idx := strings.Index(rawURL, "://"); idx != -1 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash != -1 {
			path = rest[slash+1:]
		}
	}

	// jsdelivr: /npm/pkg@ver/... or /gh/...
	path = strings.TrimPrefix(path, "npm/")
	path = strings.TrimPrefix(path, "gh/")

	// Take first path segment (before next /)
	if slash := strings.Index(path, "/"); slash != -1 {
		path = path[:slash]
	}

	// path should now be "react@18.3.1" or similar
	if strings.Contains(path, "@") && path != "" {
		return path
	}
	return ""
}
