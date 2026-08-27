# Third-Party Notices

## Overview

This document acknowledges the third-party components used in serabutin-id-backend and their respective licenses.

**Important:** The proprietary license for serabutin-id-backend applies ONLY to the original code developed by the project authors. It does NOT apply to third-party components, which retain their respective licenses.

---

## Dependencies

### Go Standard Library
- **License:** BSD 3-Clause License
- **Usage:** Core Go language functionality

### github.com/disintegration/imaging v1.6.2
- **License:** MIT License
- **Source:** https://github.com/disintegration/imaging
- **Purpose:** Image processing and manipulation

### github.com/go-playground/validator/v10 v10.27.0
- **License:** MIT License
- **Source:** https://github.com/go-playground/validator
- **Purpose:** Struct field validation

### github.com/gofiber/fiber/v3 v3.1.0
- **License:** MIT License
- **Source:** https://github.com/gofiber/fiber
- **Purpose:** Web framework

### github.com/golang-jwt/jwt/v5 v5.3.1
- **License:** BSD 3-Clause License
- **Source:** https://github.com/golang-jwt/jwt
- **Purpose:** JWT authentication

### github.com/google/uuid v1.6.0
- **License:** BSD 3-Clause License
- **Source:** https://github.com/google/uuid
- **Purpose:** UUID generation

### github.com/minio/minio-go/v7 v7.2.0
- **License:** Apache License 2.0
- **Source:** https://github.com/minio/minio-go
- **Purpose:** S3-compatible object storage client

### github.com/redis/go-redis/v9 v9.19.0
- **License:** BSD 2-Clause License
- **Source:** https://github.com/redis/go-redis
- **Purpose:** Redis client

### github.com/rs/zerolog v1.35.1
- **License:** MIT License
- **Source:** https://github.com/rs/zerolog
- **Purpose:** Logging library

### github.com/spf13/viper v1.21.0
- **License:** MIT License
- **Source:** https://github.com/spf13/viper
- **Purpose:** Configuration management

### github.com/stretchr/testify v1.11.1
- **License:** MIT License
- **Source:** https://github.com/stretchr/testify
- **Purpose:** Testing assertions and mocking

### github.com/testcontainers/testcontainers-go v0.42.0
- **License:** MIT License
- **Source:** https://github.com/testcontainers/testcontainers-go
- **Purpose:** Test containers for integration testing

### github.com/testcontainers/testcontainers-go/modules/postgres v0.42.0
- **License:** MIT License
- **Source:** https://github.com/testcontainers/testcontainers-go
- **Purpose:** PostgreSQL test container

### github.com/testcontainers/testcontainers-go/modules/redis v0.42.0
- **License:** MIT License
- **Source:** https://github.com/testcontainers/testcontainers-go
- **Purpose:** Redis test container

### golang.org/x/crypto v0.51.0
- **License:** BSD 3-Clause License
- **Source:** https://golang.org/x/crypto
- **Purpose:** Cryptographic functions

### gorm.io/driver/postgres v1.6.0
- **License:** Apache License 2.0
- **Source:** https://github.com/go-gorm/postgres
- **Purpose:** PostgreSQL database driver for GORM

### gorm.io/gorm v1.31.1
- **License:** Apache License 2.0
- **Source:** https://github.com/go-gorm/gorm
- **Purpose:** Object-relational mapper

---

## License Types Summary

| License | Number of Dependencies |
|---------|------------------------|
| MIT License | 9 |
| BSD 3-Clause License | 4 |
| Apache License 2.0 | 3 |
| BSD 2-Clause License | 1 |
| Go Standard Library (BSD 3-Clause) | 1 |

---

## Additional Notices

### TestContainers
TestContainers is used solely for integration testing purposes. TestContainers and its modules are licensed under MIT License.

### PostgreSQL Driver
The GORM PostgreSQL driver is used for database connectivity. GORM and its drivers are licensed under Apache License 2.0.

### Redis Client
The go-redis library is used for Redis connectivity and caching. It is licensed under BSD 2-Clause License.

---

## License Compatibility

The third-party licenses listed above are compatible with the proprietary license of serabutin-id-backend in that:

1. They do not require the proprietary code to be open-sourced
2. They permit use in proprietary/commercial software
3. They do not transfer any additional rights to the serabutin-id-backend project beyond what each library provides

Users of serabutin-id-backend should comply with the respective licenses of each third-party component when using them.

---

## Updates

This document should be updated when dependencies are added, removed, or changed.

---

**Document Version:** 1.0

**Last Updated:** 2026
