package seeddata

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// UpgradeResult exposes current observable output, never migration internals.
type UpgradeResult struct {
	DB            *sql.DB
	DataDir       string
	LLMConfig     json.RawMessage
	FileOverrides map[string]string
	CheckedTables map[string]bool
}

func AssertFiles(r *UpgradeResult, expected map[string]string) error {
	for path, content := range expected {
		if override, ok := r.FileOverrides[path]; ok {
			content = override
		}
		actual, err := os.ReadFile(filepath.Join(r.DataDir, filepath.FromSlash(path)))
		if err != nil || string(actual) != content {
			return fmt.Errorf("file %s: got %q, want %q: %v", path, actual, content, err)
		}
	}
	return nil
}

func AssertRows(r *UpgradeResult, expected []SeedRow) error {
	byTable := map[string][]map[string]any{}
	for _, row := range expected {
		byTable[row.Table] = append(byTable[row.Table], row.Values)
	}
	for table, rows := range byTable {
		var columns []string
		for column := range rows[0] {
			if column != "created_at" && column != "updated_at" {
				columns = append(columns, column)
			}
		}
		sort.Strings(columns)
		var projection []string
		for _, column := range columns {
			projection = append(projection, "CAST("+quoteSeed(column)+" AS TEXT)")
		}
		query := "SELECT " + strings.Join(projection, ",") + " FROM " + quoteSeed(table)
		actual, err := r.DB.Query(query)
		if err != nil {
			return fmt.Errorf("%s: %w", query, err)
		}
		var got []string
		for actual.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := actual.Scan(targets...); err != nil {
				_ = actual.Close()
				return err
			}
			encoded, err := json.Marshal(values)
			if err != nil {
				_ = actual.Close()
				return err
			}
			got = append(got, string(encoded))
		}
		readErr := actual.Err()
		closeErr := actual.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		var want []string
		for _, row := range rows {
			values := make([]any, len(columns))
			for i, column := range columns {
				value, exists := row[column]
				if !exists {
					return fmt.Errorf("seed expectation %s lacks column %s", table, column)
				}
				if value != nil {
					switch value {
					case true:
						values[i] = "1"
					case false:
						values[i] = "0"
					default:
						values[i] = fmt.Sprint(value)
					}
				}
			}
			encoded, err := json.Marshal(values)
			if err != nil {
				return err
			}
			want = append(want, string(encoded))
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("%s columns %v: got %v, want %v", table, columns, got, want)
		}
		if r.CheckedTables != nil {
			r.CheckedTables[table] = true
		}
	}
	return nil
}

func AssertConfig(r *UpgradeResult, expected json.RawMessage) error {
	var got, want any
	if err := json.Unmarshal(r.LLMConfig, &got); err != nil {
		return err
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("LLM configuration differs from current seed expectation: got %s, want %s", r.LLMConfig, expected)
	}
	return nil
}

func quoteSeed(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
