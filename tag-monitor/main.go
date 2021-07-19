package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"tag-monitor/helpers"

	"github.com/google/go-github/v37/github"
	"github.com/robfig/cron"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	var wg = &sync.WaitGroup{}
	// adding 1 to the waitgroup here to make the program wait indefinitely
	wg.Add(1)
	c := cron.New()
	// check for tags at start of hour (times of form **:00)
	c.AddFunc("@hourly", func() {
		// to test every minute, uncomment the following line
		// c.AddFunc("0 * * * * *", func() {
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
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(helpers.DBURI))
	if err != nil {
		log.Fatal(err)
	}

	var wgMongo = &sync.WaitGroup{}

	for _, repository := range helpers.MonitoredRepositories {
		wgMongo.Add(1)
		go checkRepositoriesForNewTags(githubClient, mongoClient, ctx, wgMongo, helpers.Organization, repository)
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

	mongoCollection := mongoClient.Database(helpers.DB).Collection(repository)
	var existingTags helpers.MongoTags
	err = mongoCollection.FindOne(ctx, bson.M{"repo": repository}).Decode(&existingTags)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			insertDoc := bson.D{
				{"repo", repository},
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
		ur, err := mongoCollection.UpdateOne(ctx, bson.M{"repo": repository}, updateDoc)
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
