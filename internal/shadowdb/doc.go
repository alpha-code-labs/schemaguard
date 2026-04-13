// Package shadowdb provisions and tears down the ephemeral Docker-based
// Postgres instance that SchemaGuard uses as a shadow database.
//
// Implementation begins in Milestone 2 (see docs/tasks.md). v1 is Docker-only;
// support for pointing at an externally-managed Postgres instance is deferred
// to v1.5 per docs/DECISIONS.md.
package shadowdb
