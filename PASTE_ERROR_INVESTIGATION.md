# Investigation: "No mailbox available" Error on Paste

## Subject
Traced the "No mailbox available" error that appears when a user copies and pastes text in the TUI, particularly when the pasted content or resulting message triggers the `>>agentname` handler.

## Codebase Overview
The claudio TUI is built with Bubbletea, with bracketed paste support enabled. The architecture includes:
- **Root model** (`internal/tui/root.go`) — main TUI state machine and message router
- **Prompt component** (`internal/tui/prompt/prompt.go`) — textarea with paste buffering and long-paste collapsing
- **Teams/Agents system** (`internal/teams/runner.go`, `internal/teams/mailbox.go`) — manages agents and message passing
- **Message dispatch** — pasted text is expanded, submitted, and routed to either the LLM or a specific agent via `>>agentname` syntax

## Key Findings

### 1. Error Definition & Location
- **File:** `internal/tui/root.go:4850`
- **Function:** `handleAgentMessage(text string) (tea.Model, tea.Cmd)`
- **Error condition:** Triggered when `m.appCtx.TeamRunner.GetMailbox()` returns `nil`

```go
mailbox := m.appCtx.TeamRunner.GetMailbox()
if mailbox == nil {
    m.addMessage(ChatMessage{Type: MsgError, Content: "No mailbox available"})
    m.refreshViewport()
    return m, nil
}
```

### 2. Paste Event Handling & Text Expansion
- **Paste capture:** `internal/tui/prompt/prompt.go:179-201`
  - Bracketed paste is enabled at TUI Init: `internal/tui/root.go:642`
  - When `msg.Paste` is true, text is accumulated in `m.pasteBuffer`
  - For large pastes (>pasteThreshold), a reference like `[Pasted text #1 +42 lines]` is inserted instead

- **Text expansion on submit:** `internal/tui/prompt/prompt.go:222-234` and `594-612`
  - When Enter is pressed, `ExpandedValue()` expands all paste references to full text
  - A `SubmitMsg` is created with the fully expanded text
  - Example: `>>agent [Pasted text #1]` becomes `>>agent <full pasted content>`

- **Paste finalization:** `internal/tui/prompt/prompt.go:561-590`
  - Long pastes are stored in `m.pastedContents[id]`
  - A reference token is inserted in the textarea instead of the full text
  - This keeps the UI responsive for large pastes

### 3. Message Routing to handleAgentMessage
- **File:** `internal/tui/root.go:2037-2038`
- **Trigger condition:** Message text starts with `">>"` (two angle brackets)
- **Call chain:**
  1. Prompt component sends `SubmitMsg` on Enter (line 233 in prompt.go)
  2. Root's `Update()` catches it (line 1259 in root.go)
  3. Routes to `handleSubmit(msg.Text)` (line 1259)
  4. Inside `handleSubmit()`, text is checked for `>>` prefix (line 2037)
  5. If match, calls `handleAgentMessage(text)` (line 2038)

### 4. The Root Cause: GetMailbox() Returns Nil
- **File:** `internal/teams/runner.go:981-994`
- **Implementation:**
  ```go
  func (r *TeammateRunner) GetMailbox() *Mailbox {
      return r.getMailbox(r.ActiveTeamName())
  }
  
  func (r *TeammateRunner) getMailbox(teamName string) *Mailbox {
      if teamName == "" {
          return nil
      }
      r.mu.RLock()
      defer r.mu.RUnlock()
      return r.mailboxes[teamName]
  }
  ```

- **Why it returns nil:**
  1. `ActiveTeamName()` returns empty string if:
     - No explicit active team is set (`r.activeTeam == ""`)
     - AND no teammates have been spawned yet (no agents in `r.teammates`)
  2. When teamName is empty, `getMailbox()` immediately returns `nil`
  3. A mailbox is only created when a team or agent is actively spawned/working

- **File:** `internal/teams/runner.go:954-964` (ActiveTeamName logic):
  ```go
  func (r *TeammateRunner) ActiveTeamName() string {
      r.mu.RLock()
      defer r.mu.RUnlock()
      if r.activeTeam != "" {
          return r.activeTeam
      }
      for _, s := range r.teammates {  // If no teammates, loop returns ""
          return s.TeamName
      }
      return ""
  }
  ```

### 5. The Paste Trigger Scenario
The error appears when:
1. User pastes text that starts with or expands to `>>agentname message`
2. Example: User pastes `>>my-agent` followed by pasted content starting with `[Pasted text #1]`
3. The expanded text becomes `>>my-agent [full pasted content]`
4. This routes to `handleAgentMessage()`
5. But no agents have been spawned yet (common in fresh sessions), so `GetMailbox()` returns `nil`
6. Error message is shown to user

