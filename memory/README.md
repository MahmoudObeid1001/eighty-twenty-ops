# Agent Memory System

**Rule**: Before coding → Read relevant memory docs. If missing/unclear → Update diagrams first. After changes → Update diagrams + rules.

## Purpose

This folder contains evidence-based documentation of the system's actual behavior. Every diagram, flow, and rule is backed by code references - no assumptions or wishful thinking.

## Structure

- **`flows/`** - Mermaid diagrams of key workflows (class lifecycle, mentor workflow, student success, complaints)
- **`db/`** - Database ERD and table index
- **`permissions/`** - RBAC matrix and route access map 
- **`decisions/`** - Business rules currently implemented + open questions

## Evidence-Based Documentation

Every Mermaid diagram includes **Evidence** sections showing:
- File path
- Function/handler name  
- SQL migration (if relevant)

If evidence cannot be found, it's marked as **TODO** with explanation.

## How to Use

1. **When starting a feature**: Read the relevant flow diagram and RBAC matrix
2. **When unsure about behavior**: Check `decisions/business_rules.md`
3. **After implementing changes**: Update the affected diagrams and add evidence
4. **When finding inconsistencies**: Document in `decisions/open_questions.md`

## Key Files

| File | Purpose |
|------|---------|
| [`flows/class_lifecycle.md`](flows/class_lifecycle.md) | Class states and transitions |
| [`flows/mentor_workflow.md`](flows/mentor_workflow.md) | Mentor's view and actions |
| [`flows/student_success_workflow.md`](flows/student_success_workflow.md) | SS absence & follow-up flows |
| [`flows/complaints_workflow.md`](flows/complaints_workflow.md) | Complaint creation → resolution |
| [`db/erd.md`](db/erd.md) | Database schema (core tables) |
| [`permissions/rbac_matrix.md`](permissions/rbac_matrix.md) | What each role can do |
| [`permissions/route_access_map.md`](permissions/route_access_map.md) | Routes → allowed roles |
| [`decisions/business_rules.md`](decisions/business_rules.md) | Implemented rules (evidence-based) |
| [`decisions/open_questions.md`](decisions/open_questions.md) | Inconsistencies & TODOs |
