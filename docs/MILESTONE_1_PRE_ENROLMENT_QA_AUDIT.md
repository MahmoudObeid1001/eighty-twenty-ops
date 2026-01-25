# Milestone 1 (Pre-Enrolment) — QA + System Audit

**Scope:** Pre-Enrolment flows, payments/refunds, lead status, ledger, scheduling.  
**Purpose:** Verify correctness, safety, and readiness to proceed. No new features.

---

## Step 1 — Scenario Generation

### Normal flows

1. **Create lead → Book test → Mark tested → Set offer → Add course payment → Mark ready → Send to classes**  
   Admin creates lead (name, phone), books placement test (date/time/type), marks tested, sets bundle + final price, adds course payment(s) up to remaining balance, sets class days/time when fully paid, marks ready, sends to classes. Expect: status progression, ledger IN for placement test + course payments, schedule persisted.

2. **Cancel lead with no course payments**  
   Admin opens lead, clicks Cancel Lead, gets modal “no refund needed”, confirms. Expect: status → cancelled, no refund transaction, success “Lead cancelled successfully.”

3. **Cancel lead with course payments + refund**  
   Admin opens paid lead, Cancel Lead, modal shows refund form. Enters amount ≤ total course paid, method, date (today or past), confirms. Expect: refund OUT transaction created, lead cancelled, “Lead cancelled and refund recorded.”

4. **Move to waiting list (paid or unpaid)**  
   Admin clicks “Move to Waiting List”. Expect: status → waiting_for_round, no refund, payments unchanged.

5. **Placement test payment + finance sync**  
   Admin sets placement test fee paid, date, method, saves. Expect: placement test stored, UpsertPlacementTestIncome runs, IN transaction category placement_test, ledger updated.

6. **Course payment with offer set**  
   Admin sets offer final price, adds course payment (amount ≤ remaining balance), saves. Expect: CreateLeadPayment, IN transaction category course_payment, total course paid ≤ final price, status may move to paid_full when fully paid.

### Partial actions

7. **Save without placement test**  
   Admin fills basic info only, saves. Expect: lead updated, no placement test, no finance sync.

8. **Save placement test only (no offer)**  
   Admin updates placement test payment, leaves offer unchanged. Expect: placement test updated, offer and status unchanged (no implicit OFFER_SENT).

9. **Mark ready without schedule**  
   Admin clicks “Mark Ready to Start” but does not set class days/time. Expect: validation error, “Both Class Days and Class Time are required,” detail page re-rendered with error.

10. **Mark ready without full payment**  
    Admin tries “Mark Ready” before course fully paid. Expect: “Cannot mark READY_TO_START before full payment.”

### Mistakes & retries

11. **Cancel with refund amount &gt; total course paid**  
    Admin enters refund &gt; total course paid, submits. Expect: redirect with `error=amount_exceeds`, modal shown again, no refund created, lead not cancelled.

12. **Cancel with future refund date**  
    Admin sets refund date in future. Expect: validation error, redirect `error=future_date`, no refund, no cancel.

13. **Course payment with future date**  
    Admin submits course payment with future date. Expect: “Payment date cannot be in the future,” payment not created.

14. **Duplicate phone on create/update**  
    Admin sets phone to existing lead’s. Expect: PHONE_ALREADY_EXISTS, friendly message, form values preserved, “Open existing lead” link when `existing_lead_id` returned.

15. **Retry cancel after CreateRefund succeeds but CancelLead fails**  
    Refund created, then cancel fails (e.g. DB error). Admin retries cancel. Current behavior: CreateRefund called again → **double refund**. Invariant violation.

### Edge cases (money, status, scheduling)

16. **Fully paid → refund → status revert**  
    Lead paid in full, refund created. Expect: UpdateLeadStatusFromPayment runs, status reverts to offer_sent when total course paid &lt; final price; cancelled leads ignored.

17. **Schedule section locked until fully paid**  
    Lead not fully paid. Expect: schedule (class days/time) disabled/dimmed, “Locked until course is fully paid.” Submitting other sections must not clear existing schedule.

18. **Class time persist (Save)**  
    Admin sets class time (e.g. 07:30), saves. Expect: scheduling upserted with TO_CHAR/::TIME handling, reload shows same time.

19. **mark_ready vs Save scheduling**  
    mark_ready uses UpsertSchedulingClassDaysTime (class_days, class_time only). Save uses UpdateLeadDetail (full scheduling upsert). Both write to `scheduling`. No transactional coordination; last write wins.

20. **Course payment without offer_final_price**  
    UI disables course payment when no final price. Backend CreateLeadPayment does **not** check offer_final_price or remaining balance. Crafted POST can add payment without offer or exceed remaining balance → **invariant gap**.

---

## Step 2 — Invariants Extraction

**Must never be violated:**

### Payments & refunds