### 6. Why This Happens on Large Pastes Specifically
- Small pastes (<pasteThreshold) are inserted directly into textarea
- Large pastes are collapsed to `[Pasted text #N]` references for UI performance
- If the pasted content begins with `>>agentname` or is pasted after a `>>` prefix in the prompt, the reference itself becomes a token
- On submission, the reference is expanded to full content
- This full content combined with the prefix triggers the agent message handler

## Symbol Map
| Symbol | File | Role |
|--------|------|------|
| `handleAgentMessage` | `internal/tui/root.go:4822` | Routes `>>agent message` to specific agent; checks for nil mailbox |
| `GetMailbox` | `internal/teams/runner.go:981` | Returns active team's mailbox or nil if no active team |
| `ActiveTeamName` | `internal/teams/runner.go:954` | Returns active team name or empty string if none |
| `finalizePaste` | `internal/tui/prompt/prompt.go:561` | Processes accumulated paste text; collapses large pastes to references |
| `ExpandedValue` | `internal/tui/prompt/prompt.go:594` | Expands all `[Pasted text #N]` references to their original content |
| `handleSubmit` | `internal/tui/root.go:2012` | Routes text to commands, agent messages, or LLM; calls handleAgentMessage if text starts with `>>` |
| `Update` (prompt) | `internal/tui/prompt/prompt.go:172` | Captures bracketed paste events; finalizes paste on Enter |

## Dependencies & Data Flow
1. **Paste Event Entry:** Bubbletea sends `tea.KeyMsg{Paste: true}` for bracketed paste sequences
2. **Capture & Buffer:** Prompt's `Update()` accumulates runes in `m.pasteBuffer` (lines 179-196)
3. **Collapse Decision:** `finalizePaste()` checks text length against `pasteThreshold` (line 570)
   - Small paste: inserted directly into textarea
   - Large paste: stored in `m.pastedContents[id]`, reference `[Pasted text #id]` inserted instead
4. **Submit Trigger:** Enter key calls `ExpandedValue()` to restore all paste references to full text
5. **Message Dispatch:**
   - `SubmitMsg` sent to root's `Update()`
   - Text routed through `handleSubmit()`
   - If starts with `>>`, routed to `handleAgentMessage()`
6. **Mailbox Check:** `handleAgentMessage()` calls `GetMailbox()`
   - If no active team/agents spawned: returns `nil`
   - Error added to chat and displayed to user

## Risks & Observations

### Critical Issue
**The error occurs because `handleAgentMessage()` unconditionally requires an active team with a mailbox.** This is not validated before the function is called. The user can type or paste `>>agentname message` at any time, but agents are only available after they've been spawned by a running team.

### Edge Cases
1. **No team is active:** Common in fresh sessions before running `/team` or invoking a skill
2. **Team exists but no agents spawned:** Team may be created but no agents running yet
3. **Pasted content with `>>` prefix:** A naive paste like `>>agent` or `>> agent` that wasn't intended as agent syntax could trigger this

### Design Question
The error message "No mailbox available" is technically accurate but not user-friendly. A better message would be:
- "No active team. Use a team or run /skill to activate agents for `>>` messaging."
- Or: "Agent messaging not available without an active team."

### Missing Guard
`handleAgentMessage()` should ideally check:
1. Is there an active team? (`ActiveTeamName() != ""`)
2. Has the team spawned any agents? (Check if `mailbox != nil` before using it)
3. Is the named agent actually part of the active team?

Currently, it only checks #2 at line 4849, but by that point the error is already imminent.

## Open Questions
1. **Is this intentional behavior?** Should users be prevented from using `>>` syntax when no agents are available, or should the command be queued for later?
2. **Should the error be recoverable?** Could the message be re-submitted after a team is activated, or should the user re-type it?
3. **What's the expected UX for team-less sessions?** Should `>>` be documented as team-only, or should single-prompt LLM sessions also support agent-style messaging?
4. **Paste timing issue:** When a user pastes `>>agent` immediately after opening the TUI, before spawning any agents, should the paste be automatically queued or buffered?

## Technical Details
- **Bracketed Paste Support:** Enabled via `tea.EnableBracketedPaste` at TUI init (root.go:642)
- **Bubbletea Integration:** The framework delivers paste as single `tea.KeyMsg{Paste: true}` (see comment at prompt.go:197-198)
- **pasteThreshold:** 200 characters (`internal/tui/prompt/prompt.go:58`)
  - Pastes **below** 200 chars: inserted directly into textarea
  - Pastes **above** 200 chars: collapsed to `[Pasted text #<id> +<lines> lines]` reference
- **Reference Format:** `[Pasted text #<id> +<lines> lines]` for multi-line pastes; `[Pasted text #<id>]` for single-line
- **Mailbox Creation:** Happens when team agents are spawned, not when team is created; see `internal/teams/runner.go` for spawn logic
