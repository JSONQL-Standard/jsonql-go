package jsonql

import (
	"fmt"
	"strings"
)

// MongoResult holds the generated MongoDB operation details
type MongoResult struct {
	Collection string
	Operation  string // "find", "insertOne", "updateMany", "deleteMany", "aggregate"
	Filter     map[string]interface{}
	Projection map[string]interface{}
	Sort       map[string]interface{}
	Limit      int64
	Skip       int64
	Pipeline   []map[string]interface{} // for aggregate
	Document   map[string]interface{}   // for insertOne
	Update     map[string]interface{}   // for updateMany ($set)
}

// MongoTranspiler converts JSONQL queries to MongoDB operations
type MongoTranspiler struct{}

// NewMongoTranspiler creates a new MongoDB transpiler
func NewMongoTranspiler() *MongoTranspiler {
	return &MongoTranspiler{}
}

// Transpile converts a JSONQL query to a MongoResult
func (t *MongoTranspiler) Transpile(query *JSONQLQuery, collection string) (*MongoResult, error) {
	if !isValidIdentifier(collection) {
		return nil, fmt.Errorf("Invalid collection name: %s", collection)
	}

	result := &MongoResult{
		Collection: collection,
		Operation:  "find",
		Filter:     make(map[string]interface{}),
	}

	// 1. WHERE → filter
	if query.Where != nil {
		filter, err := t.processWhere(query.Where)
		if err != nil {
			return nil, err
		}
		result.Filter = filter
	}

	// 2. FIELDS → projection
	if len(query.Fields) > 0 {
		projection := make(map[string]interface{})
		for _, f := range query.Fields {
			if !isValidIdentifier(f) {
				return nil, fmt.Errorf("Invalid field name: %s", f)
			}
			projection[f] = 1
		}
		result.Projection = projection
	}

	// 3. SORT → sort
	if len(query.Sort) > 0 {
		sortDoc := make(map[string]interface{})
		for _, s := range query.Sort {
			field := s
			order := 1
			if strings.HasPrefix(s, "-") {
				field = s[1:]
				order = -1
			}
			if !isValidIdentifier(field) {
				return nil, fmt.Errorf("Invalid sort field: %s", field)
			}
			sortDoc[field] = order
		}
		result.Sort = sortDoc
	}

	// 4. LIMIT
	if query.Limit != nil && *query.Limit > 0 {
		result.Limit = int64(*query.Limit)
	}

	// 5. OFFSET → skip
	if query.Offset != nil && *query.Offset > 0 {
		result.Skip = int64(*query.Offset)
	}

	// 6. AGGREGATE → aggregation pipeline
	if len(query.Aggregate) > 0 {
		result.Operation = "aggregate"
		pipeline := []map[string]interface{}{}

		// Match stage (from WHERE)
		if len(result.Filter) > 0 {
			pipeline = append(pipeline, map[string]interface{}{"$match": result.Filter})
		}

		// Group stage
		groupStage := map[string]interface{}{}
		if len(query.GroupBy) > 0 {
			groupId := make(map[string]interface{})
			for _, g := range query.GroupBy {
				if !isValidIdentifier(g) {
					return nil, fmt.Errorf("Invalid group by field: %s", g)
				}
				groupId[g] = "$" + g
			}
			groupStage["_id"] = groupId
			// Also project group fields
			for _, g := range query.GroupBy {
				groupStage[g] = map[string]interface{}{"$first": "$" + g}
			}
		} else {
			groupStage["_id"] = nil
		}

		for alias, aggDef := range query.Aggregate {
			aggMap, ok := aggDef.(map[string]interface{})
			if !ok {
				continue
			}
			for funcName, field := range aggMap {
				fieldName, ok := field.(string)
				if !ok {
					continue
				}
				switch funcName {
				case "count":
					if fieldName == "*" {
						groupStage[alias] = map[string]interface{}{"$sum": 1}
					} else {
						groupStage[alias] = map[string]interface{}{"$sum": map[string]interface{}{
							"$cond": []interface{}{
								map[string]interface{}{"$ne": []interface{}{"$" + fieldName, nil}},
								1, 0,
							},
						}}
					}
				case "sum":
					groupStage[alias] = map[string]interface{}{"$sum": "$" + fieldName}
				case "avg":
					groupStage[alias] = map[string]interface{}{"$avg": "$" + fieldName}
				case "min":
					groupStage[alias] = map[string]interface{}{"$min": "$" + fieldName}
				case "max":
					groupStage[alias] = map[string]interface{}{"$max": "$" + fieldName}
				default:
					return nil, fmt.Errorf("unknown aggregate function: %s", funcName)
				}
			}
		}
		pipeline = append(pipeline, map[string]interface{}{"$group": groupStage})

		// Sort stage (for aggregation)
		if len(query.Sort) > 0 && result.Sort != nil {
			pipeline = append(pipeline, map[string]interface{}{"$sort": result.Sort})
		}

		// Skip/Limit stages
		if result.Skip > 0 {
			pipeline = append(pipeline, map[string]interface{}{"$skip": result.Skip})
		}
		if result.Limit > 0 {
			pipeline = append(pipeline, map[string]interface{}{"$limit": result.Limit})
		}

		result.Pipeline = pipeline
	}

	return result, nil
}

