// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package monitoring

import (
	"database/sql"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

const rowCountCutoff = 10000.0

// NewTableSizeCollector returns a new collector that monitors table sizes.
func NewTableSizeCollector(namespace string, db *sql.DB) (*dbTableSizeCollector, error) {
	var dbName string
	q := `SELECT current_database();`
	err := db.QueryRow(q).Scan(&dbName)
	if err != nil {
		return nil, err
	}
	return &dbTableSizeCollector{
		countDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "database", "table_row_count"),
			"table row count",
			[]string{"database", "table"},
			nil),
		db:     db,
		dbName: dbName,
	}, nil
}

type dbTableSizeCollector struct {
	countDesc *prometheus.Desc

	db     *sql.DB
	dbName string
}

var _ prometheus.Collector = (*dbTableSizeCollector)(nil)

// Describe implements the prometheus.Collector interface.
func (u *dbTableSizeCollector) Describe(c chan<- *prometheus.Desc) {
	c <- u.countDesc
}

// Collect implements the prometheus.Collector interface.
func (u *dbTableSizeCollector) Collect(ch chan<- prometheus.Metric) {
	// Collecting table sizes is done in two steps. First table row count
	// estimates are queried, because this is fast.
	// Then for tables whose row count estimate is below the threshold,
	// an exact query is issued.
	tableEstimateQuery := `SELECT t.table_name, c.reltuples 
        FROM information_schema.tables t INNER JOIN pg_class c
            ON c.relname = t.table_name 
            WHERE t.table_schema='public' AND t.table_type='BASE TABLE'`

	tables := map[string]float64{}
	var tableName string
	var rowEstimate float64

	rows, err := u.db.Query(tableEstimateQuery)
	if err != nil {
		log.Errorf("failed to query existing tables: %v", err)
	}
	for rows.Next() {
		err = rows.Scan(&tableName, &rowEstimate)
		if err != nil {
			rows.Close()
			log.Errorf("failed to scan defined table names: %v", err)
		}
		tables[tableName] = rowEstimate
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		log.Errorf("failed to scan defined table names: %v", err)
	}
	if len(tables) == 0 {
		log.Warningf("no tables found on DB %q", u.dbName)
		return
	}
	for tableName, rowEstimate := range tables {
		// If the table's row count estimate is more than the cutoff value,
		// report the estimate.
		if rowEstimate > rowCountCutoff {
			mCount, err := prometheus.NewConstMetric(u.countDesc, prometheus.GaugeValue, rowEstimate,
				u.dbName, tableName)
			if err != nil {
				log.Errorf("failed to report table size for %q: %v", tableName, err)
				return
			}
			ch <- mCount
			continue
		}
		var rows int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)

		if err := u.db.QueryRow(query).Scan(&rows); err != nil {
			log.Errorf("failed to query table size for %q: %v", tableName, err)
			return
		}

		mCount, err := prometheus.NewConstMetric(u.countDesc, prometheus.GaugeValue, float64(rows),
			u.dbName, tableName)
		if err != nil {
			log.Errorf("failed to report table size for %q: %v", tableName, err)
			return
		}
		ch <- mCount
	}
}
