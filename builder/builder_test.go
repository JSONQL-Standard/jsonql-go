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

// ---------- v1.1 feature tests ----------

func TestBuilder_GroupBy(t *testing.T) {
	q := New().
		From("orders").
		GroupBy("status", "category").
		Build()

	if len(q.GroupBy) != 2 {
		t.Errorf("Expected 2 groupBy fields, got %d", len(q.GroupBy))
	}
	if q.GroupBy[0] != "status" {
		t.Errorf("Expected first groupBy 'status', got '%s'", q.GroupBy[0])
	}
}

func TestBuilder_Aggregate(t *testing.T) {
	q := New().
		From("orders").
		GroupBy("status").
		Aggregate(map[string]interface{}{
			"total": map[string]interface{}{"count": "id"},
		}).
		Build()

	if q.Aggregate == nil {
		t.Fatal("Expected Aggregate to be set")
	}
	if _, ok := q.Aggregate["total"]; !ok {
		t.Error("Expected 'total' key in aggregate")
	}
}

func TestBuilder_DistinctAll(t *testing.T) {
	q := New().
		From("users").
		Select("name").
		Distinct(true).
		Build()

	if q.Distinct == nil {
		t.Fatal("Expected Distinct to be set")
	}
	if !q.Distinct.All {
		t.Error("Expected Distinct.All to be true")
	}
}

func TestBuilder_DistinctFields(t *testing.T) {
	q := New().
		From("products").
		DistinctFields("category", "status").
		Build()

	if q.Distinct == nil {
		t.Fatal("Expected Distinct to be set")
	}
	if len(q.Distinct.Fields) != 2 {
		t.Errorf("Expected 2 distinct fields, got %d", len(q.Distinct.Fields))
	}
	if q.Distinct.Fields[0] != "category" {
		t.Errorf("Expected first distinct field 'category', got '%s'", q.Distinct.Fields[0])
	}
}

func TestBuilder_Include(t *testing.T) {
	q := New().
		From("users").
		Include(map[string]interface{}{
			"posts": map[string]interface{}{
				"fields": []string{"id", "title"},
			},
		}).
		Build()

	if q.Include == nil {
		t.Fatal("Expected Include to be set")
	}
	if _, ok := q.Include["posts"]; !ok {
		t.Error("Expected 'posts' key in include")
	}
}

func TestBuilder_Reset(t *testing.T) {
	b := New().From("users").Select("id", "name").Limit(10)
	q1 := b.Build()
	if q1.From != "users" {
		t.Errorf("Before reset: expected from 'users', got '%s'", q1.From)
	}

	b.Reset()
	q2 := b.Build()
	if q2.From != "" {
		t.Errorf("After reset: expected empty from, got '%s'", q2.From)
	}
	if len(q2.Fields) != 0 {
		t.Errorf("After reset: expected 0 fields, got %d", len(q2.Fields))
	}
}

// ---------- Condition helpers ----------

func TestCondition_Eq(t *testing.T) {
	c := Eq("active")
	if c["eq"] != "active" {
		t.Errorf("Expected eq=active, got %v", c["eq"])
	}
}

func TestCondition_Neq(t *testing.T) {
	c := Neq("deleted")
	if c["neq"] != "deleted" {
		t.Errorf("Expected neq=deleted, got %v", c["neq"])
	}
}

func TestCondition_GtLt(t *testing.T) {
	if Gt(10)["gt"] != 10 {
		t.Error("Gt failed")
	}
	if Gte(10)["gte"] != 10 {
		t.Error("Gte failed")
	}
	if Lt(10)["lt"] != 10 {
		t.Error("Lt failed")
	}
	if Lte(10)["lte"] != 10 {
		t.Error("Lte failed")
	}
}

func TestCondition_In(t *testing.T) {
	c := In("a", "b", "c")
	vals, ok := c["in"].([]interface{})
	if !ok {
		t.Fatal("Expected in to be slice")
	}
	if len(vals) != 3 {
		t.Errorf("Expected 3 values, got %d", len(vals))
	}
}

func TestCondition_Like(t *testing.T) {
	c := Like("%test%")
	if c["like"] != "%test%" {
		t.Errorf("Expected like=%%test%%, got %v", c["like"])
	}
}

func TestCondition_Contains(t *testing.T) {
	c := Contains("alice")
	if c["like"] != "%alice%" {
		t.Errorf("Expected like=%%alice%%, got %v", c["like"])
	}
}

func TestCondition_StartsWith(t *testing.T) {
	c := StartsWith("hello")
	if c["like"] != "hello%" {
		t.Errorf("Expected like=hello%%, got %v", c["like"])
	}
}

