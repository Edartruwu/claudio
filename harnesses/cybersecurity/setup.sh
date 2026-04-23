#!/usr/bin/env bash
# Cybersecurity harness installer
# Detects OS and installs all required tools.
# Idempotent — safe to run multiple times.

set -euo pipefail

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# ---------------------------------------------------------------------------
# Tracking
# ---------------------------------------------------------------------------
INSTALLED=()
ALREADY_PRESENT=()
FAILED=()

# ---------------------------------------------------------------------------
# Banner
# ---------------------------------------------------------------------------
print_banner() {
  echo -e "${CYAN}${BOLD}"
  echo "  ╔═══════════════════════════════════════════════════╗"
  echo "  ║     Claudio Cybersecurity Harness — Setup         ║"
  echo "  ║     Installs all required pentesting tools        ║"
  echo "  ╚═══════════════════════════════════════════════════╝"
  echo -e "${RESET}"
}

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------
info()    { echo -e "${CYAN}[INFO]${RESET}  $*"; }
ok()      { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*"; }
error()   { echo -e "${RED}[ERROR]${RESET} $*"; }
section() { echo -e "\n${BOLD}${YELLOW}▶ $*${RESET}"; }

# ---------------------------------------------------------------------------
# OS detection
# ---------------------------------------------------------------------------
detect_os() {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
  elif [[ -f /etc/os-release ]]; then
    source /etc/os-release
    case "${ID:-}" in
      ubuntu|debian|linuxmint|pop)
        OS="debian"
        ;;
      fedora|rhel|centos|rocky|almalinux)
        OS="fedora"
        ;;
      arch|manjaro|endeavouros|garuda)
        OS="arch"
        ;;
      *)
        OS="unknown"
        ;;
    esac
  else
    OS="unknown"
  fi
}

# ---------------------------------------------------------------------------
# Tool presence check
# ---------------------------------------------------------------------------
is_installed() {
  command -v "$1" &>/dev/null
}

# ---------------------------------------------------------------------------
# Install wrappers — each returns 0 on success, 1 on failure
# ---------------------------------------------------------------------------

# Try a command; on failure mark tool failed and continue (don't exit).
try_install() {
  local tool="$1"
  shift
  if "$@" &>/dev/null 2>&1; then
    ok "Installed: ${tool}"
    INSTALLED+=("$tool")
    return 0
  else
    error "Failed: ${tool}"
    FAILED+=("$tool")
    return 1
  fi
}

# ------ macOS ------
install_brew_cask() {
  local tool="$1"; local pkg="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  info "Installing ${tool} via brew cask..."
  try_install "$tool" brew install --cask "$pkg"
}

install_brew() {
  local tool="$1"; local pkg="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  info "Installing ${tool} via brew..."
  try_install "$tool" brew install "$pkg"
}

# ------ Debian/Ubuntu ------
install_apt() {
  local tool="$1"; local pkg="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  info "Installing ${tool} via apt..."
  try_install "$tool" sudo apt-get install -y "$pkg"
}

# ------ Fedora/RHEL ------
install_dnf() {
  local tool="$1"; local pkg="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  info "Installing ${tool} via dnf..."
  try_install "$tool" sudo dnf install -y "$pkg"
}

# ------ Arch ------
install_pacman() {
  local tool="$1"; local pkg="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  info "Installing ${tool} via pacman..."
  try_install "$tool" sudo pacman -S --noconfirm "$pkg"
}

# ------ Go install ------
install_go_tool() {
  local tool="$1"; local pkg="$2"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  if ! is_installed go; then
    warn "Go not found — skipping ${tool} (install Go first)"
    FAILED+=("$tool (needs Go)")
    return 1
  fi
  info "Installing ${tool} via go install..."
  if go install "$pkg" &>/dev/null 2>&1; then
    # go install puts binaries in $(go env GOPATH)/bin
    local gobin
    gobin="$(go env GOPATH)/bin"
    if [[ -f "${gobin}/${tool}" ]]; then
      ok "Installed: ${tool} → ${gobin}/${tool}"
      INSTALLED+=("$tool")
      warn "Ensure ${gobin} is in your PATH"
      return 0
    else
      error "go install ran but ${tool} not found in ${gobin}"
      FAILED+=("$tool")
      return 1
    fi
  else
    error "Failed: ${tool}"
    FAILED+=("$tool")
    return 1
  fi
}

# ------ gem fallback ------
install_gem() {
  local tool="$1"; local gem_name="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  if ! is_installed gem; then
    warn "gem not found — skipping ${tool}"
    FAILED+=("$tool (needs gem)")
    return 1
  fi
  info "Installing ${tool} via gem..."
  try_install "$tool" gem install "$gem_name"
}

