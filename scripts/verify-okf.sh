#!/usr/bin/env bash
set -euo pipefail

go_root=$(cd "$(dirname "$0")/.." && pwd)
rust_root=${MINISTORE_RUST_ROOT:-/home/okecho/nonibytes/ministore-rust}
work=$(mktemp -d)
postgres_name=""
cleanup() {
  if [[ -n "$postgres_name" ]]; then docker rm -f "$postgres_name" >/dev/null 2>&1 || true; fi
  rm -rf "$work"
}
trap cleanup EXIT

(cd "$go_root/testdata/okf/v0.2" && sha256sum -c MANIFEST.sha256)
(cd "$rust_root/testdata/okf/v0.2" && sha256sum -c MANIFEST.sha256)
diff -qr "$go_root/testdata/okf/v0.2" "$rust_root/testdata/okf/v0.2"

(cd "$go_root" && gofmt -d $(git diff --name-only -- '*.go') | tee "$work/gofmt.diff" && test ! -s "$work/gofmt.diff")
(cd "$go_root" && go vet ./... && CGO_ENABLED=0 go test ./... && go test -race ./...)
if command -v cc >/dev/null 2>&1; then (cd "$go_root" && CGO_ENABLED=1 go test -tags 'cgo_sqlite fts5' ./...); fi
(cd "$rust_root" && rustfmt --edition 2021 --check $(find crates/ministore-okf -name '*.rs' -type f) crates/ministore-cli/src/commands/okf.rs && cargo clippy --workspace --all-targets --all-features -- -D warnings && cargo test --workspace --all-features)

(cd "$go_root" && go build -o "$work/ministore-go" ./cmd/ministore)
(cd "$rust_root" && cargo build -p ministore-cli)
rust_cli="$rust_root/target/debug/ministore"

for cli in "$work/ministore-go" "$rust_cli"; do
  missing="$work/dry-missing-$(basename "$cli").db"
  "$cli" okf sync --bundle "$go_root/testdata/okf/v0.2/valid/minimal" --index "$missing" --dry-run --format json >/dev/null
  test ! -e "$missing"
done

if [[ -x /usr/bin/time ]]; then
  python3 - "$work" <<'PY'
import os,sys
root=sys.argv[1]
for count,name in [(100,'small'),(3000,'large')]:
    directory=f'{root}/{name}';os.mkdir(directory)
    source='---\ntype: Note\n---\n'+'x'*2048+'\n'
    for number in range(count):
        with open(f'{directory}/{number:05}.md','w') as handle: handle.write(source)
PY
  for implementation in go rust; do
    cli="$work/ministore-go"; [[ "$implementation" == rust ]] && cli="$rust_cli"
    /usr/bin/time -f %M -o "$work/${implementation}-small.rss" "$cli" okf validate --bundle "$work/small" --format json >/dev/null
    /usr/bin/time -f %M -o "$work/${implementation}-large.rss" "$cli" okf validate --bundle "$work/large" --format json >/dev/null
    awk 'NR==FNR{s=$1;next} $1>s*3{exit 1}' "$work/${implementation}-small.rss" "$work/${implementation}-large.rss"
  done
fi

python3 - "$go_root" "$rust_root" "$work" "$rust_cli" <<'PY'
import json, sqlite3, subprocess, sys
go_root,rust_root,work,rust_cli=sys.argv[1:]
go_cli=work+'/ministore-go'
fixtures=['valid/minimal','valid/provenance','valid/attested-computation','valid/graph-paths','permissive/broken-links','permissive/bare-verified','permissive/scalar-tags','permissive/lifecycle-legacy','permissive/unknown-type-and-keys','permissive/newer-version']
validation_fixtures=fixtures+['invalid/missing-frontmatter','invalid/invalid-yaml','invalid/missing-type','invalid/malformed-index','invalid/malformed-log']
for fixture in validation_fixtures:
    def validate(cli,root):
        result=subprocess.run([cli,'okf','validate','--bundle',f'{root}/testdata/okf/v0.2/{fixture}','--format','json'],text=True,capture_output=True)
        report=json.loads(result.stdout)
        report['validation'].pop('bundle',None)
        for finding in report['findings']:
            for key in ['message','line','column']: finding.pop(key,None)
        return result.returncode,report
    assert validate(go_cli,go_root)==validate(rust_cli,rust_root),fixture
for number,fixture in enumerate(fixtures):
    go_db=f'{work}/go-{number}.db';rust_db=f'{work}/rust-{number}.db'
    go_report=json.loads(subprocess.check_output([go_cli,'okf','sync','--bundle',f'{go_root}/testdata/okf/v0.2/{fixture}','--index',go_db,'--format','json']))
    rust_report=json.loads(subprocess.check_output([rust_cli,'okf','sync','--bundle',f'{rust_root}/testdata/okf/v0.2/{fixture}','--index',rust_db,'--format','json']))
    for report in (go_report,rust_report): report.pop('bundle',None);report.pop('duration_ms',None);report['validation'].pop('bundle',None)
    assert go_report==rust_report,(fixture,go_report,rust_report)
    load=lambda db:[json.loads(row[0]) for row in sqlite3.connect(db).execute('SELECT data_json FROM items ORDER BY path')]
    assert load(go_db)==load(rust_db),fixture
PY

if rg -n 'Max(File|Bundle|Finding|Link|Yaml|YAML)|max_(file|bundle|finding|link|yaml)' "$go_root/okf" "$rust_root/crates/ministore-okf/src"; then
  echo "arbitrary OKF input limit found" >&2
  exit 1
fi

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  postgres_name="ministore-okf-verify-$$"
  docker run -d --name "$postgres_name" -e POSTGRES_PASSWORD=ministore -p 127.0.0.1::5432 postgres:17-alpine >/dev/null
  for _ in $(seq 1 30); do docker exec "$postgres_name" pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done
  postgres_port=$(docker port "$postgres_name" 5432/tcp | awk -F: '{print $NF}')
  (cd "$go_root" && MINISTORE_POSTGRES_TEST_DSN="postgres://postgres:ministore@127.0.0.1:${postgres_port}/postgres?sslmode=disable" go test ./ministore -run 'Test(WriteBatch|ScanPaths)Postgres' -count=1)
  (cd "$go_root" && MINISTORE_POSTGRES_TEST_DSN="postgres://postgres:ministore@127.0.0.1:${postgres_port}/postgres?sslmode=disable" go test ./okf -run TestSyncPostgres -count=1)
fi

echo "OKF verification passed"
