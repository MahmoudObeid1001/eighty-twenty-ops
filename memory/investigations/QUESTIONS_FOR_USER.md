# Business Logic Questions - Need Your Input

**Purpose**: Remaining questions about system behavior that require your clarification. Please fill in your answers below.

---

## Community Officer & HR Workflows

### Q1: Community Officer - what is their complete workflow?

**What we know**:
- Table: `community_officer_feedback` exists
- Purpose: Submit feedback for sessions 4 & 8
- Route: `/community-officer` dashboard exists

**Questions for you**:
1. What exactly does the Community Officer do in their daily workflow?
2. When/how do they submit feedback?
3. Who sees their feedback? What happens with it?
4. Are there any other CO responsibilities not captured in code yet?

**Your Answer**:
```
[Please describe the Community Officer workflow]
```

---

### Q2: HR Role - what is their complete workflow?

**What we know**:
- Route: `/app/hr/mentors` exists
- Purpose appears to be HR operations for mentors

**Questions for you**:
1. What does HR do in the system?
2. What information do they view/edit?
3. Are they involved in hiring, evaluations, payroll, or other HR tasks?
4. Is this role actively used or planned for future?

**Your Answer**:
```
[Please describe the HR workflow]
```

---

## Attendance & Escalation

### Q3: Are there escalation rules we should implement?

**What we know**: Currently no automatic escalation thresholds

**Questions for you**:
1. Should there be automatic alerts for X absences?
2. What are the escalation thresholds you want? (e.g., 2 absences = warning, 3 = escalate)
3. Who gets notified at each level?
4. Should this be hardcoded or configurable?

**Your Answer**:
```
[Please describe desired escalation rules]
```

---

### Q4: Attendance marking - is there a deadline policy?

**What we know**: System allows marking attendance anytime (past/future sessions)

**Questions for you**:
1. Should mentors have a deadline to mark attendance?
2. If yes, how long after session? (e.g., 24 hours, 48 hours)
3. What happens if they miss the deadline?
4. Or is flexible attendance marking intentional?

**Your Answer**:
```
[Please describe attendance deadline policy]
```

---

## Complaints System

### Q5: Complaint categories - what are the valid values?

**What we know**: Database allows any text, no constraints

**Questions for you**:
1. What complaint categories should Student Success be able to select?
2. Examples: mentor, content, technical, admin, scheduling, student_behavior, other?
3. Are these fixed or should they be configurable?

**Your Answer**:
```
[Please list complaint categories]
```

---

### Q6: Complaint urgency levels - what are the valid values?

**Questions for you**:
1. What urgency levels exist? (e.g., low, medium, high, critical)
2. Do different urgency levels trigger different workflows?
3. Who decides urgency - SS or system?

**Your Answer**:
```
[Please list urgency levels]
```

---

### Q7: Complaint escalation - are there multiple tiers?

**What we know**: Currently single-tier (SS creates → MH handles)

**Questions for you**:
1. Should some complaints escalate beyond Mentor Head?
2. If yes, what determines escalation? (category? urgency? time?)
3. Who would be the next tier? (Admin? External support?)
4. Or is single-tier sufficient?

**Your Answer**:
```
[Please describe complaint escalation tiers]
```

---

## Grades Feature

### Q8: Grades - is this feature planned or abandoned?

**What we know**: `grades` table exists but no API/UI implemented

**Questions for you**:
1. Should we implement grade tracking?
2. If yes, who enters grades? (Mentor? Mentor Head?)
3. What grades are tracked? (quiz, homework, final, overall?)
4. Who can view grades?
5. Or should we remove the table as unused?

**Your Answer**:
```
[Please clarify grades feature status]
```

---

## Finance Module

### Q9: Finance module - should we expand documentation?

**What we know**: Finance migrations exist but not fully documented in memory system

**Questions for you**:
1. Is finance module actively used?
2. What are the key workflows we should document?
3. Priority: high, medium, or low for expanding finance docs?

**Your Answer**:
```
[Please describe finance documentation priority]
```

---

## Architecture Decisions

### Q10: SSR vs React - what's the long-term plan?

**What we know**: System currently has both SSR templates and React SPA

**Questions for you**:
1. Is dual architecture intentional long-term?
2. Should new features be SSR or React? (or case-by-case?)
3. Any plans to migrate everything to React?
4. Or keep SSR for admin, React for dashboards?

**Your Answer**:
```
[Please describe architecture direction]
```

---

## Submit Your Answers

Once you've filled in your answers above, I'll:
1. Update `memory/decisions/business_rules.md` with confirmed rules
2. Remove resolved questions from `memory/decisions/open_questions.md`
3. Create new documentation for workflows you describe (CO, HR, etc.)
4. Update relevant workflow diagrams with your clarifications

**Please save this file with your answers and let me know when complete!**