# ------ cargo install ------
install_cargo() {
  local tool="$1"; local crate="${2:-$1}"
  if is_installed "$tool"; then
    ok "Already present: ${tool}"
    ALREADY_PRESENT+=("$tool")
    return 0
  fi
  if ! is_installed cargo; then
    warn "cargo not found — skipping ${tool}"
    FAILED+=("$tool (needs cargo)")
    return 1
  fi
  info "Installing ${tool} via cargo..."
  if cargo install "$crate" &>/dev/null 2>&1; then
    ok "Installed: ${tool}"
    INSTALLED+=("$tool")
    return 0
  else
    error "Failed: ${tool}"
    FAILED+=("$tool")
    return 1
  fi
}

# ---------------------------------------------------------------------------
# Per-OS install routines
# ---------------------------------------------------------------------------

install_macos() {
  section "macOS — Homebrew"

  if ! is_installed brew; then
    warn "Homebrew not found. Install from https://brew.sh then re-run."
    exit 1
  fi

  install_brew  nmap
  install_brew  nikto
  install_brew  sqlmap
  install_brew  jq

  # whatweb — no brew formula, no pip package. Install from GitHub source.
  if ! is_installed whatweb; then
    info "Installing whatweb from source..."
    local whatweb_dir="${HOME}/.local/share/whatweb"
    if [ -d "$whatweb_dir" ]; then
      info "Updating existing whatweb clone..."
      git -C "$whatweb_dir" pull --quiet 2>/dev/null || true
    else
      git clone --quiet https://github.com/urbanadventurer/WhatWeb.git "$whatweb_dir" 2>/dev/null
    fi
    if [ -f "$whatweb_dir/whatweb" ]; then
      # Ensure Ruby deps are installed
      if is_installed bundle; then
        (cd "$whatweb_dir" && bundle install --quiet 2>/dev/null) || true
      elif is_installed gem; then
        (cd "$whatweb_dir" && gem install bundler --quiet 2>/dev/null && bundle install --quiet 2>/dev/null) || true
      fi
      # Symlink to a location in PATH
      local bin_dir="${HOME}/.local/bin"
      mkdir -p "$bin_dir"
      ln -sf "$whatweb_dir/whatweb" "$bin_dir/whatweb"
      if [[ ":$PATH:" != *":$bin_dir:"* ]]; then
        warn "Add ${bin_dir} to your PATH: export PATH=\"\$HOME/.local/bin:\$PATH\""
      fi
      ok "Installed: whatweb (source → ${whatweb_dir})"
      INSTALLED+=(whatweb)
    else
      warn "Could not install whatweb — install manually: https://github.com/urbanadventurer/WhatWeb"
      FAILED+=(whatweb)
    fi
  else
    ok "Already present: whatweb"
    ALREADY_PRESENT+=(whatweb)
  fi

  install_brew  feroxbuster
  install_brew  ffuf

  # Go tools
  install_go_tool subfinder   "github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
  install_go_tool httpx       "github.com/projectdiscovery/httpx/cmd/httpx@latest"
  install_go_tool nuclei      "github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"

  # OWASP ZAP
  if ! is_installed zap.sh && ! [[ -d "/Applications/OWASP ZAP.app" ]]; then
    info "Installing OWASP ZAP via brew cask..."
    if brew install --cask owasp-zap &>/dev/null 2>&1; then
      ok "Installed: OWASP ZAP"
      INSTALLED+=(owasp-zap)
    else
      error "Failed: OWASP ZAP"
      FAILED+=("owasp-zap")
    fi
  else
    ok "Already present: OWASP ZAP"
    ALREADY_PRESENT+=(owasp-zap)
  fi

  # Node.js
  check_nodejs
}

install_debian() {
  section "Debian/Ubuntu — apt"

  info "Updating apt cache..."
  sudo apt-get update -qq

  install_apt nmap
  install_apt nikto
  install_apt sqlmap
  install_apt jq

  # whatweb — package name varies
  if ! is_installed whatweb; then
    if apt-cache show whatweb &>/dev/null 2>&1; then
      install_apt whatweb
    else
      info "whatweb not in apt — trying gem..."
      install_gem whatweb
    fi
  else
    ok "Already present: whatweb"
    ALREADY_PRESENT+=(whatweb)
  fi

  # feroxbuster — not in apt, try cargo
  if ! is_installed feroxbuster; then
    info "feroxbuster not in apt — trying cargo..."
    install_cargo feroxbuster feroxbuster
  else
    ok "Already present: feroxbuster"
    ALREADY_PRESENT+=(feroxbuster)
  fi

  # ffuf — try apt, then go install
  if ! is_installed ffuf; then
    if apt-cache show ffuf &>/dev/null 2>&1; then
      install_apt ffuf
    else
      info "ffuf not in apt — trying go install..."
      install_go_tool ffuf "github.com/ffuf/ffuf/v2@latest"
    fi
  else
    ok "Already present: ffuf"
    ALREADY_PRESENT+=(ffuf)
  fi

  # Go tools
  install_go_tool subfinder "github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
  install_go_tool httpx     "github.com/projectdiscovery/httpx/cmd/httpx@latest"
  install_go_tool nuclei    "github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"

  # OWASP ZAP
  install_zap_linux

  # Node.js
  check_nodejs
}