- **I1.** `total_course_paid` = sum(lead_payments) − sum(refunds for lead). No negative net.
- **I2.** Refund amount ≤ `total_course_paid` at creation time. Enforced in CreateRefund; handler also checks before CreateRefund.
- **I3.** Placement test money is **not** refundable. Refunds apply only to course payments.
- **I4.** All payment/refund dates must be ≤ today (no future). Enforced via `ValidateNotFutureDate` for placement test, course payment, cancel refund.
- **I5.** Course payments: total course paid ≤ `offer_final_price`. **Not** enforced server-side in CreateLeadPayment or handler. UI only.
- **I6.** Course payments only when `offer_final_price` set. **Not** enforced server-side.
- **I7.** Ledger: every placement test payment (paid &gt; 0) syncs to IN transaction (category placement_test); every course payment to IN (course_payment); every refund to OUT (refund). Idempotent keys for placement test; refund/course payment create new rows.

### Lead status

- **I8.** READY_TO_START only if: fully paid **and** assigned level **and** class_days **and** class_time. Enforced in mark_ready.
- **I9.** Schedule (class days/time) writable only when fully paid. Enforced in Save; UI locks otherwise.
- **I10.** WAITING (move to waiting list) allowed regardless of course payments. No refund created.
- **I11.** CANCELLED: soft cancel only. No hard delete. Refund required when `total_course_paid` &gt; 0; must create refund before cancel.
- **I12.** Status auto-update: when `total_course_paid` ≥ `offer_final_price` → paid_full; when refund drops total below → offer_sent. Cancelled leads excluded.

### Ledger integrity

- **I13.** IN = placement_test, course_payment (etc.); OUT = refund, expenses. Categories consistent.
- **I14.** `transactions.ref_key` unique where set. Prevents duplicate sync for placement test.
- **I15.** CreateLeadPayment: insert `lead_payments` **and** `transactions` IN. CreateRefund: insert `transactions` OUT only. No orphan payment rows.

### Schedule

- **I16.** Class days ∈ {Sun/Wed, Sat/Tues, Mon/Thu}; class time ∈ {07:30, 10:00}. Enforced in handler.
- **I17.** Both class_days and class_time required when setting **new** schedule; partial update allowed when schedule already exists.

### Cancel flow

- **I18.** Cancel + refund must be atomic: either both succeed or neither. **Currently violated:** CreateRefund then CancelLead, no transaction. Retry after CancelLead failure → double refund.
- **I19.** When `total_course_paid` == 0, cancel must **not** create refund. Correct. But redirect always adds `refund_recorded=1` → **incorrect success message** (“refund recorded” when none).

---

## Step 3 — Regression Risk Scan

### Fragile areas

1. **Cancel flow (action=cancel)**  
   - CreateRefund then CancelLead; no DB transaction. **Risk:** Refund persisted, cancel fails → retry → double refund.  
   - Redirect always `cancelled=1&refund_recorded=1`. **Risk:** “Lead cancelled and refund recorded” shown even when no refund (totalCoursePaid == 0).

2. **Finance refund handler (`POST /finance/refund/{id}`)**  
   - Calls `CreateRefund` **twice** (lines 295 and 336). Validation (amount ≤ totalPaid − totalRefunded) runs **after** first CreateRefund. **Risk:** Double refund on every successful request.  
   - Uses `detail.Payment.AmountPaid` (legacy) for “total paid” instead of `GetTotalCoursePaid`. **Risk:** Wrong base when using lead_payments; inconsistent with cancel flow.

3. **renderDetailWithError**  
   - Used for mark_ready validation (e.g. schedule required, not fully paid). Does **not** pass `StatusDisplayName`, `StatusBgColor`, `StatusTextColor`, `StatusBorderColor`, `ShowCancelModal`, `ShowFollowUpBanner`, etc. **Risk:** Template uses these; missing keys → empty/zero values, broken status banner or modal behavior.

4. **Course payment creation (Save)**  
   - Handler requires type, amount, method, date. **No** check for `offer_final_price` or `remaining_balance`. CreateLeadPayment does not enforce them. **Risk:** Overpayment or payment without offer via crafted POST; ledger/logic inconsistency.

5. **Scheduling persistence**  
   - `mark_ready` uses `UpsertSchedulingClassDaysTime` (class_days, class_time as plain strings). `UpdateLeadDetail` uses `::TIME` for class_time. **Risk:** Type/casting drift; `UpsertSchedulingClassDaysTime` might behave differently with TIME.  
   - Save preserves schedule from existing detail when form omits it (e.g. disabled). Relies on hidden inputs and “preserve if empty” logic. **Risk:** Form or layout changes could drop schedule.

### Implicit assumptions

6. **Single form / action coupling**  
   - Update handler switches on `action`; SaveFull shares form with status actions. Implicit “update only provided sections” via `shouldProcessOffer`, etc. **Risk:** New actions or form fields can unintentionally toggle offer processing or overwrite sections.

