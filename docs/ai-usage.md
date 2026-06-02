# AI Usage Statement

- Version: 0.2
- Date: 2026-05-27
- Status: Sealed
- Owner: zqw
- Audience: Hackathon reviewers

## 1. Purpose

This document explains how AI was used during first-stage requirement analysis, product design, technical design, evidence collection, Spec writing, and red-team response.

This document is a hackathon review deliverable. It is not an instruction document for coding agents. Coding agents should use `AGENTS.md`, `docs/master-spec.md`, `docs/evidence-summary.md`, current phase specs, and referenced design documents.

## 2. AI Roles

| Role | Usage |
|---|---|
| ChatGPT | Requirement clarification, UPMIO/ClickHouse architecture explanation, Spec structure design, Codex prompt design, red-team report interpretation, and manual document correction guidance |
| Codex | Kubernetes environment preparation, UPMIO repository discovery, runtime validation, and initial documentation fixes |
| Red-team reviewer | Independent review against the Phase Spec Driven Development guide and hackathon deliverable requirements |
| User / human owner | Final decisions, scope control, security baseline, MVP selection, and acceptance judgment |

## 3. AI Usage by Stage

| Stage | AI Usage | Output | Human Validation |
|---|---|---|---|
| Requirement understanding | ChatGPT helped interpret the hackathon requirement and ClickHouse Day1/Day2 scope | Initial project framing and questions | User corrected scope and clarified that Day1/Day2 are project scope, not optional features |
| Product design | ChatGPT proposed product positioning, roles, user journey, and page concepts | Product design draft | User required a concise Master Spec and separate design docs |
| Technical design | ChatGPT explained UPMIO layers, ClickHouse HA topology, UnitSet/GrpcCall boundaries, and monitoring choices | Architecture/design drafts | User decided to prioritize `upm-packages/clickhouse` and reject insecure `no_password` |
| Discovery evidence | Codex inspected UPMIO repositories and generated discovery reports | `docs/discovery/` | User and ChatGPT reviewed conclusions and identified runtime validation gaps |
| Runtime validation | Codex installed UPMIO operators and tested UnitSet, Keeper, ClickHouse Server, monitoring, and GrpcCall | `docs/runtime/` | ChatGPT summarized evidence and user decided MVP vs roadmap boundaries |
| Spec writing | ChatGPT generated Master Spec, design docs, phase specs, and AGENTS drafts | `docs/` package | User reviewed and requested multiple structure changes |
| Red-team response | ChatGPT interpreted red-team report and generated a repair plan; Codex applied initial fixes; ChatGPT performed manual second review | repaired docs package | User requested final review and English translation |

## 4. Key Prompt Types

| Prompt Type | Goal | Guardrails | Main Output |
|---|---|---|---|
| Discovery Prompt | Inspect UPMIO repositories, CRDs, MySQL implementation, monitoring capability | No feature code; evidence-based output only | `docs/discovery/*.md` |
| Runtime Validation Prompt | Install and validate UPMIO operators, UnitSet behavior, ClickHouse Keeper/Server, monitoring, GrpcCall | Do not implement manager; record failures honestly | `docs/runtime/*.md` |
| Red-team Fix Prompt | Repair first-stage Spec gaps from red-team report | Documentation only; do not modify raw evidence | new/updated Spec, design, phase, and AGENTS files |
| Phase Spec Prompt | Turn project plan into executable phase contracts | Keep scope bounded; include acceptance and verification | `docs/phases/*.md` |

## 5. Human Decisions

The following decisions were made by the user/human owner, not blindly accepted from AI output:

| Decision | Human Rationale |
|---|---|
| Do not start coding before Spec is ready | The first-stage assessment focuses on Spec quality |
| Use Go for Manager Backend | Aligns with Kubernetes and UPMIO ecosystem |
| Servers must only install Kubernetes prerequisites | Application components must run inside Kubernetes |
| Treat existing ClickHouse package as incomplete reference asset | Runtime validation showed template and metrics gaps |
| Fix `upm-packages/clickhouse` first | Package fixes are required before higher-level workflows |
| Reject `<no_password/>` | Production baseline must use Secret-based password initialization |
| Use ClickHouse native Prometheus endpoint for MVP | Simpler than exporter sidecar and compatible with kube-prometheus-stack |
| Keep Prometheus/Grafana external | UPMIO public repos provide integration points, not full stack |
| Exclude ClickHouse GrpcCall from MVP | Runtime image rejected `type=clickhouse` |
| Use 1 shard x 2 replicas as first executable baseline | Validated runtime path; multi-shard remains roadmap |
| Keep N shards x M replicas in project scope | Required for product-grade ClickHouse architecture |
| Keep AI usage as reviewer document, not coding-agent input | Coding agents need specs and phase contracts, not review narrative |

## 6. AI Output Validation

AI output was not accepted as evidence by itself.

Accepted evidence includes:

- command output;
- file paths and repository inspection;
- Kubernetes runtime state;
- ClickHouse SQL output;
- Prometheus/PodMonitor validation results;
- red-team checklist results;
- human review decisions.

Runtime evidence overrides source-code assumptions.

## 7. Limitations

- Private or enterprise UPMIO components were not directly validated.
- Runtime validation used the available public operator/package images and locally imported artifacts.
- ClickHouse GrpcCall support may exist in newer source/image combinations, but was not verified at runtime.
- This document records how AI supported the first-stage Spec. It is not an implementation contract.
