package okf

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ministore/ministore/ministore"
	"gopkg.in/yaml.v3"
)

const ProjectionVersion = 1

func ProjectionSchema() ministore.Schema {
	w3, w2, w1 := 3.0, 2.0, 1.0
	f := map[string]ministore.FieldSpec{}
	keyword := func(name string, multi bool) {
		f[name] = ministore.FieldSpec{Type: ministore.FieldKeyword, Multi: multi}
	}
	date := func(name string, multi bool) { f[name] = ministore.FieldSpec{Type: ministore.FieldDate, Multi: multi} }
	number := func(name string, multi bool) {
		f[name] = ministore.FieldSpec{Type: ministore.FieldNumber, Multi: multi}
	}
	keyword("type", false)
	f["title"] = ministore.FieldSpec{Type: ministore.FieldText, Weight: &w3}
	f["description"] = ministore.FieldSpec{Type: ministore.FieldText, Weight: &w2}
	f["body"] = ministore.FieldSpec{Type: ministore.FieldText, Weight: &w1}
	for _, n := range []string{"tags", "verified_by", "source_ids", "source_resources", "source_authors", "link_targets", "backlinks"} {
		keyword(n, true)
	}
	for _, n := range []string{"status", "resource", "generated_by", "trust_tier", "runtime", "okf_version", "okf_source_path", "okf_projection_hash"} {
		keyword(n, false)
	}
	for _, n := range []string{"generated_at", "latest_verified_at", "stale_after"} {
		date(n, false)
	}
	date("source_last_modified", true)
	number("source_usage_counts", true)
	number("okf_projection_version", false)
	return ministore.Schema{Fields: f}
}

func WalkProjections(ctx context.Context, root string, opts ValidateOptions, emit ProjectionSink) (summary ValidationSummary, err error) {
	stage, summary, err := prepareBundle(ctx, root, opts)
	if err != nil {
		return ValidationSummary{}, err
	}
	defer func() {
		if closeErr := stage.close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	after := ""
	for {
		var source string
		var raw []byte
		err := stage.db.QueryRowContext(ctx, `SELECT path,raw FROM concepts WHERE path > ? COLLATE BINARY ORDER BY path COLLATE BINARY LIMIT 1`, after).Scan(&source, &raw)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return ValidationSummary{}, fmt.Errorf("read staged OKF concept: %w", err)
		}
		after = source
		projection, err := stage.project(ctx, source, raw, summary.TargetVersion)
		if err != nil {
			return ValidationSummary{}, err
		}
		if emit != nil {
			if err := emit(projection); err != nil {
				return ValidationSummary{}, err
			}
		}
	}
	return summary, nil
}

func (s *validationStage) project(ctx context.Context, source string, raw []byte, version string) (Projection, error) {
	doc, _, err := ParseDocument(source, raw)
	if err != nil {
		return nil, err
	}
	m := doc.Metadata()
	p := Projection{"path": "/" + strings.TrimSuffix(source, ".md"), "raw_document": string(raw), "body": string(doc.Body()), "title": strings.TrimSuffix(path.Base(source), path.Ext(source)), "status": "stable", "trust_tier": "unverified", "okf_version": version, "okf_source_path": source, "okf_projection_version": ProjectionVersion}
	if m != nil {
		copyString := func(key string) {
			if v, ok := m.String(key); ok && v != "" {
				p[key] = v
			}
		}
		copyString("type")
		copyString("title")
		copyString("description")
		copyString("resource")
		copyString("status")
		copyString("runtime")
		if v, ok := m.Date("stale_after"); ok && isProjectionDate(v) {
			p["stale_after"] = v
		}
		if values, _, _ := m.Strings("tags"); len(values) > 0 {
			p["tags"] = sortedUnique(values)
		}
		projectGenerated(m, p)
		projectVerified(m, p)
		projectSources(m, p)
	}
	links, err := s.edgePaths(ctx, source, true)
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		p["link_targets"] = links
	}
	backs, err := s.edgePaths(ctx, source, false)
	if err != nil {
		return nil, err
	}
	if len(backs) > 0 {
		p["backlinks"] = backs
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	p["okf_projection_hash"] = hex.EncodeToString(sum[:])
	return p, nil
}

func (s *validationStage) edgePaths(ctx context.Context, source string, forward bool) ([]string, error) {
	query, arg := `SELECT target FROM edges WHERE source=? ORDER BY target COLLATE BINARY`, source
	if !forward {
		query = `SELECT source FROM edges WHERE target=? ORDER BY source COLLATE BINARY`
	}
	rows, err := s.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, "/"+strings.TrimSuffix(v, ".md"))
	}
	return out, rows.Err()
}

