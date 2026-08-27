# serabutin-id-backend

Main backend API service for [Serabutin.id](https://serabutin.id) — marketplace lokal untuk jasa dan pekerjaan serabutan.

## Tech Stack

- **Go 1.25** with Fiber v3
- **PostgreSQL** (GORM) for persistent storage
- **Redis** for caching and rate limiting
- **JWT** for authentication
- **MinIO** for object storage

## Quick Start (Docker Compose)

```bash
docker compose up -d
```

The API will be available at the port configured in `docker-compose.yml`.

## Local Development

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- PostgreSQL and Redis (via Docker)

### Setup

1. Start infrastructure:

```bash
docker compose up -d
```

2. Configure environment:

```bash
cp .env.example .env
```

3. Run the server:

```bash
go run ./cmd/server
```

### Build

```bash
go build ./cmd/server
```

### Tests

```bash
go test ./...
```

---

## License & Intellectual Property

This project is proprietary software and is **NOT** distributed under an open-source license.

**Copyright © 2026 Fransiscus Xaverius. All rights reserved.**

### Who May Use This Software

This software may only be used by persons or organizations who have received explicit written permission from the copyright holder.

### Permitted Use

With proper authorization, the software may be used for:
- Academic research purposes
- Internship-related activities under authorized programs
- Evaluation for authorized academic or research purposes

### Restrictions

- **Redistribution:** Source code and binaries may not be redistributed
- **Commercial Use:** Commercial use requires a separate written agreement
- **Public Deployment:** Public deployment is not permitted without authorization
- **Derivative Works:** Creating derivative works requires separate permission

### Ownership

All intellectual property rights in this software, including but not limited to:
- Source code
- Architecture and design
- API structure
- Business logic
- Documentation

remain the exclusive property of the original authors and copyright holder.

Use of this software during an academic internship or research program does **NOT** transfer ownership, copyright, or any intellectual property rights to the intern, their educational institution, or their host organization.

### Documentation

- [LICENSE](./LICENSE) — Full proprietary license terms
- [COPYRIGHT.md](./COPYRIGHT.md) — Copyright and ownership notice
- [NOTICE.md](./NOTICE.md) — Legal ownership notice
- [docs/IP-OWNERSHIP.md](./docs/IP-OWNERSHIP.md) — IP ownership structure
- [docs/INTERNSHIP-LICENSE.md](./docs/INTERNSHIP-LICENSE.md) — Academic/internship use terms
- [docs/DEVELOPMENT-HISTORY.md](./docs/DEVELOPMENT-HISTORY.md) — Development timeline evidence
- [THIRD-PARTY-NOTICES.md](./THIRD-PARTY-NOTICES.md) — Third-party component notices

### Third-Party Components

This software uses third-party Go libraries, each with their own applicable licenses (MIT, Apache 2.0, BSD). See [THIRD-PARTY-NOTICES.md](./THIRD-PARTY-NOTICES.md) for details.

---

## Contact

For licensing inquiries or authorization requests:

**Fransiscus Xaverius**
