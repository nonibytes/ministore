#!/bin/bash
set -e

echo "========================================="
echo "Ministore Go E2E Test Suite"
echo "Tests both SQLite and PostgreSQL backends"
echo "========================================="
echo

CLI="/tmp/ministore"
SQLITE_INDEX="/tmp/ministore-sqlite-test.db"
PG_CONN="postgresql://chapera:chapera_dev_secret@localhost:5432/chapera"
PG_SCHEMA="ministore_test_$$"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass() {
    echo -e "${GREEN}✓${NC} $1"
}

fail() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

info() {
    echo -e "${BLUE}➜${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Function to run tests for a backend
run_tests() {
    local BACKEND="$1"
    local INDEX="$2"
    local BACKEND_FLAG=""
    local SCHEMA_FLAG=""

    if [ "$BACKEND" = "postgres" ]; then
        BACKEND_FLAG="--backend postgres"
        SCHEMA_FLAG="--schema-name $PG_SCHEMA"
    fi

    echo
    echo -e "${BLUE}=========================================${NC}"
    echo -e "${BLUE}Testing $BACKEND backend${NC}"
    echo -e "${BLUE}=========================================${NC}"
    echo

    # Create schema file
    cat > /tmp/schema.json <<'EOF'
{
  "fields": {
    "title": { "type": "text", "weight": 2.0 },
    "body": { "type": "text", "weight": 1.0 },
    "tags": { "type": "keyword", "multi": true },
    "category": { "type": "keyword" },
    "priority": { "type": "number" },
    "score": { "type": "number" },
    "due": { "type": "date" },
    "active": { "type": "bool" }
  }
}
EOF

    # 1. CREATE INDEX
    info "Creating index..."
    $CLI index create -i "$INDEX" --schema /tmp/schema.json $BACKEND_FLAG $SCHEMA_FLAG || fail "Index creation failed"
    pass "Index created"

    # 2. INSERT TEST DATA
    info "Inserting test data..."

    echo '{"path": "/docs/rust-guide", "title": "Rust Programming Guide", "body": "Learn Rust programming with examples", "tags": ["rust", "programming", "tutorial"], "category": "technical", "priority": 5, "score": 95.5, "due": "2024-03-15", "active": true}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG
    echo '{"path": "/docs/python-basics", "title": "Python Basics", "body": "Introduction to Python programming", "tags": ["python", "programming"], "category": "technical", "priority": 3, "score": 88.0, "active": true}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG
    echo '{"path": "/notes/meeting-2024", "title": "Team Meeting Notes", "body": "Discussed project roadmap and milestones", "tags": ["meeting", "planning"], "category": "notes", "priority": 2, "score": 70.0, "due": "2024-02-01", "active": false}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG
    echo '{"path": "/blog/search-engines", "title": "Building Search Engines", "body": "How to build efficient search systems", "tags": ["search", "engineering"], "category": "blog", "priority": 4, "score": 92.0, "active": true}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG
    echo '{"path": "/drafts/archived-post", "title": "Old Draft", "body": "This is archived content", "tags": ["draft"], "category": "archive", "priority": 1, "score": 50.0, "active": false}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG
    pass "Test data inserted"

    # 3. BASIC QUERIES
    info "Testing basic predicates..."

    # Text search
    RESULT=$($CLI search -i "$INDEX" -w 'programming' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -ge 2 ] || fail "text search 'programming' should return at least 2 items, got $RESULT"
    pass "text search works"

    # keyword exact match
    RESULT=$($CLI search -i "$INDEX" -w 'tags:rust' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 1 ] || fail "tags:rust should return 1 item, got $RESULT"
    pass "keyword exact match works"

    # keyword prefix
    RESULT=$($CLI search -i "$INDEX" -w 'tags:prog*' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "tags:prog* should return 2 items, got $RESULT"
    pass "keyword prefix match works"

    # bool predicate
    RESULT=$($CLI search -i "$INDEX" -w 'programming AND active:true' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "programming AND active:true should return 2 items, got $RESULT"
    pass "bool predicate works"

    # 4. MULTI-PREDICATE QUERIES
    info "Testing multi-predicate queries..."

    # AND query
    RESULT=$($CLI search -i "$INDEX" -w 'tags:programming AND active:true' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "tags:programming AND active:true should return 2 items, got $RESULT"
    pass "AND query works"

    # OR query
    RESULT=$($CLI search -i "$INDEX" -w 'tags:rust OR tags:python' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "tags:rust OR tags:python should return 2 items, got $RESULT"
    pass "OR query works"

    # NOT query
    RESULT=$($CLI search -i "$INDEX" -w 'programming AND NOT active:false' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "programming AND NOT active:false should return 2 items, got $RESULT"
    pass "NOT query works"

    # 5. PATH QUERIES
    info "Testing path predicates..."

    RESULT=$($CLI search -i "$INDEX" -w 'path:"/docs/*"' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 2 ] || fail "path:/docs/* should return 2 items, got $RESULT"
    pass "path glob works"

    # 6. RANKING
    info "Testing ranking modes..."

    # Recency ranking
    RESULT=$($CLI search -i "$INDEX" -w 'programming' --rank recency --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -ge 2 ] || fail "Recency ranking returned no results"
    pass "Recency ranking works"

    # Field ranking
    FIRST_PATH=$($CLI search -i "$INDEX" -w 'category:technical' --rank 'field:score' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items[0].path')
    [ "$FIRST_PATH" = "/docs/rust-guide" ] || fail "Field ranking by score should return rust-guide first, got $FIRST_PATH"
    pass "Field ranking works"

    # 7. DISCOVER
    info "Testing discover..."

    # Discover fields
    FIELD_COUNT=$($CLI discover fields -i "$INDEX" --format json $BACKEND_FLAG $SCHEMA_FLAG | jq 'length')
    [ "$FIELD_COUNT" -ge 8 ] || fail "discover fields should show at least 8 fields, got $FIELD_COUNT"
    pass "Discover fields works"

    # Discover values
    TAG_COUNT=$($CLI discover values -i "$INDEX" --field tags --format json $BACKEND_FLAG $SCHEMA_FLAG | jq 'length')
    [ "$TAG_COUNT" -ge 5 ] || fail "discover values for tags should show at least 5 values, got $TAG_COUNT"
    pass "Discover values works"

    # 8. STATS
    info "Testing stats..."

    STATS=$($CLI stats -i "$INDEX" --field priority --format json $BACKEND_FLAG $SCHEMA_FLAG)
    AVG=$(echo "$STATS" | jq -r '.avg')
    [ -n "$AVG" ] && [ "$(echo "$AVG > 0" | bc)" -eq 1 ] || fail "stats should return valid average"
    pass "Stats works"

    # 9. UPDATE
    info "Testing update..."

    echo '{"path": "/docs/rust-guide", "title": "Rust Programming Guide Updated", "body": "Learn Rust programming with examples", "tags": ["rust", "advanced", "tutorial"], "category": "technical", "priority": 5, "score": 95.5, "due": "2024-03-15", "active": true}' | $CLI put -i "$INDEX" --json $BACKEND_FLAG $SCHEMA_FLAG

    RESULT=$($CLI search -i "$INDEX" -w 'tags:advanced' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 1 ] || fail "Updated tag should be searchable, got $RESULT"
    pass "Update works"

    # 10. GET
    info "Testing get..."

    RESULT=$($CLI get -i "$INDEX" -p "/docs/rust-guide" $BACKEND_FLAG $SCHEMA_FLAG 2>&1)
    echo "$RESULT" | grep -q "Rust Programming Guide Updated" || fail "Get should return updated title"
    pass "Get works"

    # 11. PEEK
    info "Testing peek..."

    RESULT=$($CLI peek -i "$INDEX" -p "/docs/rust-guide" $BACKEND_FLAG $SCHEMA_FLAG 2>&1)
    echo "$RESULT" | grep -q "technical" || fail "Peek should return document data"
    pass "Peek works"

    # 12. DELETE
    info "Testing delete..."

    $CLI delete -i "$INDEX" -p /drafts/archived-post $BACKEND_FLAG $SCHEMA_FLAG || fail "Delete by path failed"

    RESULT=$($CLI search -i "$INDEX" -w 'category:archive' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 0 ] || fail "Deleted item should not be found, got $RESULT"
    pass "Delete works"

    # 13. DELETE WHERE
    info "Testing delete where..."

    $CLI delete -i "$INDEX" -w 'active:false' $BACKEND_FLAG $SCHEMA_FLAG || fail "Delete where failed"

    RESULT=$($CLI search -i "$INDEX" -w 'category:notes' --format json $BACKEND_FLAG $SCHEMA_FLAG | jq -r '.items | length')
    [ "$RESULT" -eq 0 ] || fail "Items matching delete where should be gone, got $RESULT"
    pass "Delete where works"

    # 14. PAGINATION
    info "Testing pagination..."

    RESULT=$($CLI search -i "$INDEX" -w 'programming' --limit 1 --format json $BACKEND_FLAG $SCHEMA_FLAG)
    ITEM_COUNT=$(echo "$RESULT" | jq -r '.items | length')
    HAS_MORE=$(echo "$RESULT" | jq -r '.has_more')
    [ "$ITEM_COUNT" -eq 1 ] || fail "Limit 1 should return 1 item, got $ITEM_COUNT"
    [ "$HAS_MORE" = "true" ] || fail "has_more should be true"
    pass "Pagination works"

    echo
    echo -e "${GREEN}All $BACKEND tests passed!${NC}"
}

# Cleanup function
cleanup() {
    info "Cleaning up..."
    rm -f "$SQLITE_INDEX"*
    rm -f /tmp/schema.json

    # Drop PostgreSQL schema if it exists
    if command -v psql &> /dev/null; then
        psql "$PG_CONN" -c "DROP SCHEMA IF EXISTS $PG_SCHEMA CASCADE" 2>/dev/null || true
    else
        # Use docker exec to run psql
        docker exec chapera-db psql -U chapera -d chapera -c "DROP SCHEMA IF EXISTS $PG_SCHEMA CASCADE" 2>/dev/null || true
    fi
}

# Clean up on exit
trap cleanup EXIT

# Build CLI if needed
if [ ! -f "$CLI" ]; then
    info "Building CLI..."
    go build -o "$CLI" ./cmd/ministore
fi

# Run SQLite tests
cleanup  # Clean any previous runs
run_tests "sqlite" "$SQLITE_INDEX"

# Run PostgreSQL tests
info "Checking PostgreSQL connection..."
if docker exec chapera-db psql -U chapera -d chapera -c "SELECT 1" &>/dev/null; then
    run_tests "postgres" "$PG_CONN"
else
    warn "PostgreSQL not available, skipping PostgreSQL tests"
fi

echo
echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}All backend tests completed successfully!${NC}"
echo -e "${GREEN}=========================================${NC}"
