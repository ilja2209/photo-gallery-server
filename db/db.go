package db

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"log"
	"time"
)

type Image struct {
	Id primitive.ObjectID `bson:"_id"`
}

func NewMongoClient(uri string, login string, psswd string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clientOptions := options.Client()
	clientOptions.ApplyURI(uri)
	clientOptions.SetAuth(options.Credential{
		Username: login,
		Password: psswd,
	})
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	} else if client == nil {
		return nil, errors.New("can't connect to DB")
	}
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func CreateImageDocument(client *mongo.Client, sourcePath string, indexationStatus bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := client.Database("gallery").Collection("images")
	res, err := collection.InsertOne(ctx, bson.M{"source_path": sourcePath, "indexation_status": indexationStatus})
	if err != nil {
		return "", err
	}
	id := res.InsertedID
	return id.(primitive.ObjectID).Hex(), nil
}

func UpdateIndexationStatus(client *mongo.Client, key string, indexationStatus bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := client.Database("gallery").Collection("images")
	id, err := primitive.ObjectIDFromHex(key)
	if err != nil {
		panic(err)
	}
	filter := bson.D{{"_id", id}}
	update := bson.D{{"$set", bson.D{{"indexation_status", indexationStatus}}}}

	_, err = collection.UpdateOne(ctx, filter, update)
	return err
}

func GetRandomImageDocument(client *mongo.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := client.Database("gallery").Collection("images")
	pipeline := []bson.M{
		bson.M{"$match": bson.M{"indexation_status": true}},
		bson.M{"$sample": bson.M{"size": 1}},
	}
	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return "", err
	}
	defer cur.Close(ctx)
	if cur.Next(ctx) {
		var result Image
		err := cur.Decode(&result)
		if err != nil {
			return "", err
		}
		return result.Id.Hex(), nil
	} else {
		err := cur.Err()
		if err != nil {
			log.Fatal(err)
		}
	}
	return "", errors.New("can't find the document")
}
