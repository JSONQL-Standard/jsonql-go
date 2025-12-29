package jsonql_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jsonql-standard/jsonql-go"
)

func TestHydrator_MergeRows(t *testing.T) {
	schema := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":   {Type: "number"},
					"name": {Type: "string"},
				},
				Relations: map[string]*jsonql.JSONQLRelation{
					"posts": {Type: "hasMany", Table: "posts", Field: "user_id"},
					"profile": {Type: "hasOne", Table: "profiles", Field: "user_id"},
				},
			},
			"posts": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":      {Type: "number"},
					"title":   {Type: "string"},
					"user_id": {Type: "number"},
				},
				Relations: map[string]*jsonql.JSONQLRelation{
					"comments": {Type: "hasMany", Table: "comments", Field: "post_id"},
				},
			},
			"profiles": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":  {Type: "number"},
					"bio": {Type: "string"},
				},
			},
			"comments": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":   {Type: "number"},
					"text": {Type: "string"},
				},
			},
		},
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
			got := hydrator.MergeRows(tt.input, schema, tt.root)

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
