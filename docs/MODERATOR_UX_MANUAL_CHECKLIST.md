# Moderator UX + Permissions — Manual Checklist

Use this checklist when verifying moderator role behavior. No new business features; UI + backend permission hardening only.

---

## 1. Pre-Enrolment List

- [ ] **Moderator cannot see Delete**  
  Log in as moderator → go to `/pre-enrolment`. Confirm **Delete** button is **hidden** for all leads. **Open** and **New Lead** remain visible.

- [ ] **Send to Classes hidden for moderator**  
  As moderator, **Send to Classes** is not shown in the list (including for READY leads).

- [ ] **Admin unchanged**  
  Log in as admin → list shows Delete, Send to Classes (when applicable) as before.

---

## 2. Pre-Enrolment Detail — Moderator Mode

- [ ] **Moderator can edit only name, phone, source, notes**  
  As moderator, open a lead. Confirm:
  - **Moderator Mode: Limited Edit** banner is shown.
  - **Lead Info** (full name, phone, source, notes) is **editable**.
  - **Save** button is present and working.

- [ ] **Edits persist**  
  Change phone/name/source/notes → Save. Reload page. Values are updated.

- [ ] **Moderator cannot see payments, offers, etc.**  
  As moderator, confirm the following are **hidden**:
  - Placement Test (fee, payment, test date/time/type/level/notes)
  - Offer & Pricing (bundle, base/final price, discount)
  - Booking & Materials
  - Course Payment (summary, add payment, payment history)
  - Round / Schedule (class days, class time)
  - Shipping
  - Refund, Cancel Lead, Send to Classes
  - All status action buttons (Mark Tested, Mark Offer Sent, Move to Waiting List, Mark Ready)

- [ ] **Simple status label only**  
  Status banner (e.g. “Offer Sent”, “Tested”) may still be shown. No money details (e.g. amounts, paid in full breakdown) for moderator.

- [ ] **Admin unchanged**  
  As admin, detail page shows all sections and actions as before.

---

## 3. Backend — Moderator Restrictions

- [ ] **Moderator cannot delete lead (including direct POST)**  
  As moderator, POST to `/pre-enrolment/{id}` with `action=delete` (e.g. via devtools or curl). Expect **403 Forbidden**; lead is **not** deleted.

- [ ] **Moderator save only updates basic fields**  
  As moderator, submit form with only `full_name`, `phone`, `source`, `notes` changed. Confirm only those fields change in DB. Other fields (offer, placement test, schedule, etc.) are **unchanged**.

- [ ] **Phone uniqueness**  
  As moderator, change phone to one that already exists → friendly error (e.g. “Phone number already exists”), “Open existing lead” link if available; no raw SQL error.

---

## 4. Access Denied (Classes / Finance)

- [ ] **Moderator visiting /classes**  
  Log in as moderator → go to `/classes`. Expect:
  - **403** status.
  - **Styled HTML** page (site layout, sidebar).
  - Message: “Access Restricted: This section (Classes Board) is available to administrators only.”
  - **Back to Pre-Enrolment** button/link.

- [ ] **Moderator visiting /finance**  
  Same as above for `/finance`, with “Finance” in the message.

- [ ] **No plain “Forbidden: Insufficient permissions”**  
  Moderators must **not** see raw text 403; always the custom access-restricted page.

- [ ] **Admin unchanged**  
  Admin can access `/classes` and `/finance` as before.

---

## 5. Nav + Layout

- [ ] **Classes / Finance hidden in sidebar for moderator**  
  As moderator, sidebar shows **Pre-Enrolment** only (plus disabled Learning/Reports). **Classes** and **Finance** links are **hidden**.

- [ ] **Admin unchanged**  
  As admin, sidebar still shows Classes and Finance.

- [ ] **Layout/logo/sidebar design unchanged**  
  No visual or structural changes to layout, logo, or sidebar beyond hiding nav items for moderator.

---

## Quick smoke

1. Moderator: list → no Delete; detail → edit name/phone/source/notes → Save → persists.
2. Moderator: detail → no payments/offers/schedule/status actions.
3. Moderator: `/classes` and `/finance` → 403 + friendly access-restricted page + Back to Pre-Enrolment.
4. Admin: list, detail, Classes, Finance behave as before.
