package jsonql_test

import (
	"testing"

	"github.com/jsonql-standard/jsonql-go"
)

func TestParser_BasicQuery(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"fields": []interface{}{"id", "name"},
	}
	q, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(q.Fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(q.Fields))
	}
}

func TestParser_EmptyQuery(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{}
	_, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("Empty query should be valid: %v", err)
	}
}

func TestParser_EmptyFieldsRejected(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"fields": []interface{}{},
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("Empty fields array should be rejected")
	}
}

func TestParser_UnknownPropertyRejected(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"unknown_key": "value",
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("Unknown property should be rejected")
	}
}

func TestParser_NegativeLimit(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"limit": float64(-1),
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("Negative limit should be rejected")
	}
}

func TestParser_NegativeSkip(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"skip": float64(-5),
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("Negative skip should be rejected")
	}
}

func TestParser_ValidSort(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"sort": []interface{}{"name", "-created_at"},
	}
	q, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(q.Sort) != 2 {
		t.Errorf("Expected 2 sort fields, got %d", len(q.Sort))
	}
}

func TestParser_WhereOperators(t *testing.T) {
	tests := []struct {
		name  string
		where map[string]interface{}
	}{
		{"eq", map[string]interface{}{"status": map[string]interface{}{"eq": "active"}}},
		{"neq", map[string]interface{}{"status": map[string]interface{}{"neq": "deleted"}}},
		{"gt", map[string]interface{}{"age": map[string]interface{}{"gt": float64(18)}}},
		{"gte", map[string]interface{}{"age": map[string]interface{}{"gte": float64(18)}}},
		{"lt", map[string]interface{}{"age": map[string]interface{}{"lt": float64(65)}}},
		{"lte", map[string]interface{}{"age": map[string]interface{}{"lte": float64(65)}}},
		{"in", map[string]interface{}{"role": map[string]interface{}{"in": []interface{}{"admin", "mod"}}}},
		{"like", map[string]interface{}{"name": map[string]interface{}{"like": "%alice%"}}},
	}

	p := jsonql.NewParser()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]interface{}{"where": tc.where}
			_, err := p.Parse(input, nil, "users")
			if err != nil {
				t.Fatalf("Unexpected error for operator %s: %v", tc.name, err)
			}
		})
	}
}

func TestParser_WhereFieldReference(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"where": map[string]interface{}{
			"price": map[string]interface{}{
				"gt": map[string]interface{}{"field": "cost"},
			},
		},
	}
	_, err := p.Parse(input, nil, "products")
	if err != nil {
		t.Fatalf("Field reference should be valid: %v", err)
	}
}

func TestParser_WhereLogicalOperators(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"where": map[string]interface{}{
			"and": []interface{}{
				map[string]interface{}{"status": map[string]interface{}{"eq": "active"}},
				map[string]interface{}{"age": map[string]interface{}{"gt": float64(18)}},
			},
		},
	}
	_, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("AND clause should be valid: %v", err)
	}
}

func TestParser_GroupBy(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"groupBy": []interface{}{"status"},
	}
	q, err := p.Parse(input, nil, "orders")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(q.GroupBy) != 1 || q.GroupBy[0] != "status" {
		t.Errorf("Expected groupBy [status], got %v", q.GroupBy)
	}
}

func TestParser_Aggregate(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"aggregate": map[string]interface{}{
			"total": map[string]interface{}{"count": "id"},
		},
	}
	q, err := p.Parse(input, nil, "orders")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if q.Aggregate == nil {
		t.Fatal("Expected aggregate to be set")
	}
}

func TestParser_UnknownAggregateFunction(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"aggregate": map[string]interface{}{
			"med": map[string]interface{}{"median": "age"},
		},
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("Unknown aggregate function should be rejected")
	}
}

func TestParser_DistinctBoolean(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"distinct": true,
		"fields":   []interface{}{"name"},
	}
	q, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if q.Distinct == nil || !q.Distinct.All {
		t.Error("Expected Distinct.All to be true")
	}
}

func TestParser_DistinctArray(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"distinct": []interface{}{"category", "status"},
	}
	q, err := p.Parse(input, nil, "products")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if q.Distinct == nil || len(q.Distinct.Fields) != 2 {
		t.Errorf("Expected 2 distinct fields, got %v", q.Distinct)
	}
}

// ---------- Options enforcement ----------

func TestParser_MaxLimitEnforced(t *testing.T) {
	p := jsonql.NewParserWithOptions(&jsonql.ParserOptions{MaxLimit: 50})
	input := map[string]interface{}{
		"limit": float64(100),
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("MaxLimit should be enforced")
	}
}

func TestParser_MaxLimitPermitsValid(t *testing.T) {
	p := jsonql.NewParserWithOptions(&jsonql.ParserOptions{MaxLimit: 50})
	input := map[string]interface{}{
		"limit": float64(25),
	}
	_, err := p.Parse(input, nil, "users")
	if err != nil {
		t.Fatalf("Limit 25 should be valid with MaxLimit 50: %v", err)
	}
}

func TestParser_AllowedFieldsEnforced(t *testing.T) {
	p := jsonql.NewParserWithOptions(&jsonql.ParserOptions{
		AllowedFields: []string{"id", "name"},
	})
	input := map[string]interface{}{
		"fields": []interface{}{"id", "email"},
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("AllowedFields should be enforced")
	}
}

func TestParser_AllowedIncludesEnforced(t *testing.T) {
	p := jsonql.NewParserWithOptions(&jsonql.ParserOptions{
		AllowedIncludes: []string{"posts"},
	})
	input := map[string]interface{}{
		"include": map[string]interface{}{
			"sessions": map[string]interface{}{},
		},
	}
	_, err := p.Parse(input, nil, "users")
	if err == nil {
		t.Fatal("AllowedIncludes should be enforced")
	}
}

// ---------- Complex query ----------

func TestParser_ComplexQuery(t *testing.T) {
	p := jsonql.NewParser()
	input := map[string]interface{}{
		"fields":  []interface{}{"status", "total"},
		"limit":   float64(100),
		"skip":    float64(0),
		"sort":    []interface{}{"-total"},
		"groupBy": []interface{}{"status"},
		"aggregate": map[string]interface{}{
			"total": map[string]interface{}{"sum": "amount"},
		},
		"where": map[string]interface{}{
			"status": map[string]interface{}{
				"in": []interface{}{"active", "pending"},
			},
		},
	}
	_, err := p.Parse(input, nil, "orders")
	if err != nil {
		t.Fatalf("Complex query should be valid: %v", err)
	}
}