install_fedora() {
  section "Fedora/RHEL — dnf"

  install_dnf nmap
  install_dnf nikto
  install_dnf sqlmap
  install_dnf jq

  # whatweb
  if ! is_installed whatweb; then
    if dnf info whatweb &>/dev/null 2>&1; then
      install_dnf whatweb
    else
      info "whatweb not in dnf — trying gem..."
      install_gem whatweb
    fi
  else
    ok "Already present: whatweb"
    ALREADY_PRESENT+=(whatweb)
  fi

  # feroxbuster — try dnf (Fedora has it), else cargo
  if ! is_installed feroxbuster; then
    if dnf info feroxbuster &>/dev/null 2>&1; then
      install_dnf feroxbuster
    else
      info "feroxbuster not in dnf — trying cargo..."
      install_cargo feroxbuster feroxbuster
    fi
  else
    ok "Already present: feroxbuster"
    ALREADY_PRESENT+=(feroxbuster)
  fi

  # ffuf — try dnf, then go install
  if ! is_installed ffuf; then
    if dnf info ffuf &>/dev/null 2>&1; then
      install_dnf ffuf
    else
      install_go_tool ffuf "github.com/ffuf/ffuf/v2@latest"
    fi
  else
    ok "Already present: ffuf"
    ALREADY_PRESENT+=(ffuf)
  fi

  # Go tools
  install_go_tool subfinder "github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
  install_go_tool httpx     "github.com/projectdiscovery/httpx/cmd/httpx@latest"
  install_go_tool nuclei    "github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"

  # OWASP ZAP
  install_zap_linux

  # Node.js
  check_nodejs
}

install_arch() {
  section "Arch Linux — pacman"

  install_pacman nmap
  install_pacman nikto
  install_pacman sqlmap
  install_pacman jq

  # whatweb
  if ! is_installed whatweb; then
    if pacman -Si whatweb &>/dev/null 2>&1; then
      install_pacman whatweb
    else
      info "whatweb not in pacman — trying gem..."
      install_gem whatweb
    fi
  else
    ok "Already present: whatweb"
    ALREADY_PRESENT+=(whatweb)
  fi

  # feroxbuster — in AUR; try pacman first, then cargo
  if ! is_installed feroxbuster; then
    if pacman -Si feroxbuster &>/dev/null 2>&1; then
      install_pacman feroxbuster
    else
      info "feroxbuster not in official repos — trying cargo..."
      install_cargo feroxbuster feroxbuster
    fi
  else
    ok "Already present: feroxbuster"
    ALREADY_PRESENT+=(feroxbuster)
  fi

  # ffuf
  if ! is_installed ffuf; then
    if pacman -Si ffuf &>/dev/null 2>&1; then
      install_pacman ffuf
    else
      install_go_tool ffuf "github.com/ffuf/ffuf/v2@latest"
    fi
  else
    ok "Already present: ffuf"
    ALREADY_PRESENT+=(ffuf)
  fi

  # Go tools
  install_go_tool subfinder "github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
  install_go_tool httpx     "github.com/projectdiscovery/httpx/cmd/httpx@latest"
  install_go_tool nuclei    "github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"

  # OWASP ZAP
  if ! is_installed zaproxy; then
    if pacman -Si zaproxy &>/dev/null 2>&1; then
      install_pacman zaproxy
    else
      install_zap_linux
    fi
  else
    ok "Already present: OWASP ZAP"
    ALREADY_PRESENT+=(owasp-zap)
  fi

  # Node.js
  check_nodejs
}

# ---------------------------------------------------------------------------
# OWASP ZAP — Linux generic (download from GitHub)
# ---------------------------------------------------------------------------
install_zap_linux() {
  if is_installed zaproxy || is_installed zap.sh; then
    ok "Already present: OWASP ZAP"
    ALREADY_PRESENT+=(owasp-zap)
    return 0
  fi
  warn "OWASP ZAP not available via package manager."
  warn "Download from: https://www.zaproxy.org/download/"
  warn "Or run: snap install zaproxy --classic  (if snap available)"
  FAILED+=("owasp-zap (manual install required — https://www.zaproxy.org/download/)")
}

