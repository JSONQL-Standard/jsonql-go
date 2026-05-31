package jsonql_test

import (
	"testing"

	"github.com/jsonql-standard/jsonql-go"
)

func TestMongoTranspiler_BasicFind(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Fields: []string{"id", "name"},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Collection != "users" {
		t.Errorf("Expected collection 'users', got '%s'", result.Collection)
	}
	if result.Operation != "find" {
		t.Errorf("Expected operation 'find', got '%s'", result.Operation)
	}
	if len(result.Projection) != 2 {
		t.Errorf("Expected 2 projection fields, got %d", len(result.Projection))
	}
}

func TestMongoTranspiler_WhereEq(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Where: map[string]interface{}{
			"status": map[string]interface{}{"eq": "active"},
		},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Filter["status"] == nil {
		t.Error("Expected filter on status")
	}
}

func TestMongoTranspiler_WhereComparison(t *testing.T) {
	tests := []struct {
		name string
		op   string
		val  interface{}
	}{
		{"gt", "gt", float64(18)},
		{"gte", "gte", float64(18)},
		{"lt", "lt", float64(65)},
		{"lte", "lte", float64(65)},
		{"neq", "neq", "deleted"},
	}

	mt := jsonql.NewMongoTranspiler()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := &jsonql.JSONQLQuery{
				Where: map[string]interface{}{
					"age": map[string]interface{}{tc.op: tc.val},
				},
			}
			_, err := mt.Transpile(q, "users")
			if err != nil {
				t.Fatalf("Unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

func TestMongoTranspiler_WhereIn(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Where: map[string]interface{}{
			"role": map[string]interface{}{
				"in": []interface{}{"admin", "mod"},
			},
		},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Filter["role"] == nil {
		t.Error("Expected filter on role")
	}
}

func TestMongoTranspiler_WhereLike(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Where: map[string]interface{}{
			"name": map[string]interface{}{"like": "%alice%"},
		},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Filter["name"] == nil {
		t.Error("Expected filter on name")
	}
}

func TestMongoTranspiler_ContainsEscapesRegexMeta(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	cases := []struct {
		op   string
		in   string
		want string
	}{
		{"contains", "a.b*", "a\\.b\\*"},
		{"starts", "a.", "^a\\."},
		{"ends", ".b", "\\.b$"},
	}
	for _, c := range cases {
		q := &jsonql.JSONQLQuery{
			Where: map[string]interface{}{
				"name": map[string]interface{}{c.op: c.in},
			},
		}
		result, err := mt.Transpile(q, "users")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.op, err)
		}
		nameFilter, ok := result.Filter["name"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: expected map filter on name, got %T", c.op, result.Filter["name"])
		}
		if got := nameFilter["$regex"]; got != c.want {
			t.Errorf("%s: expected $regex %q, got %q", c.op, c.want, got)
		}
	}
}

func TestMongoTranspiler_Sort(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Sort: []string{"name", "-created_at"},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Sort == nil {
		t.Fatal("Expected sort to be set")
	}
	if result.Sort["name"] != 1 {
		t.Errorf("Expected name ascending, got %v", result.Sort["name"])
	}
	if result.Sort["created_at"] != -1 {
		t.Errorf("Expected created_at descending, got %v", result.Sort["created_at"])
	}
}

func TestMongoTranspiler_LimitSkip(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	limit := 10
	offset := 20
	q := &jsonql.JSONQLQuery{
		Limit:  &limit,
		Offset: &offset,
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Limit != 10 {
		t.Errorf("Expected limit 10, got %d", result.Limit)
	}
	if result.Skip != 20 {
		t.Errorf("Expected skip 20, got %d", result.Skip)
	}
}

func TestMongoTranspiler_LogicalAnd(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Where: map[string]interface{}{
			"and": []interface{}{
				map[string]interface{}{"status": map[string]interface{}{"eq": "active"}},
				map[string]interface{}{"age": map[string]interface{}{"gt": float64(18)}},
			},
		},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Filter["$and"] == nil {
		t.Error("Expected $and in filter")
	}
}

func TestMongoTranspiler_LogicalOr(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{
		Where: map[string]interface{}{
			"or": []interface{}{
				map[string]interface{}{"role": map[string]interface{}{"eq": "admin"}},
				map[string]interface{}{"role": map[string]interface{}{"eq": "mod"}},
			},
		},
	}
	result, err := mt.Transpile(q, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Filter["$or"] == nil {
		t.Error("Expected $or in filter")
	}
}

func TestMongoTranspiler_InvalidCollection(t *testing.T) {
	mt := jsonql.NewMongoTranspiler()
	q := &jsonql.JSONQLQuery{}
	_, err := mt.Transpile(q, "drop()--")
	if err == nil {
		t.Fatal("Expected error for invalid collection name")
	}
}
