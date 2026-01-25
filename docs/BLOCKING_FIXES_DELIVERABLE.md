# Milestone 1 — Blocking Fixes Deliverable

Fixes for the 5 blocking issues from the QA audit. Minimal changes only.

---

## 1. Finance refund handler double CreateRefund

**What was wrong:** `POST /finance/refund/{leadID}` called `CreateRefund` twice (before and after validation). Validation used `detail.Payment.AmountPaid` (legacy) and a manual refund SUM instead of `GetTotalCoursePaid`.

**How fixed:**
- All validation runs **before** any DB call: amount > 0, payment method, date parse, `ValidateNotFutureDate`, amount ≤ total course paid.
- Total course paid from `models.GetTotalCoursePaid(leadID)` (same as pre-enrolment).
- `CreateRefund` invoked **once** after validation.
- Removed `db` import from finance handler.

**Files:** `internal/handlers/finance.go`

**Manual test:**
1. Open a lead with course payments. Create refund via `/finance/refund/{id}` form (amount, method, date).
2. Confirm exactly one refund OUT in Finance ledger; balance correct.
3. Submit again with same lead → second refund created (normal); total refunded ≤ total course paid.

---

## 2. Cancel + refund not atomic / not safe on retry

**What was wrong:** Cancel flow created refund then cancelled lead. No transaction. Retry after `CancelLead` failure caused double refund.

**How fixed:** Option B — idempotent refund. Added `CreateCancelRefundIdempotent` in repository:
- Uses `ref_key = "cancel_refund:<leadID>:<date>:<amount>"`.
- `INSERT ... ON CONFLICT (ref_key) DO NOTHING RETURNING id`. If conflict, skip insert, return success.
- Cancel handler calls `CreateCancelRefundIdempotent` instead of `CreateRefund`, then `CancelLead`.

**Files:** `internal/models/repository.go` (new `CreateCancelRefundIdempotent`), `internal/handlers/pre_enrolment.go` (cancel case uses it).

**Manual test:**
1. Cancel a paid lead with refund (amount, method, date). Confirm refund + cancel.
2. Simulate retry: duplicate POST (e.g. reload and resubmit cancel form). Confirm no second refund; lead still cancelled.

---

## 3. Cancel success message when no refund

**What was wrong:** Redirect always included `refund_recorded=1`, so “Lead cancelled and refund recorded.” appeared even when no refund (total course paid = 0).

**How fixed:** Redirect with `refund_recorded=1` **only** when `totalCoursePaid > 0`. Otherwise redirect with `cancelled=1` only.

**Files:** `internal/handlers/pre_enrolment.go` (cancel case redirect logic).

**Manual test:**
1. Cancel lead with **no** course payments. Confirm message: “Lead cancelled successfully.” (no “refund recorded”).
2. Cancel lead **with** course payments + refund. Confirm: “Lead cancelled and refund recorded.”

---

## 4. Course payment server-side bounds

**What was wrong:** No server-side checks. Crafted POST could add course payments without offer or exceed remaining balance.

**How fixed:** In SaveFull, before `CreateLeadPayment`:
- Require `existingDetail.Offer.FinalPrice` exists and > 0; otherwise error.
- `GetTotalCoursePaid(leadID)`, `remainingBalance = finalPrice - totalCoursePaid` (min 0).
- Reject if `amount > remainingBalance` or `totalCoursePaid + amount > finalPrice`.

**Files:** `internal/handlers/pre_enrolment.go` (course payment block in SaveFull).

**Manual test:**
1. Lead with no offer. Try to add course payment via form (or crafted POST). Confirm error: offer must be set first.
2. Lead with offer, remaining balance 500. Add amount 600 → error. Add 500 → success.
3. Lead already fully paid. UI disables form; no new payment.

---

## 5. renderDetailWithError missing template context

**What was wrong:** `renderDetailWithError` (used for e.g. mark_ready validation errors) did not pass `StatusDisplayName`, `StatusBgColor`, `StatusTextColor`, `StatusBorderColor`, `ShowFollowUpBanner`, `ShowCancelModal`, etc. Template could break or show wrong UI.

**How fixed:**
- Added `buildDetailViewModel(detail, leadID, userRole)` that returns the shared detail map (placement test, payments, totals, flags, status display, defaults for error/modal/phone).
- `Detail()` uses it, then applies overrides from query (error, success, cancel modal, phone, etc.), then `UpdateLeadStatusFromPayment` when needed, then `PlacementTestPaid` when modal.
- `renderDetailWithError` uses it, then sets `Error` and `SuccessMessage`, and renders.

**Files:** `internal/handlers/pre_enrolment.go` (`buildDetailViewModel`, refactored `Detail`, `renderDetailWithError`).

**Manual test:**
1. Trigger mark_ready validation error (e.g. not fully paid, or missing schedule). Confirm detail page re-renders with error **and** correct status banner, follow-up banner, etc.
2. Normal detail load unchanged. Cancel modal, success messages, phone error still work.

---

## File list

| File | Changes |
|------|---------|
| `internal/handlers/finance.go` | CreateRefund: validate first, use GetTotalCoursePaid, single CreateRefund; drop db import |
| `internal/handlers/pre_enrolment.go` | Cancel: idempotent refund, redirect refund_recorded only when refund; course payment bounds; buildDetailViewModel + Detail/renderDetailWithError refactor |
| `internal/models/repository.go` | Add `CreateCancelRefundIdempotent` |

---

## Quick manual test checklist

- [ ] **1.** Finance refund: create refund → one OUT; use GetTotalCoursePaid.
- [ ] **2.** Cancel paid lead + refund → success; retry cancel → no double refund.
- [ ] **3.** Cancel unpaid lead → “Lead cancelled successfully.” only; cancel paid → “refund recorded.”
- [ ] **4.** Course payment without offer → error; amount > remaining → error; valid payment → OK.
- [ ] **5.** Mark ready validation error → detail page with correct status/banners; normal detail unchanged.
