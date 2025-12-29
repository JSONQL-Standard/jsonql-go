package builder

import (
	"testing"
)

func TestBuilder_Basic(t *testing.T) {
	q := New().
		From("users").
		Select("id", "name").
		Build()

	if q.From != "users" {
		t.Errorf("Expected From 'users', got '%s'", q.From)
	}
	if len(q.Fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(q.Fields))
	}
	if q.Fields[0] != "id" || q.Fields[1] != "name" {
		t.Errorf("Fields mismatch: %v", q.Fields)
	}
}

func TestBuilder_Where(t *testing.T) {
	q := New().
		From("users").
		Where(map[string]interface{}{"age": map[string]interface{}{"gt": 18}}).
		Build()

	if q.Where == nil {
		t.Fatal("Expected Where clause")
	}
	age, ok := q.Where["age"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected age condition")
	}
	if age["gt"] != 18 {
		t.Errorf("Expected age > 18, got %v", age["gt"])
	}
}

func TestBuilder_AndWhere(t *testing.T) {
	q := New().
		From("users").
		Where(map[string]interface{}{"active": true}).
		AndWhere(map[string]interface{}{"age": map[string]interface{}{"gt": 18}}).
		Build()

	if q.Where["and"] == nil {
		t.Fatal("Expected AND clause")
	}
	ands := q.Where["and"].([]interface{})
	if len(ands) != 2 {
		t.Errorf("Expected 2 AND conditions, got %d", len(ands))
	}
}

func TestBuilder_OrWhere(t *testing.T) {
	q := New().
		From("users").
		Where(map[string]interface{}{"role": "admin"}).
		OrWhere(map[string]interface{}{"role": "moderator"}).
		Build()

	if q.Where["or"] == nil {
		t.Fatal("Expected OR clause")
	}
	ors := q.Where["or"].([]interface{})
	if len(ors) != 2 {
		t.Errorf("Expected 2 OR conditions, got %d", len(ors))
	}
}

func TestBuilder_Pagination(t *testing.T) {
	q := New().
		From("posts").
		Limit(10).
		Offset(20).
		Build()

	if *q.Limit != 10 {
		t.Errorf("Expected Limit 10, got %d", *q.Limit)
	}
	if *q.Offset != 20 {
		t.Errorf("Expected Offset 20, got %d", *q.Offset)
	}
}

func TestBuilder_OrderBy(t *testing.T) {
	q := New().
		From("posts").
		OrderBy("created_at", "-title").
		Build()

	if len(q.Sort) != 2 {
		t.Errorf("Expected 2 sort fields, got %d", len(q.Sort))
	}
	if q.Sort[0] != "created_at" {
		t.Errorf("Expected first sort 'created_at', got '%s'", q.Sort[0])
	}
}
