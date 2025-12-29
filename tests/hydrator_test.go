package jsonql_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jsonql-standard/jsonql-go"
)

func TestHydrator_MergeRows(t *testing.T) {
	schemaJSON := `{
		"tables": {
			"users": {
				"fields": {
					"id":   { "type": "number" },
					"name": { "type": "string" }
				},
				"relations": {
					"posts": { "type": "hasMany", "table": "posts", "field": "user_id" },
					"profile": { "type": "hasOne", "table": "profiles", "field": "user_id" }
				}
			},
			"posts": {
				"fields": {
					"id":      { "type": "number" },
					"title":   { "type": "string" },
					"user_id": { "type": "number" }
				},
				"relations": {
					"comments": { "type": "hasMany", "table": "comments", "field": "post_id" }
				}
			},
			"profiles": {
				"fields": {
					"id":  { "type": "number" },
					"bio": { "type": "string" }
				}
			},
			"comments": {
				"fields": {
					"id":   { "type": "number" },
					"text": { "type": "string" }
				}
			}
		}
	}`
	var schema jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		t.Fatalf("Failed to parse schema: %v", err)
	}

	hydrator := jsonql.NewHydrator()

	tests := []struct {
		name     string
		input    []map[string]interface{}
		root     string
		expected []map[string]interface{}
	}{
		{
			name: "Basic Single Row",
			root: "users",
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice", "posts": []map[string]interface{}{}, "profile": nil},
			},
		},
		{
			name: "HasMany Relation (Posts)",
			root: "users",
			input: []map[string]interface{}{
				{
					"id": 1, "name": "Alice",
					"posts": map[string]interface{}{"id": 101, "title": "Post 1"},
				},
				{
					"id": 1, "name": "Alice",
					"posts": map[string]interface{}{"id": 102, "title": "Post 2"},
				},
			},
			expected: []map[string]interface{}{
				{
					"id":   1,
					"name": "Alice",
					"posts": []map[string]interface{}{
						{"id": 101, "title": "Post 1"},
						{"id": 102, "title": "Post 2"},
					},
					"profile": nil,
				},
			},
		},
		{
			name: "HasOne Relation (Profile)",
			root: "users",
			input: []map[string]interface{}{
				{
					"id": 1, "name": "Alice",
					"profile": map[string]interface{}{"id": 501, "bio": "Developer"},
				},
			},
			expected: []map[string]interface{}{
				{
					"id":   1,
					"name": "Alice",
					"profile": map[string]interface{}{
						"id": 501, "bio": "Developer",
					},
					"posts": []map[string]interface{}{},
				},
			},
		},
		{
			name: "Nested Relations (User -> Posts -> Comments)",
			root: "users",
			input: []map[string]interface{}{
				{
					"id": 1, "name": "Alice",
					"posts": map[string]interface{}{
						"id": 101, "title": "Post 1",
						"comments": map[string]interface{}{"id": 901, "text": "Nice"},
					},
				},
				{
					"id": 1, "name": "Alice",
					"posts": map[string]interface{}{
						"id": 101, "title": "Post 1",
						"comments": map[string]interface{}{"id": 902, "text": "Cool"},
					},
				},
			},
			expected: []map[string]interface{}{
				{
					"id":   1,
					"name": "Alice",
					"posts": []map[string]interface{}{
						{
							"id":    101,
							"title": "Post 1",
							"comments": []map[string]interface{}{
								{"id": 901, "text": "Nice"},
								{"id": 902, "text": "Cool"},
							},
						},
					},
					"profile": nil,
				},
			},
		},
		{
			name: "Aggregates (Posts Count)",
			root: "users",
			input: []map[string]interface{}{
				{
					"id": 1, "name": "Alice",
					"posts": map[string]interface{}{"count": 5},
				},
			},
			expected: []map[string]interface{}{
				{
					"id":   1,
					"name": "Alice",
					"posts": map[string]interface{}{"count": 5},
					"profile": nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hydrator.MergeRows(tt.input, &schema, tt.root)

			// Use JSON comparison for easier map equality check
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.expected)

			var gotMap, wantMap interface{}
			json.Unmarshal(gotJSON, &gotMap)
			json.Unmarshal(wantJSON, &wantMap)

			if !reflect.DeepEqual(gotMap, wantMap) {
				t.Errorf("MergeRows() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