func projectGenerated(m *Metadata, p Projection) {
	if n, ok := m.Lookup("generated"); ok {
		n = ResolveAlias(n)
		if n != nil && n.Kind == yaml.MappingNode {
			if v, ok := MappingLookup(n, "by"); ok {
				if s, ok := NodeString(v); ok && s != "" {
					p["generated_by"] = s
				}
			}
			if v, ok := MappingLookup(n, "at"); ok {
				if s, ok := NodeDate(v); ok {
					if _, e := time.Parse(time.RFC3339, s); e == nil {
						p["generated_at"] = s
					}
				}
			}
		}
	}
	if _, ok := p["generated_at"]; !ok {
		if v, ok := m.Date("timestamp"); ok {
			if _, e := time.Parse(time.RFC3339, v); e == nil {
				p["generated_at"] = v
			}
		}
	}
}

func projectVerified(m *Metadata, p Projection) {
	events, _, _ := m.Mappings("verified")
	var actors []string
	var latest string
	human := false
	for _, event := range events {
		byN, byOK := MappingLookup(event, "by")
		atN, atOK := MappingLookup(event, "at")
		by, bv := NodeString(byN)
		at, av := NodeDate(atN)
		if !byOK || !atOK || !bv || !av || by == "" {
			continue
		}
		if _, e := time.Parse(time.RFC3339, at); e != nil {
			continue
		}
		actors = append(actors, by)
		if at > latest {
			latest = at
		}
		if strings.HasPrefix(by, "human:") {
			human = true
		}
	}
	actors = sortedUnique(actors)
	if len(actors) > 0 {
		p["verified_by"] = actors
		p["latest_verified_at"] = latest
		if human {
			p["trust_tier"] = "human-reviewed"
		} else {
			p["trust_tier"] = "machine-confirmed"
		}
	}
}

func projectSources(m *Metadata, p Projection) {
	sources, _, _ := m.Mappings("sources")
	var ids, resources, authors, modified []string
	var counts []float64
	for _, s := range sources {
		for key, dst := range map[string]*[]string{"id": &ids, "resource": &resources, "author": &authors} {
			if n, ok := MappingLookup(s, key); ok {
				if v, ok := NodeString(n); ok && v != "" {
					*dst = append(*dst, v)
				}
			}
		}
		if n, ok := MappingLookup(s, "last_modified"); ok {
			if v, ok := NodeDate(n); ok && isProjectionDate(v) {
				modified = append(modified, v)
			}
		}
		if n, ok := MappingLookup(s, "usage_count"); ok && validUsageCount(n) {
			v, _ := strconv.ParseFloat(strings.ReplaceAll(n.Value, "_", ""), 64)
			counts = append(counts, v)
		}
	}
	for key, values := range map[string][]string{"source_ids": ids, "source_resources": resources, "source_authors": authors, "source_last_modified": modified} {
		if values = sortedUnique(values); len(values) > 0 {
			p[key] = values
		}
	}
	if len(counts) > 0 {
		sort.Float64s(counts)
		unique := counts[:0]
		for _, v := range counts {
			if len(unique) == 0 || unique[len(unique)-1] != v {
				unique = append(unique, v)
			}
		}
		numbers := make([]json.Number, len(unique))
		for i, value := range unique {
			if value < float64(uint64(1)<<63) && value == float64(int64(value)) {
				numbers[i] = json.Number(strconv.FormatInt(int64(value), 10))
			} else {
				numbers[i] = json.Number(normalizeJSONExponent(strconv.FormatFloat(value, 'g', -1, 64)))
			}
		}
		p["source_usage_counts"] = numbers
	}
}

func normalizeJSONExponent(value string) string {
	mantissa, exponent, ok := strings.Cut(value, "e")
	if !ok {
		return value
	}
	sign := ""
	if strings.HasPrefix(exponent, "+") || strings.HasPrefix(exponent, "-") {
		sign = exponent[:1]
		exponent = exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	if exponent == "" {
		exponent = "0"
	}
	return mantissa + "e" + sign + exponent
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	for _, v := range values {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}

func isProjectionDate(value string) bool {
	return isISODate(value) || func() bool { _, err := time.Parse(time.RFC3339, value); return err == nil }()
}
