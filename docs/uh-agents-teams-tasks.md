# User Hypothesis: Agents, Teams, and Tasks

## Problem Statement

Claudio operators need a way to coordinate complex multi-step work across multiple specialized agents. Current workflows force operators to manually schedule agents one at a time, wait for results, then decide next steps—creating bottlenecks for parallel work, conditional logic, and team-based task allocation.

## User Groups

### Primary: Team Leads / Orchestrators
**Need:** Decompose work into focused tasks, assign them to the right agent specialists in parallel, track progress, and handle conditional next-steps based on results.

**Pain:** Manual agent scheduling is error-prone and slow; no way to express "run these 3 agents in parallel, then if any fail, escalate to senior engineer."

### Secondary: Individual Operators
**Need:** Spawn agents for specialized work without managing coordination details; see what's assigned to them and what results came back.

**Need:** Default harnesses that pre-assemble common agent teams for typical workflows (e.g., "security audit," "performance optimization").

## Core Features

### 1. Teams – Pre-assembled Agent Groups
**Goal:** Minimize repetitive agent coordination by packaging specialist agents into named teams.

**User story:**
- As a team lead, I can define a team (e.g., "frontend-audit-team" = frontend-senior + qa) so I don't repeatedly specify the same agent set.
- I can instantiate a team once, then refer to it by name in task flows.
- Teams can be version-controlled and shared across operators.

**Design notes:**
- Teams are lightweight references to agent specs (no orchestration logic of their own).
- Operators can override team members at instantiation time ("use senior-backend instead of mid-level").
- Teams support templating: variables like `$PROJECT_DOMAIN` are resolved at instantiation.

**Acceptance criteria:**
- ✓ Team definition file format (JSON or YAML)
- ✓ Save/load teams from filesystem
- ✓ Instantiate a team and get a list of agents to spawn
- ✓ Override individual team members at instantiation

---

### 2. Tasks – Assignable Units of Work
**Goal:** Break complex work into discrete, trackable, assignable units that can be delegated to agents or humans.

**User story:**
- As a team lead, I can create a task (e.g., "Review PR #42 for security issues") and assign it to an agent or human.
- I can see which tasks are pending, in-progress, and completed.
- I can mark tasks as done or blocked with notes.

**Design notes:**
- Tasks track: title, description, assignee (agent or human), status (pending/in-progress/done/blocked), owner (who created it), and created/updated timestamps.
- Tasks are persistent (stored in filesystem or lightweight DB).
- Team leads can query tasks by status, assignee, or tags.

**Acceptance criteria:**
- ✓ Create / read / update / delete tasks
- ✓ Task storage (filesystem JSON is fine for MVP)
- ✓ Task listing with filters (by status, assignee)
- ✓ Mark task complete and attach notes

---

### 3. Workflows – Task Sequencing with Conditions
**Goal:** Express sequential and parallel work with conditional branching ("if this task fails, escalate to X").

**User story:**
- As a team lead, I can define a workflow (e.g., "run tests → if pass, deploy; if fail, notify slack").
- Workflows spawn tasks, wait for completion, then decide next steps.
- I can re-run failed tasks without re-running the whole workflow.

**Design notes:**
- Workflows are DAGs: tasks are nodes, edges express dependencies or conditions.
- Conditions are simple: task success/failure, result content (e.g., "if test coverage < 80%").
- Workflows are expressed in YAML or JSON for version control.
- MVP: sequential execution with success/failure branches; no complex looping.

**Acceptance criteria:**
- ✓ Define workflow (YAML/JSON) with tasks and dependencies
- ✓ Execute workflow: spawn tasks in order, wait for results, branch on success/failure
- ✓ Log workflow execution (which tasks ran, results, timestamps)
- ✓ Resumable workflows (re-run a failed task and continue)

---

### 4. Task Communication – Passing Results Between Agents
**Goal:** Allow tasks to send output to subsequent tasks (e.g., "run investigation, then pass findings to the fixer").

**User story:**
- As an operator, I can chain tasks so that output from one becomes input to the next.
- I don't manually copy-paste results between agent calls.

**Design notes:**
- Tasks store output (stdout, artifacts, structured results).
- Workflows can interpolate task output into downstream task descriptions (e.g., `{task_investigation.findings}`).
- Output format is flexible: plain text, JSON, file paths.

**Acceptance criteria:**
- ✓ Tasks capture and store output
- ✓ Workflows support output interpolation in task descriptions
- ✓ Operator can query task output before/after execution

---

## Implementation Roadmap

### Phase 1: Tasks Foundation (Week 1)
- [ ] Task schema and CRUD operations
- [ ] Persistent storage (filesystem JSON)
- [ ] Task listing and filtering by status/assignee
- [ ] CLI commands: `claudio task create`, `list`, `update`, `done`

### Phase 2: Teams (Week 1–2)
- [ ] Team definition schema (YAML/JSON)
- [ ] Team instantiation with variable substitution
- [ ] Team member override at runtime
- [ ] CLI commands: `claudio team create`, `list`, `instantiate`

### Phase 3: Workflows (Week 2–3)
- [ ] Workflow schema (DAG definition)
- [ ] Workflow execution engine (sequential + conditional branching)
- [ ] Task spawning within workflows
- [ ] Execution logging and resumability
- [ ] CLI commands: `claudio workflow create`, `run`, `logs`, `retry`

### Phase 4: Task Communication (Week 3–4)
- [ ] Output capture and storage
- [ ] Interpolation in task descriptions
- [ ] Workflow-level variable passing
- [ ] CLI commands: `claudio task output <id>`

### Phase 5: Operator UX (Week 4+)
- [ ] Default harnesses (pre-built teams + workflows for common patterns)
- [ ] Dashboard / status view (which tasks are running, who's assigned to what)
- [ ] Slack / email notifications for task updates
- [ ] Operator search and discovery of available teams/workflows

---

## Success Metrics

1. **Adoption:** Team leads regularly use task/team/workflow CLIs for multi-agent coordination.
2. **Reduction in manual work:** Operators spend < 5 min setting up a 5-agent audit (vs. > 20 min today).
3. **Error reduction:** Fewer mistakes from manual agent sequencing and result passing.
4. **Satisfaction:** Operators report "I can now easily run a full audit without manual coordination."

---

## Open Questions / Notes

- **Persistence:** Should we use a lightweight file-based store or a real DB? (MVP: files, upgrade later if needed.)
- **Human assignees:** Should tasks support assignment to humans (e.g., "escalate to X for manual review")? (Likely yes, but out of MVP scope.)
- **Agent integration:** How tightly coupled should Agent tool be? (Task should NOT know about Agent internals; just spawn and collect results.)
- **Notifications:** Should workflow completion/failure auto-notify? (Nice-to-have; implement in Phase 5.)