7. **Detail vs list payment state**  
   - List uses `GetAllLeads` + `ComputeLeadFlags` (amount_paid, final_price from payment/offer). Detail uses `GetTotalCoursePaid` + offer. **Risk:** List “payment state” can diverge from detail if legacy payment vs lead_payments disagree.

8. **Moderator vs admin**  
   - Moderators cannot update status, cancel, send to classes, etc. Assumed enforced only in handlers. **Risk:** Any direct API or alternate handler bypass could skip checks.

### Future-change risks

9. **New finance or payment flows**  
   - Adding new payment types or refund paths without reusing CreateRefund/CreateLeadPayment or respecting GetTotalCoursePaid could break I1–I7, I13–I15.

10. **Status or stage enums**  
    - `MapOldStatusToStage`, `ComputeStageFromFormCompletion`, and UI filters depend on current status set. New statuses or renames can break mapping, filters, or “hot” logic.

11. **Template data contracts**  
    - Detail template expects many keys (Detail, Today, LeadPayments, FinalPrice, IsFullyPaid, StatusDisplayName, ShowCancelModal, etc.). Error paths (e.g. renderDetailWithError) pass a subset. **Risk:** Template evolution or new blocks that assume full Detail data can panic or misrender.

---

## Step 4 — Milestone Verdict

### **Milestone 1 Status: Blocking Issues Fixed + Moderator UX Hardened**

**All blocking issues have been resolved:**

1. ✅ **Finance refund handler double CreateRefund — FIXED**  
   `POST /finance/refund/{leadID}` now validates all inputs (amount, payment method, date, amount against `GetTotalCoursePaid`) **before** any database operation. `CreateRefund` is called exactly once. Uses `GetTotalCoursePaid` as the source of truth.

2. ✅ **Cancel + refund not atomic — FIXED**  
   Cancel flow now uses `CreateCancelRefundIdempotent` with deterministic `ref_key="cancel_refund:<leadID>:<date>:<amount>"`. Retries do not double-create refunds. Uses `ON CONFLICT (ref_key) DO NOTHING` for idempotency.

3. ✅ **Cancel success message when no refund — FIXED**  
   Redirect URL now conditionally includes `refund_recorded=1` **only** when `totalCoursePaid > 0` and a refund was actually created. Otherwise, only `cancelled=1` is included.

4. ✅ **Course payment server-side bounds — FIXED**  
   Handler now enforces: `offer_final_price` must exist and be > 0, payment amount must be > 0 and ≤ `remaining_balance`, and `total_course_paid` must never exceed `offer_final_price`. Validation occurs before any database operation.

5. ✅ **renderDetailWithError missing template data — FIXED**  
   Introduced `buildDetailViewModel` helper that centralizes all detail page data (status colors, banners, modal flags, financial totals, etc.). Both `Detail` and `renderDetailWithError` use this shared helper, ensuring consistent template context.

---

## Step 5 — Moderator UX + Permissions Hardening (Post-Audit)

**Additional improvements applied after blocking fixes:**

### Moderator Role Enhancements

**Permissions:**
- ✅ Moderators can **create leads** (full name, phone, source, notes)
- ✅ Moderators can **view leads** (list and detail pages)
- ✅ Moderators can **edit basic lead info only** (name, phone, source, notes) to fix mistakes
- ❌ Moderators **cannot** delete leads (Delete button hidden; backend rejects `action=delete`)
- ❌ Moderators **cannot** see or change payments, offers, bundles, pricing, refunds, ledger, payment methods
- ❌ Moderators **cannot** change lead status (mark tested/offer sent/ready/etc.)
- ❌ Moderators **cannot** access Classes or Finance pages (custom 403 access-restricted page shown)

**UI Changes:**
- **Pre-Enrolment List:** Delete and Send to Classes buttons hidden for moderators
- **Pre-Enrolment Detail:** "Moderator Mode: Limited Edit" — only Lead Info section editable; all other sections (Placement Test, Offer & Pricing, Booking, Course Payment, Schedule, Shipping, Refund, Cancel, status actions) are hidden
- **Access-Restricted Pages:** Custom styled HTML pages for `/classes` and `/finance` with friendly message and "Back to Pre-Enrolment" link
- **Navigation:** Classes and Finance links hidden in sidebar for moderators

**Backend Changes:**
- `IsModerator` / `IsAdmin` helpers added (`internal/handlers/role.go`)
- Moderator save flow only processes `full_name`, `phone`, `source`, `notes`; all other fields ignored
- Phone uniqueness validation with friendly error messages
- Delete action explicitly rejected for moderators (403 Forbidden)

**Documentation:**
- `docs/MODERATOR_UX_MANUAL_CHECKLIST.md` — manual QA checklist for moderator behavior
- `docs/BLOCKING_FIXES_DELIVERABLE.md` — detailed explanation of blocking fixes

---

**Summary:** All five blocking issues from the QA audit have been resolved. The system now includes robust moderator role permissions with clear UI/UX boundaries. Milestone 1 is production-ready with proper validation, idempotency, and role-based access control.