// TranspileInsert converts insert data to a MongoResult
func (t *MongoTranspiler) TranspileInsert(collection string, data map[string]interface{}) (*MongoResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("insert data cannot be empty")
	}
	if !isValidIdentifier(collection) {
		return nil, fmt.Errorf("invalid collection name: %s", collection)
	}
	return &MongoResult{
		Collection: collection,
		Operation:  "insertOne",
		Document:   data,
	}, nil
}

// TranspileUpdate converts update data to a MongoResult
func (t *MongoTranspiler) TranspileUpdate(collection string, patch map[string]interface{}, where map[string]interface{}) (*MongoResult, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("update patch cannot be empty")
	}
	if !isValidIdentifier(collection) {
		return nil, fmt.Errorf("invalid collection name: %s", collection)
	}
	filter := make(map[string]interface{})
	if where != nil {
		var err error
		filter, err = t.processWhere(where)
		if err != nil {
			return nil, err
		}
	}
	return &MongoResult{
		Collection: collection,
		Operation:  "updateMany",
		Filter:     filter,
		Update:     map[string]interface{}{"$set": patch},
	}, nil
}

// TranspileDelete converts delete criteria to a MongoResult
func (t *MongoTranspiler) TranspileDelete(collection string, where map[string]interface{}) (*MongoResult, error) {
	if !isValidIdentifier(collection) {
		return nil, fmt.Errorf("invalid collection name: %s", collection)
	}
	filter := make(map[string]interface{})
	if where != nil {
		var err error
		filter, err = t.processWhere(where)
		if err != nil {
			return nil, err
		}
	}
	return &MongoResult{
		Collection: collection,
		Operation:  "deleteMany",
		Filter:     filter,
	}, nil
}

// processWhere converts JSONQL where conditions to MongoDB filter
func (t *MongoTranspiler) processWhere(where map[string]interface{}) (map[string]interface{}, error) {
	filter := make(map[string]interface{})

	for field, cond := range where {
		if field == "or" {
			if orList, ok := cond.([]interface{}); ok {
				orConditions := make([]interface{}, 0)
				for _, item := range orList {
					if itemMap, ok := item.(map[string]interface{}); ok {
						subFilter, err := t.processWhere(itemMap)
						if err != nil {
							return nil, err
						}
						orConditions = append(orConditions, subFilter)
					}
				}
				if len(orConditions) > 0 {
					filter["$or"] = orConditions
				}
			}
			continue
		}

		if !isValidIdentifier(field) {
			return nil, fmt.Errorf("Invalid field name in where clause: %s", field)
		}

		if valMap, ok := cond.(map[string]interface{}); ok {
			mongoOp := make(map[string]interface{})
			if v, ok := valMap["eq"]; ok {
				filter[field] = v
				continue
			}
			if v, ok := valMap["neq"]; ok {
				mongoOp["$ne"] = v
			}
			if v, ok := valMap["gt"]; ok {
				mongoOp["$gt"] = v
			}
			if v, ok := valMap["gte"]; ok {
				mongoOp["$gte"] = v
			}
			if v, ok := valMap["lt"]; ok {
				mongoOp["$lt"] = v
			}
			if v, ok := valMap["lte"]; ok {
				mongoOp["$lte"] = v
			}
			if v, ok := valMap["like"]; ok {
				// Convert SQL LIKE to MongoDB regex
				if s, ok := v.(string); ok {
					pattern := strings.ReplaceAll(s, "%", ".*")
					pattern = strings.ReplaceAll(pattern, "_", ".")
					mongoOp["$regex"] = pattern
					mongoOp["$options"] = "i"
				}
			}
			if v, ok := valMap["in"]; ok {
				if slice, ok := v.([]interface{}); ok {
					mongoOp["$in"] = slice
				}
			}
			if len(mongoOp) > 0 {
				filter[field] = mongoOp
			}
		} else {
			filter[field] = cond
		}
	}

	return filter, nil
}
