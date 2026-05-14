# Lead And Student Journey

**Purpose**: One readable file for the full journey from first lead creation to student lifecycle, round close, renewal, waiting list, and cancellation/refund paths.

**Primary Evidence Sources**:
- [internal/handlers/pre_enrolment.go](/Users/mahmoud/Downloads/eighty-twenty-ops/internal/handlers/pre_enrolment.go)
- [internal/models/repository.go](/Users/mahmoud/Downloads/eighty-twenty-ops/internal/models/repository.go)
- [memory/decisions/business_rules.md](/Users/mahmoud/Downloads/eighty-twenty-ops/memory/decisions/business_rules.md)
- [memory/flows/class_lifecycle.md](/Users/mahmoud/Downloads/eighty-twenty-ops/memory/flows/class_lifecycle.md)
- [memory/flows/student_success_workflow.md](/Users/mahmoud/Downloads/eighty-twenty-ops/memory/flows/student_success_workflow.md)

---

## 1. Big Picture

There are really **two connected journeys**:

1. **Lead journey**: ops pipeline from new inquiry to paid and ready for class.
2. **Student journey**: class participation, class close, then either renewal payment or waiting-list continuation.

The system also has **special branches** for:
- sleeping leads
- cold leads
- refused renewal
- cancellation + refund
- returning students with carried credits

---

## 2. Main Status Journey

### New lead

```text
lead_created
  -> test_booked
  -> tested
  -> offer_sent
  -> booking_confirmed / deposit_paid / paid_full
  -> waiting_for_round
  -> schedule_assigned
  -> ready_to_start
  -> in_classes
```

### Returning student after class close

```text
in_classes
  -> class closes
  -> renewal_pending     if student needs a new payment cycle
  -> waiting_for_round   if student already has prepaid entitlement/credits
```

From there:

```text
renewal_pending
  -> offer_sent
  -> waiting_for_round
  -> schedule_assigned
  -> ready_to_start
  -> in_classes
```

or

```text
renewal_pending / offer_sent
  -> cold_lead          if refused renewal
```

---

## 3. New Lead Journey

### Step 1: Lead is created

- Lead enters pre-enrolment as `lead_created`.
- Basic info is saved: name, phone, source, notes.
- At this point, they are only a lead, not yet a student.

### Step 2: Placement test is booked

- Ops sets test date, time, and type.
- Status becomes `test_booked`.
- Placement test fee is part of the flow.
- Current default placement test fee is **60 EGP**.

### Step 3: Placement result is recorded

- Student Success records assigned level and notes.
- Status becomes `tested`.
- This is the point where the lead is academically classified.

### Step 4: Offer is prepared and sent

- Ops selects bundle and pricing track.
- Status becomes `offer_sent`.
- This is the commercial offer stage.

### Step 5: Payment is recorded

- Real course money is stored through `lead_payments`.
- Matching finance rows are stored in `transactions`.
- System uses **real recorded payments**, not just form values, as source of truth.

Possible payment meanings:
- `deposit_paid`
- `paid_full`
- still unpaid but offer exists

### Step 6: Waiting list

- Once payment/entitlement is sufficient, student moves to `waiting_for_round`.
- At this stage the student is operationally ready, but still not in an active class.

### Step 7: Schedule set

- Ops sets class days and class time.
- Status can move through `schedule_assigned` then `ready_to_start`.

### Step 8: Joins class

- Once assigned into an actual class flow, lead becomes `in_classes`.
- This is the handoff from lead funnel into student lifecycle.

---

## 4. Student Journey Inside Classes

### Class preparation

- Class groups are created by Ops.
- Classes can be sent to Mentor Head.
- Mentor Head assigns mentor.
- Round is started.

### Active class

- Student attends sessions.
- Attendance, tasks, participation, and grading are tracked.
- Student Success handles absence follow-up and placement-test/result support.

### Session-based business effects

- A class has 8 sessions.
- Session 1 completion is important because credit consumption starts to matter for lifecycle/refund rules.
- Active students are considered true students when status is `in_classes`.

---

## 5. Round Close And Post-Class Journey

When Mentor Head closes the round:

- Each student receives an outcome:
  - `promoted`
  - `repeated`
- Class enrollment history is written.
- Current offer/payment snapshot is cleared for the next cycle.
- Lead becomes a **returning student**.

Then the system decides:

### Case A: Student already has credit

