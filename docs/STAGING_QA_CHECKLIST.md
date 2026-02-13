# Staging QA Checklist (Negative Testing)

## Purpose
Validate that safety rails block invalid actions and that the UI communicates failures clearly.

## Scope
- After Class workflows (grading, close round, student move)
- Payment and renewal workflows in Pre-Enrolment
- Capacity and late-join guardrails

## Test Setup
1. Use **staging** with an Ops Admin account and a Mentor Head account.
2. Ensure browser DevTools Network tab is open.
3. Record each attempt with:
- Screenshot of UI result
- API status code and response body
- Timestamp and tester initials

## Global Pass/Fail Rule
- **Pass**: Invalid action is blocked by backend and UI shows a visible error/warning (banner/toast/inline message), with no silent data corruption.
- **Fail**: Action proceeds when it should not, or UI gives no meaningful feedback.

## Scenario A: The Mind Changer
### Goal
Verify roster consistency when moving a student between classes.

### Steps
1. In Classes board, pick Class A with at least 1 student and Class B in same level/days/time.
2. Move one student from Class A to Class B.
3. Refresh page.
4. Re-open both class cards.

### Checks
- Class A count decreases by 1.
- Class B count increases by 1.
- Student appears only once (no duplicate in both classes).
- If move is invalid, UI shows explicit error and counts stay unchanged.

### Evidence to Capture
- Before/after screenshots of both class cards.
- Network call for move endpoint and response code.

## Scenario B: The Premature Closer
### Goal
Ensure Close Round fails when one or more students have no final grade.

### Steps
1. Login as Mentor Head.
2. Open a class where at least one student has no final grade.
3. Click **Close Round**.

### Checks
- Request is rejected (expected `400`).
- UI shows clear error message (example: missing final grade).
- Class remains open (not closed/archived).

### Evidence to Capture
- Error message screenshot.
- Network response payload containing error.

## Scenario C: The Free Ride
### Goal
Block renewal student from skipping payment and entering waiting flow.

### Steps
1. Open Pre-Enrolment for a returning student in `renewal_pending` with 0 credits.
2. Try **Move to Waiting List** without recording valid payment.

### Checks
- Action is blocked.
- Either button is disabled with explanatory tooltip/text, or backend returns error shown in red banner.
- Lead status does not change to `waiting_for_round`.

### Evidence to Capture
- Button state screenshot or error banner screenshot.
- Lead status before/after.

## Scenario D: The Overcrowded Class
### Goal
Ensure class capacity protection works for Late Join and Move.

### Steps
1. Find a class at `6/6`.
2. Attempt to add a 7th student via **Add as Late Joiner**.
3. Attempt to move another student into the same full class.

### Checks
- System blocks both actions.
- UI shows clear failure reason (capacity / no eligible class).
- Class stays at `6/6` after refresh.

### Evidence to Capture
- Modal/error screenshot.
- Network response code and error message.

## Regression Quick Checks (Recommended)
1. Payment guard: Try recording payment before Packages Sent/Offer state is valid.
- Expected: blocked with warning banner/error.
2. Full payment mismatch: Set `Full Payment` with amount not equal final due.
- Expected: blocked with explicit validation error.
3. Late Join eligibility: Try late-join on non-`ready_to_start` lead.
- Expected: blocked with explicit error.

## Execution Log Template
| Scenario | Tester | Time | Result | API Code | UI Message Seen | Notes |
|---|---|---|---|---|---|---|
| A |  |  |  |  |  |  |
| B |  |  |  |  |  |  |
| C |  |  |  |  |  |  |
| D |  |  |  |  |  |  |

## Sign-off
- Ops Admin: __________
- QA Owner: __________
- Date: __________
