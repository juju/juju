//go:build !dqlite && cgo && (sqlite_trace || trace)

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct sqlite3 sqlite3;
typedef struct sqlite3_stmt sqlite3_stmt;

#define SQLITE_OK 0
#define SQLITE_ROW 100
#define SQLITE_STMTSTATUS_FULLSCAN_STEP 1

int sqlite3_finalize(sqlite3_stmt*);
void sqlite3_free(void*);
char *sqlite3_mprintf(const char*, ...);
int sqlite3_prepare_v2(sqlite3*, const char*, int, sqlite3_stmt**, const char**);
int sqlite3_step(sqlite3_stmt*);
int sqlite3_stmt_status(sqlite3_stmt*, int, int);
const unsigned char *sqlite3_column_text(sqlite3_stmt*, int);

static int hasPrefix(const char *s, const char *prefix) {
	while (*prefix != 0) {
		if (*s != *prefix) {
			return 0;
		}
		s++;
		prefix++;
	}
	return 1;
}

static int contains(const char *s, const char *substr) {
	const char *h;
	const char *n;
	if (*substr == 0) {
		return 1;
	}
	for (; *s != 0; s++) {
		h = s;
		n = substr;
		while (*h != 0 && *n != 0 && *h == *n) {
			h++;
			n++;
		}
		if (*n == 0) {
			return 1;
		}
	}
	return 0;
}

static int fullScanSteps(uintptr_t stmt) {
	return sqlite3_stmt_status(
		(sqlite3_stmt *)stmt,
		SQLITE_STMTSTATUS_FULLSCAN_STEP,
		1
	);
}

static int scansOnlyCoveringIndex(uintptr_t dbHandle, const char *query) {
	sqlite3_stmt *stmt = 0;
	char *explain = sqlite3_mprintf("EXPLAIN QUERY PLAN %s", query);
	int sawCoveringIndexScan = 0;
	int rc;

	if (explain == 0) {
		return 0;
	}

	rc = sqlite3_prepare_v2(
		(sqlite3 *)dbHandle,
		explain,
		-1,
		&stmt,
		0
	);
	sqlite3_free(explain);
	if (rc != SQLITE_OK) {
		return 0;
	}

	while (sqlite3_step(stmt) == SQLITE_ROW) {
		const char *detail = (const char *)sqlite3_column_text(stmt, 3);
		if (detail == 0 || !hasPrefix(detail, "SCAN ")) {
			continue;
		}
		if (!contains(detail, "USING COVERING INDEX")) {
			sqlite3_finalize(stmt);
			return 0;
		}
		sawCoveringIndexScan = 1;
	}

	sqlite3_finalize(stmt);
	return sawCoveringIndexScan;
}
*/
import "C"

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/database/client"
)

var sqliteDriverID atomic.Uint64

func sqliteDriverName(cfg appOptions) string {
	if cfg.log == nil || cfg.tracing == client.LogNone {
		return "sqlite3"
	}

	name := fmt.Sprintf("sqlite3-fullscan-%d", sqliteDriverID.Add(1))
	sql.Register(name, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.SetTrace(newFullScanTraceConfig(cfg.log, cfg.tracing))
		},
	})
	return name
}

func newFullScanTraceConfig(
	log client.LogFunc,
	level client.LogLevel,
) *sqlite3.TraceConfig {
	var mu sync.Mutex
	statements := make(map[uintptr]string)

	return &sqlite3.TraceConfig{
		Callback: func(info sqlite3.TraceInfo) int {
			switch info.EventCode {
			case sqlite3.TraceStmt:
				if info.StmtHandle == 0 || strings.HasPrefix(info.StmtOrTrigger, "--") {
					return 0
				}

				query := info.ExpandedSQL
				if query == "" {
					query = info.StmtOrTrigger
				}

				mu.Lock()
				statements[info.StmtHandle] = query
				mu.Unlock()

			case sqlite3.TraceProfile:
				if info.StmtHandle == 0 {
					return 0
				}

				fullScanSteps := int(C.fullScanSteps(C.uintptr_t(info.StmtHandle)))

				mu.Lock()
				query := statements[info.StmtHandle]
				delete(statements, info.StmtHandle)
				mu.Unlock()

				if fullScanSteps == 0 {
					return 0
				}

				if scansOnlyCoveringIndex(info.ConnHandle, query) {
					return 0
				}

				log(
					level,
					fmt.Sprintf(
						"sqlite statement performed full table scan: fullscan_steps=%d query=%q",
						fullScanSteps,
						query,
					),
				)
			}

			return 0
		},
		EventMask:       sqlite3.TraceStmt | sqlite3.TraceProfile,
		WantExpandedSQL: true,
	}
}

func scansOnlyCoveringIndex(connHandle uintptr, query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "EXPLAIN ") {
		return false
	}

	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))

	return C.scansOnlyCoveringIndex(
		C.uintptr_t(connHandle),
		cQuery,
	) == 1
}