func TestCondition_EndsWith(t *testing.T) {
	c := EndsWith("world")
	if c["like"] != "%world" {
		t.Errorf("Expected like=%%world, got %v", c["like"])
	}
}

func TestCondition_Field(t *testing.T) {
	c := Field("name", Eq("Alice"))
	inner, ok := c["name"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected name key")
	}
	if inner["eq"] != "Alice" {
		t.Errorf("Expected eq=Alice, got %v", inner["eq"])
	}
}

func TestCondition_AndOr(t *testing.T) {
	c := And(Field("a", Eq(1)), Field("b", Eq(2)))
	ands, ok := c["and"].([]interface{})
	if !ok {
		t.Fatal("Expected and key")
	}
	if len(ands) != 2 {
		t.Errorf("Expected 2 AND conditions, got %d", len(ands))
	}

	o := Or(Field("x", Eq(1)), Field("y", Eq(2)))
	ors, ok := o["or"].([]interface{})
	if !ok {
		t.Fatal("Expected or key")
	}
	if len(ors) != 2 {
		t.Errorf("Expected 2 OR conditions, got %d", len(ors))
	}
}

func TestCondition_Not(t *testing.T) {
	c := Not(Field("status", Eq("deleted")))
	if c["not"] == nil {
		t.Error("Expected not key")
	}
}

func TestCondition_FieldRef(t *testing.T) {
	c := FieldRef("cost")
	if c["field"] != "cost" {
		t.Errorf("Expected field=cost, got %v", c["field"])
	}
}

func TestCondition_FieldRefInWhere(t *testing.T) {
	q := New().
		From("products").
		Where(Field("price", map[string]interface{}{
			"gt": FieldRef("cost"),
		})).
		Build()

	if q.Where == nil {
		t.Fatal("Expected Where clause")
	}
	price, ok := q.Where["price"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected price condition")
	}
	gt, ok := price["gt"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected gt condition map")
	}
	if gt["field"] != "cost" {
		t.Errorf("Expected field ref 'cost', got %v", gt["field"])
	}
}

// ---------- Complex builder query ----------

func TestBuilder_ComplexQuery(t *testing.T) {
	q := New().
		From("orders").
		Select("status").
		Where(Field("status", In("active", "pending"))).
		GroupBy("status").
		Aggregate(map[string]interface{}{
			"total_orders":  map[string]interface{}{"count": "id"},
			"total_revenue": map[string]interface{}{"sum": "total"},
		}).
		OrderBy("status").
		Limit(100).
		Build()

	if q.From != "orders" {
		t.Errorf("Expected from 'orders', got '%s'", q.From)
	}
	if len(q.Fields) != 1 || q.Fields[0] != "status" {
		t.Errorf("Fields mismatch: %v", q.Fields)
	}
	if q.Where == nil {
		t.Error("Expected Where clause")
	}
	if len(q.GroupBy) != 1 {
		t.Error("Expected GroupBy")
	}
	if q.Aggregate == nil {
		t.Error("Expected Aggregate")
	}
	if *q.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", *q.Limit)
	}
}

// ---------- Mutation builder ----------

func TestMutationBuilder_Create(t *testing.T) {
	b := NewMutation()
	m, err := b.Create("users", map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}).Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if m.Op != "create" {
		t.Errorf("Expected op 'create', got '%s'", m.Op)
	}
	if m.Data["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got '%v'", m.Data["name"])
	}
}

func TestMutationBuilder_Update(t *testing.T) {
	m, err := NewMutation().
		Update("users", map[string]interface{}{"name": "Bob"}).
		Where(map[string]interface{}{"id": map[string]interface{}{"eq": 1}}).
		Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if m.Op != "update" {
		t.Errorf("Expected op 'update', got '%s'", m.Op)
	}
	if m.Where == nil {
		t.Error("Expected Where clause")
	}
}

func TestMutationBuilder_Delete(t *testing.T) {
	m, err := NewMutation().
		Delete("users").
		Where(map[string]interface{}{"id": map[string]interface{}{"eq": 1}}).
		Build()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if m.Op != "delete" {
		t.Errorf("Expected op 'delete', got '%s'", m.Op)
	}
}

func TestMutationBuilder_BuildWithoutInit(t *testing.T) {
	_, err := NewMutation().Build()
	if err == nil {
		t.Fatal("Expected error for build without init")
	}
}

func TestMutationBuilder_Reset(t *testing.T) {
	b := NewMutation()
	b.Create("users", map[string]interface{}{"name": "Alice"})
	b.Reset()
	_, err := b.Build()
	if err == nil {
		t.Fatal("Expected error after reset")
	}
}
