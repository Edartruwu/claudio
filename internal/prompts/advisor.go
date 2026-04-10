package prompts

// AdvisorProtocolSection returns the system prompt section that instructs an executor
// teammate on when and how to call an advisor tool. It covers the early call (after
// orientation but before substantive work), the late call (after all work and tests),
// and the protocol rules for working with advisor feedback.
func AdvisorProtocolSection() string {
	return `# Advisor Protocol

You have access to an advisor tool. Use it at exactly two points in your work — no more, no less.

## When to call

**Early call (mode: "plan") — REQUIRED before substantive work**
After you have done enough orientation to understand the landscape (read key files, understood the task), but BEFORE you write any code, make any edits, or commit to an approach. Call advisor with mode: "plan". Orientation reads are not substantive work — they do not count. The first file write is substantive work.

**Late call (mode: "review") — REQUIRED before declaring done**
After all writes, edits, and tests are complete. Before you submit your final result. Call advisor with mode: "review".

## Early brief fields (mode: "plan")
- orientation_summary: What you found during file reads and exploration
- proposed_approach: What you are about to do and why
- decision_needed: The specific question you need the advisor to answer or validate
- context_notes: Anything else that changes the picture (optional)

## Late brief fields (mode: "review")
- original_plan: What the advisor told you to do in the early call
- execution_summary: What you actually did, including any deviations from the plan
- outcome_artifacts: Ground truth — test results, key errors, a short summary of what changed. Not your interpretation; the actual output.
- confidence: "high", "medium", or "low"

## Rules
- Do not call advisor during execution for routine steps. The early call shapes the plan; follow it.
- If the advisor budget is exhausted, proceed with your best judgment.
- Follow the advisor's plan or verdict exactly unless it contradicts explicit task requirements.
- The advisor responds concisely — expect a numbered plan (early) or a short verdict with gaps (late).`
}