# ---------------------------------------------------------------------------
# Node.js check
# ---------------------------------------------------------------------------
check_nodejs() {
  section "Node.js"
  if is_installed node; then
    local ver
    ver=$(node --version 2>/dev/null | sed 's/v//')
    local major="${ver%%.*}"
    if [[ "$major" -ge 18 ]]; then
      ok "Already present: Node.js v${ver} (≥18)"
      ALREADY_PRESENT+=(nodejs)
    else
      warn "Node.js v${ver} found but <18. Upgrade via nvm: https://github.com/nvm-sh/nvm"
      FAILED+=("nodejs (upgrade to ≥18 via nvm)")
    fi
  else
    warn "Node.js not found."
    warn "Install via nvm (recommended): https://github.com/nvm-sh/nvm"
    warn "Or via package manager: brew install node / apt install nodejs"
    FAILED+=("nodejs (install ≥18 via nvm or package manager)")
  fi
}

# ---------------------------------------------------------------------------
# linpeas note
# ---------------------------------------------------------------------------
note_linpeas() {
  section "linpeas"
  info "linpeas is auto-downloaded by the linpeas-audit plugin at runtime."
  info "No manual install needed."
}

# ---------------------------------------------------------------------------
# Preview tools to install
# ---------------------------------------------------------------------------
preview_tools() {
  section "Tools to install / verify"
  echo "  nmap, nikto, sqlmap, jq, whatweb"
  echo "  feroxbuster, ffuf"
  echo "  subfinder, httpx, nuclei  (Go tools)"
  echo "  OWASP ZAP"
  echo "  Node.js 18+"
  echo "  linpeas  (auto-downloaded at runtime)"
  echo ""
}

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary() {
  section "Summary"

  if [[ ${#INSTALLED[@]} -gt 0 ]]; then
    echo -e "${GREEN}${BOLD}Installed (${#INSTALLED[@]}):${RESET}"
    for t in "${INSTALLED[@]}"; do echo "  ✓ ${t}"; done
  fi

  if [[ ${#ALREADY_PRESENT[@]} -gt 0 ]]; then
    echo -e "${CYAN}${BOLD}Already present (${#ALREADY_PRESENT[@]}):${RESET}"
    for t in "${ALREADY_PRESENT[@]}"; do echo "  · ${t}"; done
  fi

  if [[ ${#FAILED[@]} -gt 0 ]]; then
    echo -e "${RED}${BOLD}Failed / action required (${#FAILED[@]}):${RESET}"
    for t in "${FAILED[@]}"; do echo "  ✗ ${t}"; done
    echo ""
    warn "Some tools failed. Check messages above and install manually."
  else
    echo ""
    ok "All tools installed or already present."
  fi

  echo ""
  echo -e "${BOLD}Next steps:${RESET}"
  echo "  1. Ensure $(go env GOPATH 2>/dev/null || echo '~/go')/bin is in your PATH"
  echo "  2. Run: claudio harness install ./harnesses/cybersecurity"
  echo "  3. Start a session and invoke a skill: /pentest, /pentest-web, etc."
  echo ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  print_banner
  detect_os

  info "Detected OS: ${OS}"
  preview_tools

  case "$OS" in
    macos)  install_macos  ;;
    debian) install_debian ;;
    fedora) install_fedora ;;
    arch)   install_arch   ;;
    unknown)
      error "Unsupported OS detected."
      echo ""
      echo "Manual install instructions:"
      echo "  nmap:        https://nmap.org/download"
      echo "  nikto:       https://github.com/sullo/nikto"
      echo "  sqlmap:      https://sqlmap.org"
      echo "  jq:          https://stedolan.github.io/jq/download/"
      echo "  whatweb:     https://github.com/urbanadventurer/WhatWeb"
      echo "  feroxbuster: https://github.com/epi052/feroxbuster/releases"
      echo "  ffuf:        https://github.com/ffuf/ffuf/releases"
      echo "  subfinder:   go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
      echo "  httpx:       go install github.com/projectdiscovery/httpx/cmd/httpx@latest"
      echo "  nuclei:      go install github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"
      echo "  OWASP ZAP:   https://www.zaproxy.org/download/"
      echo "  Node.js 18+: https://github.com/nvm-sh/nvm"
      exit 1
      ;;
  esac

  note_linpeas
  print_summary
}

main "$@"
