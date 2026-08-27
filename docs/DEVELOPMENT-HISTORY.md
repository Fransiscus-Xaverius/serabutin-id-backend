# Development History

## Evidence of Prior Development

This document records key development milestones and artifacts for the serabutin-id-backend project to establish evidence of development chronology.

---

## Project Information

| Field | Value |
|-------|-------|
| **Project Name** | serabutin-id-backend |
| **Project Type** | Main backend API service for serabutin.id marketplace |
| **Technology Stack** | Go, Fiber, GORM, PostgreSQL, Redis |

---

## Development Timeline

### Project Conception

| Field | Value |
|-------|-------|
| Project Idea | Serabutin.id marketplace backend API |
| Initial Planning | Early May 2026 |
| Development Start Date | 2026-05-16 |

### Repository Creation

| Field | Value |
|-------|-------|
| Repository Creation Date | 2026-05-16 |
| Initial Commit | 2026-05-16 |
| Initial Commit Message | "Initial commit" |

### First Functional Version

| Field | Value |
|-------|-------|
| First Working Version Date | 2026-05-29 |
| Version 1.0.0 Date | Not yet tagged |
| First Production Deployment | Not yet deployed |

---

## Original Authors

The original authors of serabutin-id-backend are:

| Name | Role | Contribution Period |
|------|------|---------------------|
| Fransiscus Xaverius | Lead Developer | May 2026 - Present |
| Geovann Chandra | Developer | May 2026 - Present |
| Darrell Fiko | Developer | May 2026 - Present |
| Alvin Bernard Wiyono | Developer | May 2026 - Present |
| Ivan Santoso | Developer | May 2026 - Present |

Note: Contributor information from Git history includes: Fransiscus Xaverius, GeovannChandra, Alvin Bernard Wiyono, Frans, mcpe500

---

## Major Milestones

| Date | Milestone | Details |
|------|-----------|---------|
| 2026-05-16 | Initial commit | Repository created with project structure |
| 2026-05-29 | Domain layer completed | Unit tests for errors and token service |
| 2026-05-31 | PII redaction added | Privacy/security utility implemented |
| 2026-06-01 | Auth implementation | Authentication, Docker, audit logs |
| 2026-06-14 | Core APIs | Profile API, image upload, service/booking CRUD |
| 2026-06-17 | Reviews API | Review functionality implemented |
| 2026-06-28 | CI/CD setup | GitHub Actions workflow added |
| 2026-08-27 | Current state | Pre-internship IP protection applied |

---

## Pre-existing Features

The following features existed in the original implementation prior to any academic/internship use:

| Feature | Description | Implementation Date |
|---------|-------------|---------------------|
| Authentication | JWT-based auth with token service | 2026-06-01 |
| User Management | User registration, profile, authentication | 2026-06-01 |
| Service CRUD | Service listing and management | 2026-06-14 |
| Booking System | Booking creation and management | 2026-06-14 |
| Review System | Review and rating functionality | 2026-06-17 |
| Image Upload | MinIO/S3-compatible storage | 2026-06-14 |
| Rate Limiting | Redis-backed request rate limiting | 2026-05-29 |
| Audit Logging | PII redaction and audit trails | 2026-06-01 |
| Database Seeder | Test data generation | 2026-06-14 |

---

## Architecture Documentation

| Document | Date Created | Description |
|----------|-------------|-------------|
| Domain layer structure | 2026-05-16 | Clean Architecture implementation |
| Repository interfaces | 2026-05-16 | Data access abstraction |
| Handler structure | 2026-05-16 | HTTP/WebSocket handlers |

---

## Database Design

| Element | Date Designed | Description |
|---------|--------------|-------------|
| Initial Schema | 2026-05-16 | User, Service, Booking, Review models |
| Token Storage | 2026-05-29 | Redis-based token management |
| Booking Relations | 2026-06-14 | Foreign key relationships |

---

## API Development

| Endpoint Group | Date Implemented | Description |
|----------------|------------------|-------------|
| Auth endpoints | 2026-06-01 | Login, register, token refresh |
| User endpoints | 2026-06-01 | Profile management |
| Service endpoints | 2026-06-14 | Service CRUD operations |
| Booking endpoints | 2026-06-14 | Booking management |
| Review endpoints | 2026-06-17 | Review and rating operations |
| Upload endpoints | 2026-06-14 | Image/media upload |

---

## Evidence Preservation

The following Git history and repository artifacts serve as evidence of prior development:

- **Git Log**: Complete chronological commit history
- **Git Blame**: Original authorship of source files
- **Issue Tracker**: Development planning and problem tracking
- **Pull Requests**: Code review and integration history
- **Tags**: Version markers for releases

---

## How to Preserve Evidence

To maintain the evidentiary value of this development history:

1. **Do not rewrite Git history** — preserve the original commit chronology
2. **Do not squash commits** — maintain individual commit records
3. **Preserve commit metadata** — author, date, and message information
4. **Keep issue history** — maintain issue tracker records
5. **Document retroactively** — add dates to this document as they become known

---

## Notes

- All dates are based on Git commit history
- Development artifacts (emails, design files, meeting notes) should be preserved as additional evidence
- Screenshots and recordings of the application at various stages can serve as supplementary evidence

---

## Last Updated

**Date:** 2026-08-27

**Updated By:** Fransiscus Xaverius
