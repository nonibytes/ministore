package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/sqlite"
	_ "modernc.org/sqlite"
)

func main() {
	ctx := context.Background()
	dbPath := "test_index.db"

	// Clean up any existing test database
	os.Remove(dbPath)

	fmt.Println("=== Testing Ministore Implementation ===")

	// Test 1: Create an index with schema
	fmt.Println("Test 1: Creating index with schema...")
	weight3 := 3.0
	weight1 := 1.0
	schema := ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"title": {
				Type:   ministore.FieldText,
				Weight: &weight3,
			},
			"tags": {
				Type:  ministore.FieldKeyword,
				Multi: true,
			},
			"priority": {
				Type: ministore.FieldNumber,
			},
			"content": {
				Type:   ministore.FieldText,
				Weight: &weight1,
			},
		},
	}

	adapter := sqlite.New(dbPath)
	ix, err := ministore.Create(ctx, adapter, schema, ministore.DefaultIndexOptions())
	if err != nil {
		log.Fatalf("Failed to create index: %v", err)
	}
	fmt.Println("✓ Index created successfully")

	// Test 2: Verify schema
	fmt.Println("\nTest 2: Verifying schema...")
	retrievedSchema := ix.Schema()
	fmt.Printf("✓ Schema has %d fields\n", len(retrievedSchema.Fields))
	for name, spec := range retrievedSchema.Fields {
		fmt.Printf("  - %s: %s", name, spec.Type)
		if spec.Multi {
			fmt.Print(" (multi)")
		}
		if spec.Weight != nil {
			fmt.Printf(" weight=%.1f", *spec.Weight)
		}
		fmt.Println()
	}

	// Test 3: Close and reopen
	fmt.Println("\nTest 3: Closing and reopening index...")
	if err := ix.Close(); err != nil {
		log.Fatalf("Failed to close index: %v", err)
	}
	fmt.Println("✓ Index closed")

	ix, err = ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		log.Fatalf("Failed to reopen index: %v", err)
	}
	fmt.Println("✓ Index reopened successfully")

	// Test 4: Verify schema persisted
	fmt.Println("\nTest 4: Verifying schema persisted...")
	reopenedSchema := ix.Schema()
	if len(reopenedSchema.Fields) != len(schema.Fields) {
		log.Fatalf("Schema mismatch: expected %d fields, got %d", len(schema.Fields), len(reopenedSchema.Fields))
	}
	fmt.Printf("✓ Schema persisted correctly (%d fields)\n", len(reopenedSchema.Fields))

	// Test 5: Manually insert an item via SQL to test Get()
	fmt.Println("\nTest 5: Testing Get() operation...")
	db := ix.DB()
	testDoc := `{"path":"/test/doc","title":"Test Document","tags":["test","demo"],"priority":5}`
	_, err = db.ExecContext(ctx,
		"INSERT INTO items(path, data_json, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"/test/doc", testDoc, 1000000, 1000000)
	if err != nil {
		log.Fatalf("Failed to insert test item: %v", err)
	}
	fmt.Println("✓ Test item inserted via SQL")

	item, err := ix.Get(ctx, "/test/doc")
	if err != nil {
		log.Fatalf("Failed to get item: %v", err)
	}
	fmt.Printf("✓ Retrieved item: %s\n", item.Path)
	fmt.Printf("  Data: %s\n", string(item.DocJSON))
	fmt.Printf("  Created: %d, Updated: %d\n", item.Meta.CreatedAtMS, item.Meta.UpdatedAtMS)

	// Test 6: Test Get() on non-existent item
	fmt.Println("\nTest 6: Testing Get() on non-existent item...")
	_, err = ix.Get(ctx, "/does/not/exist")
	if err == nil {
		log.Fatal("Expected error for non-existent item")
	}
	if !ministore.IsKind(err, ministore.ErrNotFound) {
		log.Fatalf("Expected NotFound error, got: %v", err)
	}
	fmt.Println("✓ Correctly returns NotFound error")

	// Test 7: Test Peek()
	fmt.Println("\nTest 7: Testing Peek() operation...")
	docJSON, err := ix.Peek(ctx, "/test/doc")
	if err != nil {
		log.Fatalf("Failed to peek item: %v", err)
	}
	fmt.Printf("✓ Peek returned: %s\n", string(docJSON))

	// Test 8: Test batch operations (framework only, put/delete not implemented)
	fmt.Println("\nTest 8: Testing batch framework...")
	batch := ministore.NewBatch()
	if !batch.Empty() {
		log.Fatal("New batch should be empty")
	}
	fmt.Printf("✓ Batch created: len=%d, empty=%v\n", batch.Len(), batch.Empty())

	// Test 9: Test schema text fields
	fmt.Println("\nTest 9: Testing text field extraction...")
	textFields := schema.TextFieldsInOrder()
	fmt.Printf("✓ Found %d text fields:\n", len(textFields))
	for _, tf := range textFields {
		fmt.Printf("  - %s (weight: %.1f)\n", tf.Name, tf.Weight)
	}

	// Test 10: Test Optimize
	fmt.Println("\nTest 10: Testing optimize operation...")
	if err := ix.Optimize(ctx); err != nil {
		log.Fatalf("Failed to optimize: %v", err)
	}
	fmt.Println("✓ Optimize completed")

	// Cleanup
	ix.Close()
	fmt.Println("\n=== All Tests Passed! ===")
	fmt.Printf("\nDatabase created at: %s\n", dbPath)
	fmt.Println("You can inspect it with: sqlite3 test_index.db")
}
