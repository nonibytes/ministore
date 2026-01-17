package query

import (
	"testing"
)

func TestLexSimple(t *testing.T) {
	tokens, err := Lex("title:test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tokens: Ident("title"), Colon, Ident("test"), EOF
	if len(tokens) != 4 {
		t.Fatalf("expected 4 tokens (including EOF), got %d: %v", len(tokens), tokens)
	}
	if tokens[0].Kind != TokIdent || tokens[0].Value != "title" {
		t.Errorf("expected Ident(title), got %v", tokens[0])
	}
	if tokens[1].Kind != TokColon {
		t.Errorf("expected Colon, got %v", tokens[1])
	}
	if tokens[2].Kind != TokIdent || tokens[2].Value != "test" {
		t.Errorf("expected Ident(test), got %v", tokens[2])
	}
	if tokens[3].Kind != TokEOF {
		t.Errorf("expected EOF, got %v", tokens[3])
	}
}

func TestLexOperators(t *testing.T) {
	tokens, err := Lex("priority>5 AND active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[1].Kind != TokGt {
		t.Errorf("expected Gt, got %v", tokens[1])
	}
	if tokens[3].Kind != TokAnd {
		t.Errorf("expected And, got %v", tokens[3])
	}
}

func TestLexString(t *testing.T) {
	tokens, err := Lex(`title:"hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[2].Kind != TokString || tokens[2].Value != "hello world" {
		t.Errorf("expected String(hello world), got %v", tokens[2])
	}
}

func TestLexNumber(t *testing.T) {
	tokens, err := Lex("3.14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].Kind != TokNumber || tokens[0].Num != 3.14 {
		t.Errorf("expected Number(3.14), got %v", tokens[0])
	}
}

func TestLexParens(t *testing.T) {
	tokens, err := Lex("(a OR b) AND c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].Kind != TokLParen {
		t.Errorf("expected LParen, got %v", tokens[0])
	}
	if tokens[4].Kind != TokRParen {
		t.Errorf("expected RParen, got %v", tokens[4])
	}
}

func TestLexRange(t *testing.T) {
	tokens, err := Lex("priority:1..10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: Ident("priority"), Colon, Number(1), DotDot, Number(10), EOF
	if len(tokens) != 6 {
		t.Fatalf("expected 6 tokens (including EOF), got %d: %v", len(tokens), tokens)
	}
	if tokens[0].Kind != TokIdent || tokens[0].Value != "priority" {
		t.Errorf("expected Ident(priority), got %v", tokens[0])
	}
	if tokens[1].Kind != TokColon {
		t.Errorf("expected Colon, got %v", tokens[1])
	}
	if tokens[2].Kind != TokNumber || tokens[2].Num != 1 {
		t.Errorf("expected Number(1), got %v", tokens[2])
	}
	if tokens[3].Kind != TokDotDot {
		t.Errorf("expected DotDot, got %v", tokens[3])
	}
	if tokens[4].Kind != TokNumber || tokens[4].Num != 10 {
		t.Errorf("expected Number(10), got %v", tokens[4])
	}
	if tokens[5].Kind != TokEOF {
		t.Errorf("expected EOF, got %v", tokens[5])
	}
}

func TestLexNOT(t *testing.T) {
	tokens, err := Lex("NOT archived")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].Kind != TokNot {
		t.Errorf("expected Not, got %v", tokens[0])
	}
}

func TestLexExclamation(t *testing.T) {
	tokens, err := Lex("!done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].Kind != TokNot {
		t.Errorf("expected Not, got %v", tokens[0])
	}
	if tokens[1].Kind != TokIdent || tokens[1].Value != "done" {
		t.Errorf("expected Ident(done), got %v", tokens[1])
	}
}

func TestLexComparisons(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenKind
	}{
		{"a>5", TokGt},
		{"a>=5", TokGte},
		{"a<5", TokLt},
		{"a<=5", TokLte},
	}
	for _, tt := range tests {
		tokens, err := Lex(tt.input)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tt.input, err)
		}
		if tokens[1].Kind != tt.expected {
			t.Errorf("for %s: expected %v, got %v", tt.input, tt.expected, tokens[1].Kind)
		}
	}
}

func TestLexEscapedString(t *testing.T) {
	tokens, err := Lex(`"hello\nworld"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].Kind != TokString || tokens[0].Value != "hello\nworld" {
		t.Errorf("expected String(hello\\nworld), got %v", tokens[0])
	}
}
