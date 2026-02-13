package mongodb

import (
	"context"
	"fmt"

	"github.com/jsonql-standard/jsonql-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Driver implements MongoDB operations for JSONQL.
type Driver struct {
	client   *mongo.Client
	database *mongo.Database
}

// NewDriver creates a new MongoDB driver.
// Example URI: "mongodb://localhost:27017"
func NewDriver(uri, dbName string) (*Driver, error) {
	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(context.Background(), clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		return nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}
	return &Driver{
		client:   client,
		database: client.Database(dbName),
	}, nil
}

// ExecuteFind executes a find operation from a MongoResult.
func (d *Driver) ExecuteFind(ctx context.Context, result *jsonql.MongoResult) ([]map[string]interface{}, error) {
	coll := d.database.Collection(result.Collection)
	filter := toBSON(result.Filter)
	opts := options.Find()
	if result.Projection != nil {
		opts.SetProjection(toBSON(result.Projection))
	}
	if result.Sort != nil {
		opts.SetSort(toBSON(result.Sort))
	}
	if result.Limit > 0 {
		opts.SetLimit(result.Limit)
	}
	if result.Skip > 0 {
		opts.SetSkip(result.Skip)
	}
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb find error: %w", err)
	}
	defer cursor.Close(ctx)
	var results []map[string]interface{}
	for cursor.Next(ctx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("mongodb decode error: %w", err)
		}
		results = append(results, doc)
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, cursor.Err()
}

// ExecuteAggregate executes an aggregation pipeline from a MongoResult.
func (d *Driver) ExecuteAggregate(ctx context.Context, result *jsonql.MongoResult) ([]map[string]interface{}, error) {
	coll := d.database.Collection(result.Collection)
	pipeline := make([]bson.M, len(result.Pipeline))
	for i, stage := range result.Pipeline {
		pipeline[i] = toBSON(stage)
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongodb aggregate error: %w", err)
	}
	defer cursor.Close(ctx)
	var results []map[string]interface{}
	for cursor.Next(ctx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("mongodb decode error: %w", err)
		}
		results = append(results, doc)
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	return results, cursor.Err()
}

// ExecuteInsert inserts a document from a MongoResult.
func (d *Driver) ExecuteInsert(ctx context.Context, result *jsonql.MongoResult) (map[string]interface{}, error) {
	coll := d.database.Collection(result.Collection)
	res, err := coll.InsertOne(ctx, result.Document)
	if err != nil {
		return nil, fmt.Errorf("mongodb insert error: %w", err)
	}
	doc := make(map[string]interface{})
	for k, v := range result.Document {
		doc[k] = v
	}
	doc["_id"] = res.InsertedID
	return doc, nil
}

// ExecuteUpdate updates documents from a MongoResult.
func (d *Driver) ExecuteUpdate(ctx context.Context, result *jsonql.MongoResult) (int64, error) {
	coll := d.database.Collection(result.Collection)
	res, err := coll.UpdateMany(ctx, toBSON(result.Filter), toBSON(result.Update))
	if err != nil {
		return 0, fmt.Errorf("mongodb update error: %w", err)
	}
	return res.ModifiedCount, nil
}

// ExecuteDelete deletes documents from a MongoResult.
func (d *Driver) ExecuteDelete(ctx context.Context, result *jsonql.MongoResult) (int64, error) {
	coll := d.database.Collection(result.Collection)
	res, err := coll.DeleteMany(ctx, toBSON(result.Filter))
	if err != nil {
		return 0, fmt.Errorf("mongodb delete error: %w", err)
	}
	return res.DeletedCount, nil
}

// Execute dispatches a MongoResult to the appropriate operation.
func (d *Driver) Execute(ctx context.Context, result *jsonql.MongoResult) (interface{}, error) {
	switch result.Operation {
	case "find":
		return d.ExecuteFind(ctx, result)
	case "aggregate":
		return d.ExecuteAggregate(ctx, result)
	case "insertOne":
		return d.ExecuteInsert(ctx, result)
	case "updateMany":
		return d.ExecuteUpdate(ctx, result)
	case "deleteMany":
		return d.ExecuteDelete(ctx, result)
	default:
		return nil, fmt.Errorf("unsupported operation: %s", result.Operation)
	}
}

// Close disconnects from MongoDB.
func (d *Driver) Close() error {
	return d.client.Disconnect(context.Background())
}

// toBSON converts a map to bson.M recursively.
func toBSON(m map[string]interface{}) bson.M {
	if m == nil {
		return bson.M{}
	}
	result := bson.M{}
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = toBSON(val)
		case []interface{}:
			arr := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arr[i] = toBSON(itemMap)
				} else {
					arr[i] = item
				}
			}
			result[k] = arr
		default:
			result[k] = v
		}
	}
	return result
}