- Status becomes `waiting_for_round`.
- Reason: next level is already prepaid or entitlement still exists.

### Case B: Student needs a new payment cycle

- Status becomes `renewal_pending`.
- Reason: they need renewal offer/payment to continue.

---

## 6. Renewal Journey

For `renewal_pending` students:

### Message stage

The system can surface renewal templates based on:
- latest level
- promoted vs repeated
- attendance pattern

The message bank now includes:
- `Renewal Pending`
- `Refused Renewal Message`
- `Sleeping Leads`
- `After Placement Test`

### Commercial renewal stage

- Ops saves new offer.
- Status becomes `offer_sent`.
- Payment is collected as a new current-cycle payment.
- Then student returns to:
  - `waiting_for_round`
  - `schedule_assigned`
  - `ready_to_start`
  - `in_classes`

### Refused renewal branch

If returning student refuses renewal:
- lead can be marked `refused_renewal`
- status moves to `cold_lead`
- follow-up continues through refused-renewal message bank

---

## 7. Waiting List Journey

`waiting_for_round` means:

- the student is not lost
- they are not unpaid in the normal sense
- they are waiting for actual batch formation

Typical reasons:
- student already prepaid through previous bundle
- class not formed yet
- schedule still being grouped

From here:
- Ops sets schedule
- status becomes `ready_to_start`
- then student is inserted into class roster
- then becomes `in_classes`

---

## 8. Cold, Sleeping, And Disappeared Leads

These are separate follow-up journeys, not the normal happy path.

### Sleeping leads

Used for leads who engaged early but disappeared before progressing.

### After placement test

Used for leads who:
- reached `offer_sent`
- then disappeared without paying

### Cold lead

Used when:
- lead is intentionally marked cold
- or returning student refuses renewal

These paths are marketing/retargeting branches, not core class operations.

---

## 9. Cancellation And Refund Journey

Leads/students can be cancelled.

Important rules:

- Cancellation is not just a visual status change.
- Refund logic must be checked.
- Refunds are written as `OUT` transactions in finance.

### Refund logic depends on context

The system distinguishes between:
- current-cycle paid cash
- carryover/unused credit value
- consumed vs unused levels

### Updated pricing rules

- Placement test default fee: **60 EGP**
- Group Track single level base price: **1250 EGP**

### Refund valuation sync

The unused-credit valuation and consumed-level valuation now use **1250 EGP** instead of the old **1300 EGP**.

This matters for:
- returning students
- unused credits refund calculation
- consumed-level deduction during cancellation/refund valuation

---

## 10. Finance Touchpoints In The Journey

There are three important money views:

### Gross student receipts

- Course payments + placement test payments collected from students.

### Net student receipts

- Gross student receipts minus refunds.

### Current cash

- Full ledger cash after:
  - student inflows
  - refunds
  - salaries
  - software
  - other expenses
  - manual/opening balance adjustments

This is why:
- `net student receipts`
and
- `current cash`

are not the same number.

---

## 11. Source Of Truth Notes

### Payment truth

The real source of truth for course money is:
- `lead_payments`
- synced into `transactions`

Old `payments.amount_paid` is legacy and should not be trusted for finance reconciliation.

### Returning-student truth

Returning students are special:
- old snapshots are cleared after class close
- current-cycle money is separated from previous-cycle money
- carried credits are handled explicitly

### Class roster truth

Operationally, a real class student is primarily controlled by:
- class assignment data
- scheduling match
- `status = in_classes`

---

## 12. Practical Journey Summary

### New lead summary

```text
lead_created
-> test_booked
-> tested
-> offer_sent
-> paid / entitled
-> waiting_for_round
-> ready_to_start
-> in_classes
```

### Student summary after classes

```text
in_classes
-> round closed
-> promoted or repeated
-> waiting_for_round OR renewal_pending
-> offer_sent if renewal needed
-> paid / scheduled
-> ready_to_start
-> in_classes again
```

### Failure/retarget branches

```text
offer_sent -> cold_lead / after placement test follow-up
renewal_pending -> refused_renewal -> cold_lead
early inactive lead -> sleeping leads
any valid case -> cancelled + refund flow
```

---

## 13. Suggested Use Of This File

Use this file when you need to explain:
- where a lead currently sits
- why a returning student became `renewal_pending` vs `waiting_for_round`
- why finance numbers differ from “student money collected”
- what happens after class close
- which branch is operational vs retargeting vs finance

