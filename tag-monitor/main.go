package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/google/go-github/v37/github"
	"github.com/robfig/cron"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// repositories we track
var node = "node-mongodb-native"
var java = "mongo-java-driver" // java/reactive/scala
var c = "mongo-c-driver"
var swift = "mongo-swift-driver"
var rust = "mongo-rust-driver"
var csharp = "mongo-csharp-driver"
var goDriver = "mongo-go-driver"
var ruby = "mongo-ruby-driver"
var cxx = "mongo-cxx-driver"
var php = "mongo-php-driver"
var python = "mongo-python-driver"
var motor = "motor"
var kafka = "mongo-kafka"
var spark = "mongo-spark"

// testing purposes first

var testRepo = "rustsweeper"
var testOrg = "terakilobyte"

// mongodb vars
var DB = "tags"

type MongoTags struct {
	Repo string   `bson:"repo"`
	Tags []string `bson:"tags"`
}

var organization = "mongodb"

var monitoredRepositories = []string{
	node,
	java,
	c,
	swift,
	rust,
	csharp,
	goDriver,
	ruby,
	cxx,
	php,
	python,
	motor,
	kafka,
	spark,
}

func main() {
	var wg = &sync.WaitGroup{}
	wg.Add(1)
	c := cron.New()
	// check for tags at start of hour (times of form **:00)
	c.AddFunc("@hourly", func() {
		log.Println("Checking for new tags...")
		doChecks()
		log.Println("Done checking for new tags.")
	})
	c.Start()
	defer func() {
		c.Stop()
	}()
	wg.Wait()
}

// open connections to Atlas cluster and github repos
func doChecks() {
	githubClient := github.NewClient(nil)
	ctx := context.Background()
	uri := os.Getenv("TAG_MONITOR")
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}

	var wgMongo = &sync.WaitGroup{}

	for _, repository := range monitoredRepositories {
		wgMongo.Add(1)
		go checkRepositoriesForNewTags(githubClient, mongoClient, ctx, wgMongo, organization, repository)
	}
	wgMongo.Wait()
	if err = mongoClient.Disconnect(ctx); err != nil {
		panic(err)
	}

}

// pull list of tags for new repositories and compare them
func checkRepositoriesForNewTags(githubClient *github.Client, mongoClient *mongo.Client, ctx context.Context, wgMongo *sync.WaitGroup, organization, repository string) {
	defer wgMongo.Done()
	tags, resp, err := githubClient.Repositories.ListTags(context.Background(), organization, repository, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(resp)
	foundTags := make([]string, len(tags))
	for ix, tag := range tags {
		tagName := *tag.Name
		foundTags[ix] = tagName
	}

	mongoCollection := mongoClient.Database(DB).Collection(repository)
	var existingTags MongoTags
	err = mongoCollection.FindOne(ctx, bson.M{"repo": testRepo}).Decode(&existingTags)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			insertDoc := bson.D{
				{"repo", testRepo},
				{"tags", foundTags},
			}
			_, err := mongoCollection.InsertOne(ctx, insertDoc)
			if err != nil {
				log.Panic(err)
			}
		} else {
			log.Panic(err)
		}
	}

	for _, tag := range checkForNewTags(foundTags, existingTags.Tags) {
		fmt.Println(tag)
	}

	if len(foundTags) > 0 {
		updateDoc := bson.D{
			{"$set", bson.D{{"tags", foundTags}}},
		}
		ur, err := mongoCollection.UpdateOne(ctx, bson.M{"repo": testRepo}, updateDoc)
		if err != nil {
			log.Panic(err)
		}
		log.Println(ur.ModifiedCount, " documents updated")
	}
}

// compare arrays of tags and return difference of tags
func checkForNewTags(foundTags []string, existingTags []string) []string {
	newTags := make([]string, 2)
	for _, tag := range foundTags {
		if !contains(existingTags, tag) {
			log.Println("Found new tag:", tag)
			newTags = append(newTags, tag)
		}
	}
	return newTags
}

// check if string array contains a string.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
