package query

import (
	"testing"
)

func TestParseSimplePredicate(t *testing.T) {
	expr, err := Parse("title:hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	kw, ok := pred.Predicate.(Keyword)
	if !ok {
		t.Fatalf("expected Keyword, got %T", pred.Predicate)
	}
	if kw.Field != "title" || kw.Pattern != "hello" {
		t.Errorf("expected title:hello, got %s:%s", kw.Field, kw.Pattern)
	}
}

func TestParseAndExpression(t *testing.T) {
	expr, err := Parse("a AND b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	andExpr, ok := expr.(And)
	if !ok {
		t.Fatalf("expected And, got %T", expr)
	}
	_ = andExpr // both left and right should be Pred with Text predicates
}

func TestParseOrExpression(t *testing.T) {
	expr, err := Parse("a OR b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	orExpr, ok := expr.(Or)
	if !ok {
		t.Fatalf("expected Or, got %T", expr)
	}
	_ = orExpr
}

func TestParseNotExpression(t *testing.T) {
	expr, err := Parse("NOT archived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// NOT <bareword> should become Bool{Field: "archived", Value: false}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred for NOT shorthand, got %T", expr)
	}
	boolPred, ok := pred.Predicate.(Bool)
	if !ok {
		t.Fatalf("expected Bool predicate, got %T", pred.Predicate)
	}
	if boolPred.Field != "archived" || boolPred.Value != false {
		t.Errorf("expected archived:false, got %s:%v", boolPred.Field, boolPred.Value)
	}
}

func TestParseExclamationNot(t *testing.T) {
	expr, err := Parse("!done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	boolPred, ok := pred.Predicate.(Bool)
	if !ok {
		t.Fatalf("expected Bool predicate, got %T", pred.Predicate)
	}
	if boolPred.Field != "done" || boolPred.Value != false {
		t.Errorf("expected done:false, got %s:%v", boolPred.Field, boolPred.Value)
	}
}

func TestParseNumberComparison(t *testing.T) {
	expr, err := Parse("priority>5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	numCmp, ok := pred.Predicate.(NumberCmp)
	if !ok {
		t.Fatalf("expected NumberCmp, got %T", pred.Predicate)
	}
	if numCmp.Field != "priority" || numCmp.Op != CmpGt || numCmp.Value != 5 {
		t.Errorf("expected priority>5, got %s%s%v", numCmp.Field, numCmp.Op, numCmp.Value)
	}
}

func TestParseNumberRange(t *testing.T) {
	expr, err := Parse("priority:1..10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	numRange, ok := pred.Predicate.(NumberRange)
	if !ok {
		t.Fatalf("expected NumberRange, got %T", pred.Predicate)
	}
	if numRange.Field != "priority" || numRange.Lo != 1 || numRange.Hi != 10 {
		t.Errorf("expected priority:1..10, got %s:%v..%v", numRange.Field, numRange.Lo, numRange.Hi)
	}
}

func TestParseHasPredicate(t *testing.T) {
	expr, err := Parse("has:tags")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	hasPred, ok := pred.Predicate.(Has)
	if !ok {
		t.Fatalf("expected Has, got %T", pred.Predicate)
	}
	if hasPred.Field != "tags" {
		t.Errorf("expected has:tags, got has:%s", hasPred.Field)
	}
}

func TestParsePathGlob(t *testing.T) {
	expr, err := Parse("path:/docs/*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	pathGlob, ok := pred.Predicate.(PathGlob)
	if !ok {
		t.Fatalf("expected PathGlob, got %T", pred.Predicate)
	}
	if pathGlob.Pattern != "/docs/*" {
		t.Errorf("expected /docs/*, got %s", pathGlob.Pattern)
	}
}

func TestParseKeywordWildcard(t *testing.T) {
	expr, err := Parse("tags:test*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	kw, ok := pred.Predicate.(Keyword)
	if !ok {
		t.Fatalf("expected Keyword, got %T", pred.Predicate)
	}
	if kw.Field != "tags" || kw.Pattern != "test*" || kw.Kind != KeywordPrefix {
		t.Errorf("expected tags:test* (prefix), got %s:%s (%v)", kw.Field, kw.Pattern, kw.Kind)
	}
}

func TestParseBareText(t *testing.T) {
	expr, err := Parse("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pred, ok := expr.(Pred)
	if !ok {
		t.Fatalf("expected Pred, got %T", expr)
	}
	textPred, ok := pred.Predicate.(Text)
	if !ok {
		t.Fatalf("expected Text, got %T", pred.Predicate)
	}
	if textPred.Field != nil || textPred.FTS != "hello" {
		t.Errorf("expected bare text 'hello', got field=%v fts=%s", textPred.Field, textPred.FTS)
	}
}

func TestParseParentheses(t *testing.T) {
	expr, err := Parse("(a OR b) AND c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	andExpr, ok := expr.(And)
	if !ok {
		t.Fatalf("expected And, got %T", expr)
	}
	_, ok = andExpr.Left.(Or)
	if !ok {
		t.Fatalf("expected left to be Or, got %T", andExpr.Left)
	}
}

func TestParseComplexQuery(t *testing.T) {
	// tags:important AND (priority>5 OR !done)
	expr, err := Parse("tags:important AND (priority>5 OR !done)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	andExpr, ok := expr.(And)
	if !ok {
		t.Fatalf("expected And, got %T", expr)
	}
	// Left should be keyword predicate
	leftPred, ok := andExpr.Left.(Pred)
	if !ok {
		t.Fatalf("expected left to be Pred, got %T", andExpr.Left)
	}
	_, ok = leftPred.Predicate.(Keyword)
	if !ok {
		t.Fatalf("expected left predicate to be Keyword, got %T", leftPred.Predicate)
	}
	// Right should be Or
	_, ok = andExpr.Right.(Or)
	if !ok {
		t.Fatalf("expected right to be Or, got %T", andExpr.Right)
	}
}
